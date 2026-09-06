package kml_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rinaldo-mist/property-mapper/api/internal/kml"
)

var update = flag.Bool("update", false, "rewrite the shared golden fixture")

// goldenPath is deliberately outside the Go tree: the same file is imported by
// the frontend's applyFilters test, so both languages assert against one fixture
// and cannot drift.
const goldenPath = "../../../testdata/facilities.golden.json"

// testRules mirrors what migration 0003 seeds. The migration is the source of
// truth at runtime; spelling the rules out here keeps the test readable and
// makes a divergence between the two visible as a failing assertion.
func testRules() kml.Rules {
	return kml.Rules{
		CategoryAliases: map[string]string{
			"Sekolah":         "School",
			"School":          "School",
			"Hospital":        "Hospital",
			"Showroom Dealer": "Showroom Dealer",
			"Gas Station":     "Gas Station",
			"University":      "University",
		},
		Overrides: map[string]kml.Override{
			// Filed under JGC/Gas Station in the export; only the category is wrong.
			"SIS JGC": {AreaKey: "JGC", CategoryKey: "School"},
			// A lone placemark under the root folder with no area or category context.
			"Suzuki": {AreaKey: "Sedayu", CategoryKey: "Showroom Dealer", Name: "Suzuki Sedayu"},
		},
		FallbackCategory: "Other",
	}
}

func parseFixture(t *testing.T) *kml.Result {
	t.Helper()
	f, err := os.Open("testdata/facility-mapping.kmz")
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	info, err := f.Stat()
	require.NoError(t, err)

	res, err := kml.ParseKMZ(f, info.Size(), testRules())
	require.NoError(t, err)
	return res
}

// TestParseKMZ_Counts is the primary regression gate. Every number here was
// measured from the real export before the parser existed.
func TestParseKMZ_Counts(t *testing.T) {
	res := parseFixture(t)

	assert.Len(t, res.Features, 43, "facility points")
	assert.Len(t, res.Boundaries, 4, "township boundaries")
	assert.Empty(t, res.Dropped, "no placemark should be unresolvable in this export")

	byArea := map[string]int{}
	byCategory := map[string]int{}
	for _, f := range res.Features {
		byArea[f.AreaKey]++
		byCategory[f.CategoryKey]++
	}

	assert.Equal(t, map[string]int{
		"KHI": 27, "Sedayu": 6, "JGC": 6, "Metland": 4,
	}, byArea)

	// "Other" is absent on purpose: no facility in this export falls back to it.
	// That is why the UI renders five category chips while the pin dialog offers
	// six — a fidelity detail the frontend must reproduce.
	assert.Equal(t, map[string]int{
		"School": 17, "Showroom Dealer": 15, "Hospital": 5, "Gas Station": 5, "University": 1,
	}, byCategory)
}

func TestParseKMZ_Overrides(t *testing.T) {
	res := parseFixture(t)
	byRawName := map[string]kml.Feature{}
	for _, f := range res.Features {
		byRawName[f.RawName] = f
	}

	t.Run("category override survives a wrong folder", func(t *testing.T) {
		sis, ok := byRawName["SIS JGC"]
		require.True(t, ok)
		assert.Equal(t, "SIS JGC", sis.Name, "name is untouched")
		assert.Equal(t, "JGC", sis.AreaKey)
		assert.Equal(t, "School", sis.CategoryKey)
		assert.Equal(t, "Gas Station", sis.RawCategory, "provenance records the folder it was filed under")
		assert.True(t, sis.OverrideApplied)
		assert.Equal(t, "jgc:sis-jgc", sis.ImportKey)
	})

	t.Run("orphan placemark gets area, category and a new name", func(t *testing.T) {
		suzuki, ok := byRawName["Suzuki"]
		require.True(t, ok)
		assert.Equal(t, "Suzuki Sedayu", suzuki.Name)
		assert.Equal(t, "Sedayu", suzuki.AreaKey)
		assert.Equal(t, "Showroom Dealer", suzuki.CategoryKey)
		assert.Empty(t, suzuki.RawCategory, "no ancestor folder named a category")
		// The key derives from the RAW name, so editing the override's set_name
		// later will not orphan a logo attached to this facility.
		assert.Equal(t, "sedayu:suzuki", suzuki.ImportKey)
	})
}

// TestParseKMZ_Boundaries fingerprints the geometry. Matching exterior ring
// point counts is strong evidence that coordinates survived the parse intact.
func TestParseKMZ_Boundaries(t *testing.T) {
	res := parseFixture(t)

	points := map[string]int{}
	for _, b := range res.Boundaries {
		require.Len(t, b.Polygons, 1, "%s: current export has no multi-part boundaries", b.AreaKey)
		require.Empty(t, b.Polygons[0].Inner, "%s: no holes expected", b.AreaKey)
		points[b.AreaKey] = len(b.Polygons[0].Outer)

		first, last := b.Polygons[0].Outer[0], b.Polygons[0].Outer[len(b.Polygons[0].Outer)-1]
		assert.Equal(t, first, last, "%s: exterior ring must be closed", b.AreaKey)
	}

	assert.Equal(t, map[string]int{
		"JGC": 229, "Metland": 134, "KHI": 49, "Sedayu": 31,
	}, points)

	for _, b := range res.Boundaries {
		assert.NotEmpty(t, b.ExternalRef, "%s: KFMap object ID should parse out of the description", b.AreaKey)
	}
}

// TestParseKMZ_Golden pins every facility. Run with -update to regenerate.
func TestParseKMZ_Golden(t *testing.T) {
	res := parseFixture(t)

	type record struct {
		ImportKey   string  `json:"importKey"`
		Name        string  `json:"name"`
		AreaKey     string  `json:"areaKey"`
		CategoryKey string  `json:"categoryKey"`
		Lat         float64 `json:"lat"`
		Lng         float64 `json:"lng"`
	}

	got := make([]record, 0, len(res.Features))
	for _, f := range res.Features {
		got = append(got, record{f.ImportKey, f.Name, f.AreaKey, f.CategoryKey, f.Lat, f.Lng})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].ImportKey < got[j].ImportKey })

	// Import keys must be unique — this is the constraint the database enforces
	// with a partial unique index, asserted here where the failure is legible.
	seen := map[string]bool{}
	for _, r := range got {
		require.False(t, seen[r.ImportKey], "duplicate import key %q", r.ImportKey)
		seen[r.ImportKey] = true
	}

	encoded, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')

	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, encoded, 0o644))
		t.Logf("wrote %s (%d facilities)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "golden fixture missing; run: go test ./internal/kml -update")
	assert.JSONEq(t, string(want), string(encoded))
}
