package kml

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The XML shape.
//
// Two properties of encoding/xml do real work here for free:
//
//   - A field tagged `xml:"Folder"` with no namespace URI matches a <Folder> in
//     ANY namespace, which is exactly the el.localName === tag check the old
//     JavaScript did. The KML file declares both a default namespace and an
//     xmlns:kml prefix bound to the same URI, so pinning the URI here would be
//     fragile for no benefit.
//   - Struct field tags match DIRECT children only, which is precisely the
//     directChildren() semantics the original parser relied on to keep folder
//     context correct.

type kmlFile struct {
	XMLName  xml.Name  `xml:"kml"`
	Document container `xml:"Document"`
}

// container serves both <Folder> and <Document>; KML nests them interchangeably.
type container struct {
	Name       string      `xml:"name"`
	Placemarks []placemark `xml:"Placemark"`
	Folders    []container `xml:"Folder"`
	Documents  []container `xml:"Document"`
}

type placemark struct {
	Name        string        `xml:"name"`
	Description string        `xml:"description"`
	Point       *pointXML     `xml:"Point"`
	Polygon     *polygonXML   `xml:"Polygon"`
	Multi       *multiGeomXML `xml:"MultiGeometry"`
}

type pointXML struct {
	Coordinates string `xml:"coordinates"`
}

// polygonXML addresses the outer ring explicitly. The original JS took
// coordinates[0], which merely *happened* to be the outer ring; this stays
// correct if an export ever includes holes.
type polygonXML struct {
	Outer string   `xml:"outerBoundaryIs>LinearRing>coordinates"`
	Inner []string `xml:"innerBoundaryIs>LinearRing>coordinates"`
}

type multiGeomXML struct {
	Points   []pointXML   `xml:"Point"`
	Polygons []polygonXML `xml:"Polygon"`
}

// walkContext is the folder state inherited down the tree.
type walkContext struct {
	area     string
	category string
}

type parser struct {
	rules  Rules
	result Result
}

// Parse reads a KML document and resolves it into features and boundaries.
func Parse(data []byte, rules Rules) (*Result, error) {
	var file kmlFile
	if err := xml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse kml: %w", err)
	}

	p := &parser{rules: rules}
	p.result.Features = []Feature{}
	p.result.Boundaries = []Boundary{}
	p.result.Dropped = []Dropped{}

	// Placemarks sitting directly under the root <Document> were silently
	// ignored before; report them instead. There are none in the current export.
	for _, pm := range file.Document.Placemarks {
		p.drop(pm.Name, "/", "placemark directly under the root Document, outside any folder")
	}
	for _, folder := range file.Document.Folders {
		p.walk(folder, walkContext{}, "")
	}
	return &p.result, nil
}

func (p *parser) walk(c container, ctx walkContext, path string) {
	name := strings.TrimSpace(c.Name)
	next := ctx
	if area := NormalizeArea(name); area != "" {
		next.area = area
	}
	if category, ok := p.rules.CategoryAliases[name]; ok {
		next.category = category
	}
	here := path + "/" + name

	for _, pm := range c.Placemarks {
		p.placemark(pm, next, here)
	}
	for _, child := range c.Folders {
		p.walk(child, next, here)
	}

	// Boundaries live in a <Document> nested inside each area folder rather than
	// in a <Folder>, which is why this branch exists at all. The original parser
	// read only Polygons here and did not recurse; that is preserved, but a
	// Point found here is now reported rather than dropped in silence.
	for _, doc := range c.Documents {
		docPath := here + "/" + strings.TrimSpace(doc.Name)
		for _, pm := range doc.Placemarks {
			polys := polygonsOf(pm)
			if len(polys) > 0 {
				p.boundary(pm, polys, next, docPath)
				continue
			}
			if len(pointsOf(pm)) > 0 {
				p.drop(pm.Name, docPath, "point placemark inside a nested Document container")
			}
		}
	}
}

func (p *parser) placemark(pm placemark, ctx walkContext, path string) {
	rawName := strings.TrimSpace(pm.Name)
	if rawName == "" {
		rawName = "Tanpa nama"
	}

	if points := pointsOf(pm); len(points) > 0 {
		p.feature(rawName, points[0], ctx, path)
	}
	if polys := polygonsOf(pm); len(polys) > 0 {
		p.boundary(pm, polys, ctx, path)
	}
}

