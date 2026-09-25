package geometry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	sfgeom "github.com/peterstace/simplefeatures/geom"
)

// The service operates only on declared projected coordinate systems. The
// simplefeatures package supplies the topology validation and overlay engine;
// this package owns the cadastral-specific CRS and evidence rules around it.
const AlgorithmVersion = "parcelgraph-topology-2.0"

var epsgPattern = regexp.MustCompile(`^EPSG:([0-9]{4,6})$`)

type Point struct{ X, Y float64 }

type Polygon struct {
	Rings                  [][]Point
	Area                   float64
	MinX, MinY, MaxX, MaxY float64
	Shape                  sfgeom.Geometry
}

// ValidateCoordinateSystem deliberately uses an allow-list. A syntactically
// valid EPSG code is not proof that its unit is metres or that it is projected.
func ValidateCoordinateSystem(value string) error {
	match := epsgPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(value)))
	if len(match) != 2 {
		return errors.New("coordinate_system must use a supported projected EPSG code")
	}
	code, _ := strconv.Atoi(match[1])
	if code == 3857 || (code >= 32601 && code <= 32660) || (code >= 32701 && code <= 32760) || (code >= 4491 && code <= 4559) {
		return nil
	}
	return fmt.Errorf("EPSG:%d is not in the supported projected-metre CRS allow-list", code)
}

