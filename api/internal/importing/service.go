// Package importing turns a KMZ file into database rows.
//
// It is the single implementation behind both import paths: `pmctl import`
// hands it an *os.File, and the authenticated upload endpoint hands it a
// bytes.Reader. Both load their parsing rules from the database, so the CLI and
// the endpoint cannot drift apart.
package importing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rinaldo-mist/property-mapper/api/internal/kml"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

// SourceKind records how an import was triggered.
const (
	SourceCLI    = "cli"
	SourceUpload = "upload"
)

// Counts summarises what happened to one kind of row.
type Counts struct {
	Inserted  int `json:"inserted"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Deleted   int `json:"deleted"`
}

// Report is the operator-facing outcome of an import.
type Report struct {
	BatchID    string        `json:"batchId"`
	Filename   string        `json:"filename,omitempty"`
	Features   Counts        `json:"features"`
	Boundaries Counts        `json:"boundaries"`
	Dropped    []kml.Dropped `json:"dropped"`
	StartedAt  time.Time     `json:"startedAt"`
	FinishedAt time.Time     `json:"finishedAt"`
}

// Meta describes the origin of the file being imported.
type Meta struct {
	SourceKind string
	Filename   string
	StartedBy  pgtype.UUID // zero value for CLI runs
}

type Service struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: gen.New(pool)}
}

// ErrNoFeatures guards the orphan sweep. A KMZ that parses to zero facilities
// almost certainly means the export changed shape, not that every facility was
// demolished — so the import aborts instead of soft-deleting the whole dataset.
var ErrNoFeatures = errors.New("kmz parsed to zero facilities; refusing to sweep the existing dataset")

// LoadRules reads the category aliases and per-record overrides from the
// database. Keeping them as data means a newly discovered bad record in the next
// export needs a row, not a redeploy.
func (s *Service) LoadRules(ctx context.Context) (kml.Rules, error) {
	rules := kml.Rules{
		CategoryAliases: map[string]string{},
		Overrides:       map[string]kml.Override{},
	}

	aliases, err := s.q.ListCategoryAliases(ctx)
	if err != nil {
		return rules, fmt.Errorf("load category aliases: %w", err)
	}
	for _, a := range aliases {
		rules.CategoryAliases[a.Alias] = a.CategoryKey
	}

	overrides, err := s.q.ListImportOverrides(ctx)
	if err != nil {
		return rules, fmt.Errorf("load import overrides: %w", err)
	}
	for _, o := range overrides {
		rules.Overrides[o.MatchName] = kml.Override{
			Name:        deref(o.SetName),
			AreaKey:     deref(o.SetAreaKey),
			CategoryKey: deref(o.SetCategoryKey),
		}
	}

	fallback, err := s.q.GetFallbackCategoryKey(ctx)
	if err != nil {
		return rules, fmt.Errorf("load fallback category: %w", err)
	}
	rules.FallbackCategory = fallback

	return rules, nil
}

// RunImport parses a KMZ and reconciles the database against it.
//
// The batch row is created and finalised outside the main transaction, so a
// failed import still leaves a record of what was attempted and why it failed.
func (s *Service) RunImport(ctx context.Context, r io.ReaderAt, size int64, meta Meta) (*Report, error) {
	rules, err := s.LoadRules(ctx)
	if err != nil {
		return nil, err
	}

	digest, err := hashAll(r, size)
	if err != nil {
		return nil, err
	}

	batch, err := s.q.CreateImportBatch(ctx, gen.CreateImportBatchParams{
		SourceKind: meta.SourceKind,
		Filename:   nilIfEmpty(meta.Filename),
		FileSha256: digest,
		FileSize:   int32Ptr(size),
		StartedBy:  meta.StartedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("open import batch: %w", err)
	}

	report := &Report{
		BatchID:   uuidString(batch.ID),
		Filename:  meta.Filename,
		StartedAt: batch.StartedAt.Time,
		Dropped:   []kml.Dropped{},
	}

	runErr := s.reconcile(ctx, r, size, rules, report)
	report.FinishedAt = time.Now().UTC()

	// Recorded on a fresh context: if the caller's request was cancelled we still
	// want the batch row to stop saying "running" forever.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.finish(finishCtx, batch.ID, report, runErr); err != nil {
		if runErr != nil {
			return nil, runErr
		}
		return nil, err
	}
	if runErr != nil {
		return nil, runErr
	}
	return report, nil
}

func (s *Service) reconcile(ctx context.Context, r io.ReaderAt, size int64, rules kml.Rules, report *Report) error {
	parsed, err := kml.ParseKMZ(r, size, rules)
	if err != nil {
		return err
	}
	report.Dropped = parsed.Dropped

	if len(parsed.Features) == 0 {
		return ErrNoFeatures
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	q := gen.New(tx)

	featureKeys := make([]string, 0, len(parsed.Features))
	for _, f := range parsed.Features {
		featureKeys = append(featureKeys, f.ImportKey)

		row, err := q.UpsertImportedFacility(ctx, gen.UpsertImportedFacilityParams{
			ImportKey:       &f.ImportKey,
			Name:            f.Name,
			AreaKey:         f.AreaKey,
			CategoryKey:     f.CategoryKey,
			Lng:             f.Lng,
			Lat:             f.Lat,
			SourceFolder:    nilIfEmpty(f.SourceFolder),
			RawName:         nilIfEmpty(f.RawName),
			RawCategory:     nilIfEmpty(f.RawCategory),
			OverrideApplied: f.OverrideApplied,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// The upsert's WHERE guard suppressed a no-op update. That is the
			// definition of "unchanged", not an error.
			report.Features.Unchanged++
		case err != nil:
			return fmt.Errorf("upsert facility %q: %w", f.ImportKey, err)
		case row.Inserted:
			report.Features.Inserted++
		default:
			report.Features.Updated++
		}
	}

	deleted, err := q.SweepOrphanedFacilities(ctx, featureKeys)
	if err != nil {
		return fmt.Errorf("sweep facilities: %w", err)
	}
	report.Features.Deleted = int(deleted)

	boundaryKeys := make([]string, 0, len(parsed.Boundaries))
	for _, b := range parsed.Boundaries {
		boundaryKeys = append(boundaryKeys, b.ImportKey)

		geometry, err := geoJSONMultiPolygon(b.Polygons)
		if err != nil {
			return fmt.Errorf("encode boundary %q: %w", b.ImportKey, err)
		}

		row, err := q.UpsertBoundary(ctx, gen.UpsertBoundaryParams{
			ImportKey:   b.ImportKey,
			AreaKey:     b.AreaKey,
			Name:        b.Name,
			Description: nilIfEmpty(b.Description),
			ExternalRef: nilIfEmpty(b.ExternalRef),
			Geometry:    geometry,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			report.Boundaries.Unchanged++
		case err != nil:
			return fmt.Errorf("upsert boundary %q: %w", b.ImportKey, err)
		case row.Inserted:
			report.Boundaries.Inserted++
		default:
			report.Boundaries.Updated++
		}
	}

	// Only sweep boundaries when the export actually contained some; a KMZ with
	// facilities but no outlines should not wipe the outlines we already have.
	if len(boundaryKeys) > 0 {
		deletedBoundaries, err := q.SweepOrphanedBoundaries(ctx, boundaryKeys)
		if err != nil {
			return fmt.Errorf("sweep boundaries: %w", err)
		}
		report.Boundaries.Deleted = int(deletedBoundaries)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Service) finish(ctx context.Context, id pgtype.UUID, report *Report, runErr error) error {
	status := "succeeded"
	var errText *string
	if runErr != nil {
		status = "failed"
		msg := runErr.Error()
		errText = &msg
	}

	dropped, err := json.Marshal(report.Dropped)
	if err != nil {
		dropped = []byte("[]")
	}

	return s.q.FinishImportBatch(ctx, gen.FinishImportBatchParams{
		ID:                  id,
		Status:              status,
		FeaturesInserted:    int32(report.Features.Inserted),
		FeaturesUpdated:     int32(report.Features.Updated),
		FeaturesUnchanged:   int32(report.Features.Unchanged),
		FeaturesDeleted:     int32(report.Features.Deleted),
		BoundariesInserted:  int32(report.Boundaries.Inserted),
		BoundariesUpdated:   int32(report.Boundaries.Updated),
		BoundariesUnchanged: int32(report.Boundaries.Unchanged),
		BoundariesDeleted:   int32(report.Boundaries.Deleted),
		Dropped:             dropped,
		Error:               errText,
	})
}

// geoJSONMultiPolygon encodes rings for ST_GeomFromGeoJSON. GeoJSON uses
// [longitude, latitude] — the same order KML does — so no coordinate swapping
// happens anywhere in the import path.
func geoJSONMultiPolygon(polys []kml.Polygon) (string, error) {
	coordinates := make([][][][2]float64, 0, len(polys))
	for _, p := range polys {
		rings := make([][][2]float64, 0, 1+len(p.Inner))
		rings = append(rings, ringCoords(p.Outer))
		for _, inner := range p.Inner {
			rings = append(rings, ringCoords(inner))
		}
		coordinates = append(coordinates, rings)
	}

	encoded, err := json.Marshal(struct {
		Type        string           `json:"type"`
		Coordinates [][][][2]float64 `json:"coordinates"`
	}{Type: "MultiPolygon", Coordinates: coordinates})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func ringCoords(r kml.Ring) [][2]float64 {
	out := make([][2]float64, len(r))
	for i, c := range r {
		out[i] = [2]float64{c.Lng, c.Lat}
	}
	return out
}

func hashAll(r io.ReaderAt, size int64) ([]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(r, 0, size)); err != nil {
		return nil, fmt.Errorf("hash input: %w", err)
	}
	return h.Sum(nil), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int32Ptr(v int64) *int32 {
	n := int32(v)
	return &n
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
