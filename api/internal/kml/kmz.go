package kml

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"strings"
)

// MaxKMLBytes caps the uncompressed KML a KMZ may expand to. The current export
// is 42 KB; this is generous headroom that still refuses a zip bomb.
const MaxKMLBytes = 64 << 20

// ErrNoKML is returned when an archive contains no .kml entry.
var ErrNoKML = errors.New("kmz contains no .kml entry")

// ParseKMZ unzips a KMZ and parses the KML inside it.
//
// Taking io.ReaderAt rather than a path is what lets the pmctl CLI hand over an
// *os.File and the upload handler hand over a bytes.Reader, so both import paths
// run identical code.
func ParseKMZ(r io.ReaderAt, size int64, rules Rules) (*Result, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open kmz: %w", err)
	}

	entry := findKML(zr)
	if entry == nil {
		return nil, ErrNoKML
	}

	rc, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", entry.Name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, MaxKMLBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	if len(data) > MaxKMLBytes {
		return nil, fmt.Errorf("%s exceeds the %d byte limit", entry.Name, MaxKMLBytes)
	}
	return Parse(data, rules)
}

// findKML prefers the conventional doc.kml and otherwise takes the first .kml
// entry, mirroring what the browser implementation did.
func findKML(zr *zip.Reader) *zip.File {
	var fallback *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "doc.kml" {
			return f
		}
		if fallback == nil && strings.EqualFold(strings.ToLower(f.Name[max(0, len(f.Name)-4):]), ".kml") {
			fallback = f
		}
	}
	return fallback
}