func (p *parser) feature(rawName string, at Coord, ctx walkContext, path string) {
	override, hasOverride := p.rules.Overrides[rawName]

	// Precedence matches the original: override, then inherited folder context,
	// then a last-ditch look at the placemark's own name.
	area := firstNonEmpty(override.AreaKey, ctx.area, NormalizeArea(rawName))
	if area == "" {
		p.drop(rawName, path, "could not resolve an area from the folder tree or the name")
		return
	}

	category := firstNonEmpty(override.CategoryKey, ctx.category, p.rules.FallbackCategory)
	name := firstNonEmpty(override.Name, rawName)

	p.result.Features = append(p.result.Features, Feature{
		ImportKey:       ImportKey(area, rawName),
		Name:            name,
		RawName:         rawName,
		AreaKey:         area,
		CategoryKey:     category,
		RawCategory:     ctx.category,
		Lat:             at.Lat,
		Lng:             at.Lng,
		SourceFolder:    path,
		OverrideApplied: hasOverride,
	})
}

func (p *parser) boundary(pm placemark, polys []Polygon, ctx walkContext, path string) {
	name := strings.TrimSpace(pm.Name)
	// Boundaries intentionally do not consult overrides: those correct facility
	// records, and an override keyed on a name that also matches a boundary
	// should not silently move the township outline.
	area := firstNonEmpty(ctx.area, NormalizeArea(name))
	if area == "" {
		p.drop(name, path, "could not resolve an area for a boundary polygon")
		return
	}
	description := strings.TrimSpace(pm.Description)
	p.result.Boundaries = append(p.result.Boundaries, Boundary{
		ImportKey:   ImportKey(area, name),
		AreaKey:     area,
		Name:        name,
		Description: description,
		ExternalRef: externalRef(description),
		Polygons:    polys,
	})
}

func (p *parser) drop(name, path, reason string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Tanpa nama"
	}
	p.result.Dropped = append(p.result.Dropped, Dropped{
		Name:         name,
		SourceFolder: path,
		Reason:       reason,
	})
}

// pointsOf collects a placemark's point geometry.
//
// The original used getElementsByTagNameNS('*','Point'), a descendant search
// that happened to also reach inside <MultiGeometry>. Matching direct children
// only is stricter and identical on the current data — geometry is always a
// direct child of <Placemark> here — so MultiGeometry is handled explicitly to
// keep a future export from silently losing markers.
func pointsOf(pm placemark) []Coord {
	var out []Coord
	if pm.Point != nil {
		out = append(out, parseCoordinates(pm.Point.Coordinates)...)
	}
	if pm.Multi != nil {
		for _, pt := range pm.Multi.Points {
			out = append(out, parseCoordinates(pt.Coordinates)...)
		}
	}
	return out
}

func polygonsOf(pm placemark) []Polygon {
	var out []Polygon
	add := func(x polygonXML) {
		outer := parseCoordinates(x.Outer)
		if len(outer) == 0 {
			return
		}
		poly := Polygon{Outer: outer}
		for _, ring := range x.Inner {
			if r := parseCoordinates(ring); len(r) > 0 {
				poly.Inner = append(poly.Inner, r)
			}
		}
		out = append(out, poly)
	}
	if pm.Polygon != nil {
		add(*pm.Polygon)
	}
	if pm.Multi != nil {
		for _, poly := range pm.Multi.Polygons {
			add(poly)
		}
	}
	return out
}

// parseCoordinates reads KML's "lng,lat[,alt]" tuples, skipping anything
// unparseable or non-finite — the same filter the browser version applied.
func parseCoordinates(raw string) []Coord {
	fields := strings.Fields(raw)
	out := make([]Coord, 0, len(fields))
	for _, field := range fields {
		parts := strings.Split(field, ",")
		if len(parts) < 2 {
			continue
		}
		lng, errLng := strconv.ParseFloat(parts[0], 64)
		lat, errLat := strconv.ParseFloat(parts[1], 64)
		if errLng != nil || errLat != nil {
			continue
		}
		if math.IsNaN(lat) || math.IsNaN(lng) || math.IsInf(lat, 0) || math.IsInf(lng, 0) {
			continue
		}
		out = append(out, Coord{Lng: lng, Lat: lat})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
