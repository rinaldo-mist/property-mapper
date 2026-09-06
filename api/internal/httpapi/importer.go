package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/rinaldo-mist/property-mapper/api/internal/httpx"
	"github.com/rinaldo-mist/property-mapper/api/internal/importing"
	"github.com/rinaldo-mist/property-mapper/api/internal/kml"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

// maxKMZBytes is generous next to the 8 KB current export while still refusing
// anything that is obviously not a facility mapping file.
const maxKMZBytes = 20 << 20

// handleImport refreshes the dataset from an uploaded KMZ.
//
// This is the only way to re-import in production: pmctl runs as a Cloud Run Job
// for scheduled or manual operator use, but the UI needs a path that requires no
// shell access. Both call importing.Service.RunImport, so there is exactly one
// implementation of the reconciliation logic.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxKMZBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return httpx.TooLarge("Berkas KMZ terlalu besar.")
		}
		return httpx.BadRequest("Sertakan berkas KMZ pada field \"file\".")
	}
	defer file.Close()

	if name := strings.ToLower(header.Filename); name != "" &&
		!strings.HasSuffix(name, ".kmz") && !strings.HasSuffix(name, ".zip") {
		return httpx.UnsupportedMedia("Berkas harus berekstensi .kmz.")
	}

	// The archive reader needs random access, and at 8 KB buffering costs
	// nothing. This is also what lets the handler and the CLI share one method:
	// *os.File and *bytes.Reader are both io.ReaderAt.
	data, err := io.ReadAll(file)
	if err != nil {
		return httpx.Internal(err)
	}

	user, _ := currentUser(r.Context())
	report, err := s.importer.RunImport(r.Context(), bytes.NewReader(data), int64(len(data)), importing.Meta{
		SourceKind: importing.SourceUpload,
		Filename:   header.Filename,
		StartedBy:  user.ID,
	})
	switch {
	case errors.Is(err, kml.ErrNoKML):
		return httpx.BadRequest("Berkas tidak berisi dokumen KML.")
	case errors.Is(err, importing.ErrNoFeatures):
		return httpx.BadRequest(
			"Tidak ada fasilitas yang dapat dibaca dari berkas ini. " +
				"Data lama dipertahankan; periksa struktur folder pada KMZ.")
	case err != nil:
		return httpx.Internal(err)
	}

	httpx.JSON(w, r, http.StatusOK, report)
	return nil
}

func (s *Server) handleListImportBatches(w http.ResponseWriter, r *http.Request) error {
	var limit *int32
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return httpx.BadRequest("Parameter limit harus antara 1 dan 100.")
		}
		v := int32(n)
		limit = &v
	}

	rows, err := s.q.ListImportBatches(r.Context(), limit)
	if err != nil {
		return httpx.Internal(err)
	}

	items := make([]importBatch, 0, len(rows))
	for _, row := range rows {
		items = append(items, toImportBatch(row))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
	return nil
}

type importBatch struct {
	ID         string           `json:"id"`
	SourceKind string           `json:"sourceKind"`
	Filename   *string          `json:"filename"`
	Status     string           `json:"status"`
	Features   importing.Counts `json:"features"`
	Boundaries importing.Counts `json:"boundaries"`
	Dropped    json.RawMessage  `json:"dropped"`
	Error      *string          `json:"error"`
	StartedAt  string           `json:"startedAt"`
	FinishedAt *string          `json:"finishedAt"`
}

func toImportBatch(row gen.ListImportBatchesRow) importBatch {
	b := importBatch{
		ID:         uuidString(row.ID),
		SourceKind: row.SourceKind,
		Filename:   row.Filename,
		Status:     row.Status,
		Features: importing.Counts{
			Inserted:  int(row.FeaturesInserted),
			Updated:   int(row.FeaturesUpdated),
			Unchanged: int(row.FeaturesUnchanged),
			Deleted:   int(row.FeaturesDeleted),
		},
		Boundaries: importing.Counts{
			Inserted:  int(row.BoundariesInserted),
			Updated:   int(row.BoundariesUpdated),
			Unchanged: int(row.BoundariesUnchanged),
			Deleted:   int(row.BoundariesDeleted),
		},
		Dropped: json.RawMessage(row.Dropped),
		Error:   row.Error,
	}
	if row.StartedAt.Valid {
		b.StartedAt = row.StartedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if row.FinishedAt.Valid {
		finished := row.FinishedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		b.FinishedAt = &finished
	}
	return b
}
