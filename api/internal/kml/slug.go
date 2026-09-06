package kml

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	slugMaxLen  = 120
	slugHashLen = 8
)

// Slug reduces a name to a stable, URL-safe token.
//
// Long names are truncated and given a short hash of the full input, so two
// names that share a 120-character prefix still produce distinct slugs.
func Slug(s string) string {
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)

	var b strings.Builder
	b.Grow(len(s))
	lastDash := true // leading dashes are suppressed
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "untitled"
	}
	if len(out) <= slugMaxLen {
		return out
	}

	sum := sha256.Sum256([]byte(s))
	suffix := hex.EncodeToString(sum[:])[:slugHashLen]
	head := strings.Trim(out[:slugMaxLen-slugHashLen-1], "-")
	return head + "-" + suffix
}

// ImportKey builds the natural key an imported row is upserted on.
//
// Every part of the composition is load-bearing:
//
//   - It uses the *resolved area*, not the folder path. Folder nesting changes
//     between Google Earth exports; the township does not.
//   - It uses the *raw* KML name, not the display name, so editing an override's
//     set_name does not change the key.
//   - It deliberately excludes the category. If category were part of the key,
//     removing the "SIS JGC" override would change its key, turning an update
//     into a delete-and-recreate — which would destroy any logo an operator had
//     attached to that facility.
//   - It excludes coordinates and folder ordinals, so it survives both a
//     reordered export and a surveyor nudging a point a few metres.
//
// Collisions are therefore only possible when two facilities share an area and
// a name. The unique index on import_key turns that into a loud import failure
// rather than a silently dropped marker.
func ImportKey(areaKey, rawName string) string {
	return Slug(areaKey) + ":" + Slug(rawName)
}
