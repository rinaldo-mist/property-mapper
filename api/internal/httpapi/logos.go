package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"net/http"
	"strconv"
	"strings"

	// Registered for their decoders only: image.DecodeConfig reads dimensions
	// without decoding pixels.
	_ "image/jpeg"
	_ "image/png"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/rinaldo-mist/property-mapper/api/internal/httpx"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

// maxLogoBytes mirrors the 500 KB cap the old client-side form enforced and the
// CHECK constraint on logos.byte_size.
const maxLogoBytes = 500_000

var allowedLogoTypes = map[string]bool{
	"image/png":     true,
	"image/jpeg":    true,
	"image/webp":    true,
	"image/svg+xml": true,
}

func (s *Server) handleUploadLogo(w http.ResponseWriter, r *http.Request) error {
	// A little headroom over the cap for multipart framing, so a 500 KB file is
	// rejected by the size check below with a clear message rather than by the
	// reader with a generic one.
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoBytes+64*1024)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return httpx.TooLarge("Ukuran logo maksimal 500 KB.")
		}
		return httpx.BadRequest("Sertakan berkas pada field \"file\".")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxLogoBytes+1))
	if err != nil {
		return httpx.Internal(err)
	}
	if len(data) == 0 {
		return httpx.BadRequest("Berkas kosong.")
	}
	if len(data) > maxLogoBytes {
		return httpx.TooLarge("Ukuran logo maksimal 500 KB.")
	}

	contentType, err := resolveContentType(header.Header.Get("Content-Type"), data)
	if err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	width, height := imageDimensions(contentType, data)

	user, _ := currentUser(r.Context())
	created, err := s.q.CreateLogo(r.Context(), gen.CreateLogoParams{
		ContentType: contentType,
		ByteSize:    int32(len(data)),
		Sha256:      sum[:],
		Width:       width,
		Height:      height,
		Data:        data,
		CreatedBy:   user.ID,
	})
	if err != nil {
		return httpx.Internal(err)
	}

	httpx.JSON(w, r, http.StatusCreated, LogoUploaded{
		ID:          uuidString(created.ID),
		URL:         "/api/v1/logos/" + uuidString(created.ID),
		ContentType: created.ContentType,
		ByteSize:    int(created.ByteSize),
	})
	return nil
}

func (s *Server) handleGetLogo(w http.ResponseWriter, r *http.Request) error {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NotFound()
	}

	logo, err := s.q.GetLogo(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound()
	}
	if err != nil {
		return httpx.Internal(err)
	}

	etag := `"` + hex.EncodeToString(logo.Sha256) + `"`
	w.Header().Set("ETag", etag)
	// Safe to cache forever because ids are content-addressed: the bytes behind a
	// given id can never change, so a new image is always a new URL.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	// An uploaded SVG can contain <script>. It never executes inside an <img>,
	// so the only exposure is a user navigating directly to this URL — which
	// these three headers close off.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	w.Header().Set("Content-Type", logo.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(logo.Data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(logo.Data)
	}
	return nil
}

// resolveContentType trusts neither the client's declared type nor sniffing
// alone: the declared type must be on the allowlist AND be consistent with the
// bytes, so a .png header cannot smuggle arbitrary content.
func resolveContentType(declared string, data []byte) (string, error) {
	if i := strings.IndexByte(declared, ';'); i >= 0 {
		declared = declared[:i]
	}
	declared = strings.ToLower(strings.TrimSpace(declared))
	if !allowedLogoTypes[declared] {
		return "", httpx.UnsupportedMedia("Format logo harus PNG, JPG, WebP, atau SVG.")
	}

	sniffed := http.DetectContentType(data)
	if i := strings.IndexByte(sniffed, ';'); i >= 0 {
		sniffed = sniffed[:i]
	}
	sniffed = strings.TrimSpace(sniffed)

	switch declared {
	case "image/svg+xml":
		// DetectContentType has no SVG rule — it reports text/xml or text/plain
		// for one. Verify it at least looks like XML declaring an <svg> root.
		if !looksLikeSVG(data) {
			return "", httpx.UnsupportedMedia("Berkas bukan SVG yang valid.")
		}
	case "image/webp":
		// Go's sniffer knows WebP by its RIFF header.
		if sniffed != "image/webp" {
			return "", httpx.UnsupportedMedia("Isi berkas tidak cocok dengan format WebP.")
		}
	default:
		if sniffed != declared {
			return "", httpx.UnsupportedMedia("Isi berkas tidak cocok dengan formatnya.")
		}
	}
	return declared, nil
}

func looksLikeSVG(data []byte) bool {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

// imageDimensions reads width and height without decoding the whole image.
// Both are nil for SVG and WebP, which the standard library cannot measure here.
func imageDimensions(contentType string, data []byte) (*int32, *int32) {
	if contentType != "image/png" && contentType != "image/jpeg" {
		return nil, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil
	}
	w, h := int32(cfg.Width), int32(cfg.Height)
	return &w, &h
}