func ParsePolygon(raw string) (Polygon, error) {
	shape, err := sfgeom.UnmarshalGeoJSON([]byte(raw))
	if err != nil {
		return Polygon{}, fmt.Errorf("invalid GeoJSON: %w", err)
	}
	if !shape.IsPolygon() {
		return Polygon{}, errors.New("boundary geometry must be a Polygon")
	}
	polygon := shape.MustAsPolygon()
	if polygon.IsEmpty() {
		return Polygon{}, errors.New("polygon must not be empty")
	}
	if err := polygon.Validate(); err != nil {
		return Polygon{}, fmt.Errorf("invalid polygon topology: %w", err)
	}

	rings := make([][]Point, 0, polygon.NumRings())
	for ringIndex := 0; ringIndex < polygon.NumRings(); ringIndex++ {
		var ring sfgeom.LineString
		if ringIndex == 0 {
			ring = polygon.ExteriorRing()
		} else {
			ring = polygon.InteriorRingN(ringIndex - 1)
		}
		points := sequencePoints(ring.Coordinates())
		if len(points) < 4 || points[0] != points[len(points)-1] {
			return Polygon{}, fmt.Errorf("ring %d must be closed with at least four points", ringIndex)
		}
		orientation := signedArea(points)
		if ringIndex == 0 && orientation <= 0 {
			return Polygon{}, errors.New("exterior ring must use counter-clockwise orientation")
		}
		if ringIndex > 0 && orientation >= 0 {
			return Polygon{}, fmt.Errorf("interior ring %d must use clockwise orientation", ringIndex)
		}
		rings = append(rings, points)
	}

	result := Polygon{Rings: rings, Area: shape.Area(), Shape: shape, MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	for _, ring := range rings {
		for _, point := range ring {
			result.MinX, result.MinY = math.Min(result.MinX, point.X), math.Min(result.MinY, point.Y)
			result.MaxX, result.MaxY = math.Max(result.MaxX, point.X), math.Max(result.MaxY, point.Y)
		}
	}
	if result.Area <= 0 {
		return Polygon{}, errors.New("polygon area must be positive")
	}
	return result, nil
}

func ParsePoint(raw string) (Point, error) {
	shape, err := sfgeom.UnmarshalGeoJSON([]byte(raw))
	if err != nil {
		return Point{}, fmt.Errorf("invalid GeoJSON: %w", err)
	}
	point, ok := shape.AsPoint()
	if !ok || point.IsEmpty() {
		return Point{}, errors.New("observation geometry must be a non-empty Point")
	}
	xy, present := point.XY()
	if !present {
		return Point{}, errors.New("observation geometry must be a non-empty Point")
	}
	if math.IsNaN(xy.X) || math.IsNaN(xy.Y) || math.IsInf(xy.X, 0) || math.IsInf(xy.Y, 0) {
		return Point{}, errors.New("point coordinates must contain two finite values")
	}
	return Point{X: xy.X, Y: xy.Y}, nil
}

func Hash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func MarshalGeoJSON(shape sfgeom.Geometry) (string, error) {
	payload, err := json.Marshal(shape)
	if err != nil {
		return "", fmt.Errorf("encode geometry evidence: %w", err)
	}
	return string(payload), nil
}

func signedArea(ring []Point) float64 {
	area := 0.0
	for i := 0; i+1 < len(ring); i++ {
		area += ring[i].X*ring[i+1].Y - ring[i+1].X*ring[i].Y
	}
	return area / 2
}

func sequencePoints(sequence sfgeom.Sequence) []Point {
	points := make([]Point, 0, sequence.Length())
	for i := 0; i < sequence.Length(); i++ {
		xy := sequence.GetXY(i)
		points = append(points, Point{X: xy.X, Y: xy.Y})
	}
	return points
}

// SnapChange is evidence for one deterministic candidate-coordinate move. It
// is included in the proposed resolution and never changes stored geometry.
type SnapChange struct {
	Before    [2]float64 `json:"before"`
	After     [2]float64 `json:"after"`
	Kind      string     `json:"kind"`
	DistanceM float64    `json:"distance_m"`
}

type SnapResult struct {
	SuggestedGeoJSON string       `json:"suggested_geojson"`
	Polygon          Polygon      `json:"-"`
	Changes          []SnapChange `json:"changes"`
}

type referenceSegment struct{ start, end Point }

// SnapToReferences deterministically prioritizes a reference vertex and then
// the nearest projection onto a reference edge. Equal distances retain the
// first reference supplied by the caller, whose repository query is ordered.
func SnapToReferences(candidate Polygon, references []Polygon, tolerance float64) (SnapResult, error) {
	if tolerance <= 0 {
		encoded, err := MarshalGeoJSON(candidate.Shape)
		return SnapResult{SuggestedGeoJSON: encoded, Polygon: candidate}, err
	}
	vertices := make([]Point, 0)
	segments := make([]referenceSegment, 0)
	for _, reference := range references {
		for _, ring := range reference.Rings {
			for index, vertex := range ring {
				vertices = append(vertices, vertex)
				if index > 0 {
					segments = append(segments, referenceSegment{start: ring[index-1], end: vertex})
				}
			}
		}
	}
	changes := make([]SnapChange, 0)
	transformed := candidate.Shape.TransformXY(func(source sfgeom.XY) sfgeom.XY {
		from := Point{X: source.X, Y: source.Y}
		to, kind, distance, changed := nearestSnapTarget(from, vertices, segments, tolerance)
		if !changed {
			return source
		}
		changes = append(changes, SnapChange{Before: [2]float64{from.X, from.Y}, After: [2]float64{to.X, to.Y}, Kind: kind, DistanceM: distance})
		return sfgeom.XY{X: to.X, Y: to.Y}
	})
	if err := transformed.Validate(); err != nil {
		return SnapResult{}, fmt.Errorf("snapping would create invalid topology: %w", err)
	}
	encoded, err := MarshalGeoJSON(transformed)
	if err != nil {
		return SnapResult{}, err
	}
	snapped, err := ParsePolygon(encoded)
	if err != nil {
		return SnapResult{}, fmt.Errorf("parse snapped geometry: %w", err)
	}
	return SnapResult{SuggestedGeoJSON: encoded, Polygon: snapped, Changes: changes}, nil
}

func nearestSnapTarget(source Point, vertices []Point, segments []referenceSegment, tolerance float64) (Point, string, float64, bool) {
	best := Point{}
	bestKind := ""
	bestDistance := math.Inf(1)
	for _, vertex := range vertices {
		distance := pointDistance(source, vertex)
		if distance <= tolerance && distance < bestDistance {
			best, bestKind, bestDistance = vertex, "vertex", distance
		}
	}
	if bestKind == "" {
		for _, segment := range segments {
			projection := projectOntoSegment(source, segment.start, segment.end)
			distance := pointDistance(source, projection)
			if distance <= tolerance && distance < bestDistance {
				best, bestKind, bestDistance = projection, "edge", distance
			}
		}
	}
	if bestKind == "" || (best.X == source.X && best.Y == source.Y) {
		return source, "", 0, false
	}
	return best, bestKind, bestDistance, true
}

func projectOntoSegment(point, start, end Point) Point {
	dx, dy := end.X-start.X, end.Y-start.Y
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return start
	}
	ratio := ((point.X-start.X)*dx + (point.Y-start.Y)*dy) / lengthSquared
	ratio = math.Max(0, math.Min(1, ratio))
	return Point{X: start.X + ratio*dx, Y: start.Y + ratio*dy}
}

