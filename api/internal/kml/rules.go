package kml

import "regexp"

// Area regexes, ported verbatim from the pre-refactor normalizeArea(). Order
// matters and matches the original: the first hit wins.
//
// These live in Go rather than in a lookup table because Postgres POSIX regex
// has no \b — it spells that \y — so a table-driven version would silently
// disagree with this one on \bKHI\b. Category aliases and per-record overrides
// are plain string maps and do live in the database.
var areaPatterns = []struct {
	key string
	re  *regexp.Regexp
}{
	{"Sedayu", regexp.MustCompile(`(?i)sedayu`)},
	{"Metland", regexp.MustCompile(`(?i)metland`)},
	{"KHI", regexp.MustCompile(`(?i)\bKHI\b|harapan indah`)},
	{"JGC", regexp.MustCompile(`(?i)\bJGC\b|jakarta garden`)},
}

// NormalizeArea maps a folder or placemark name to an area key, returning ""
// when nothing matches.
func NormalizeArea(name string) string {
	for _, p := range areaPatterns {
		if p.re.MatchString(name) {
			return p.key
		}
	}
	return ""
}

// objectIDPattern pulls the KFMap object id out of a boundary description such
// as "Indicative boundary. Source: KFMap / Knight Frank Indonesia, object ID 12189."
var objectIDPattern = regexp.MustCompile(`(?i)object\s+ID\s+(\d+)`)

// externalRef extracts a boundary's upstream identifier, or "" if absent.
func externalRef(description string) string {
	if m := objectIDPattern.FindStringSubmatch(description); m != nil {
		return m[1]
	}
	return ""
}