func pointDistance(left, right Point) float64 {
	return math.Hypot(left.X-right.X, left.Y-right.Y)
}

type ParcelReference struct {
	ID      uint
	Polygon Polygon
}

type TopologyFinding struct {
	ConflictType string
	Geometry     string
	Magnitude    float64
	ParcelIDs    []uint
	Explanation  string
}

// DetectTopology compares a candidate with real neighbouring parcel geometry.
// It uses overlay and distance operations rather than bounding boxes, while
// retaining enough GeoJSON evidence for a reviewer to reproduce the finding.
func DetectTopology(base, candidate Polygon, neighbours []ParcelReference, tolerance float64) ([]TopologyFinding, error) {
	findings := make([]TopologyFinding, 0)
	sort.Slice(neighbours, func(i, j int) bool { return neighbours[i].ID < neighbours[j].ID })
	for _, neighbour := range neighbours {
		overlap, err := sfgeom.Intersection(candidate.Shape, neighbour.Polygon.Shape)
		if err != nil {
			return nil, fmt.Errorf("overlay candidate parcel with %d: %w", neighbour.ID, err)
		}
		if area := overlap.Area(); area > areaThreshold(tolerance) {
			evidence, encodeErr := MarshalGeoJSON(overlap)
			if encodeErr != nil {
				return nil, encodeErr
			}
			findings = append(findings, TopologyFinding{ConflictType: "overlap", Geometry: evidence, Magnitude: area, ParcelIDs: []uint{neighbour.ID}, Explanation: "提案边界与相邻地块发生真实面叠加；请依据观测、容差与吸附证据人工消解。"})
		}

		baseDistance, baseDefined := sfgeom.Distance(base.Shape, neighbour.Polygon.Shape)
		candidateDistance, candidateDefined := sfgeom.Distance(candidate.Shape, neighbour.Polygon.Shape)
		if baseDefined && candidateDefined && baseDistance <= tolerance && candidateDistance > tolerance {
			evidence, encodeErr := MarshalGeoJSON(candidate.Shape.Boundary())
			if encodeErr != nil {
				return nil, encodeErr
			}
			findings = append(findings, TopologyFinding{ConflictType: "gap", Geometry: evidence, Magnitude: candidateDistance, ParcelIDs: []uint{neighbour.ID}, Explanation: "原始地块与相邻地块在容差内相接，但提案边界产生了可量测缝隙。"})
		}

		boundaryIntersection, boundaryErr := sfgeom.Intersection(candidate.Shape.Boundary(), neighbour.Polygon.Shape.Boundary())
		if boundaryErr != nil {
			return nil, fmt.Errorf("overlay parcel boundaries with %d: %w", neighbour.ID, boundaryErr)
		}
		sharedLength := boundaryIntersection.Length()
		if candidateDefined && candidateDistance <= tolerance && sharedLength <= tolerance {
			evidence, encodeErr := MarshalGeoJSON(boundaryIntersection)
			if encodeErr != nil {
				return nil, encodeErr
			}
			findings = append(findings, TopologyFinding{ConflictType: "dangling_edge", Geometry: evidence, Magnitude: math.Max(tolerance-sharedLength, 0), ParcelIDs: []uint{neighbour.ID}, Explanation: "提案边界只在点位或极短线段接触相邻地块，未形成可接受的共享边。"})
		}
	}
	return deduplicateFindings(findings), nil
}

func areaThreshold(tolerance float64) float64 {
	return math.Max(0.000001, tolerance*tolerance*0.0001)
}

func deduplicateFindings(input []TopologyFinding) []TopologyFinding {
	seen := make(map[string]bool, len(input))
	output := make([]TopologyFinding, 0, len(input))
	for _, finding := range input {
		key := finding.ConflictType + "|" + finding.Geometry
		if seen[key] {
			continue
		}
		seen[key] = true
		output = append(output, finding)
	}
	return output
}
