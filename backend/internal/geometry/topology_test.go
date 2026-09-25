package geometry

import (
	"strings"
	"testing"
)

func polygonJSON(points string) string {
	return `{"type":"Polygon","coordinates":[[` + points + `]]}`
}

func mustPolygon(t *testing.T, raw string) Polygon {
	t.Helper()
	polygon, err := ParsePolygon(raw)
	if err != nil {
		t.Fatalf("ParsePolygon() error = %v", err)
	}
	return polygon
}

func hasFinding(findings []TopologyFinding, kind string, parcelID uint) (TopologyFinding, bool) {
	for _, finding := range findings {
		if finding.ConflictType == kind && len(finding.ParcelIDs) == 1 && finding.ParcelIDs[0] == parcelID {
			return finding, true
		}
	}
	return TopologyFinding{}, false
}

func TestDetectTopologyReportsOverlapAndGap(t *testing.T) {
	base := mustPolygon(t, polygonJSON(`[0,0],[10,0],[10,10],[0,10],[0,0]`))
	neighbour := mustPolygon(t, polygonJSON(`[10,0],[20,0],[20,10],[10,10],[10,0]`))

	overlapping := mustPolygon(t, polygonJSON(`[0,0],[11,0],[11,10],[0,10],[0,0]`))
	findings, err := DetectTopology(base, overlapping, []ParcelReference{{ID: 42, Polygon: neighbour}}, 0.1)
	if err != nil {
		t.Fatalf("DetectTopology() error = %v", err)
	}
	overlap, ok := hasFinding(findings, "overlap", 42)
	if !ok {
		t.Fatalf("expected overlap against parcel 42, findings = %#v", findings)
	}
	if overlap.Magnitude < 9.99 || overlap.Magnitude > 10.01 {
		t.Fatalf("overlap area = %v, want approximately 10", overlap.Magnitude)
	}

	gapped := mustPolygon(t, polygonJSON(`[0,0],[9,0],[9,10],[0,10],[0,0]`))
	findings, err = DetectTopology(base, gapped, []ParcelReference{{ID: 42, Polygon: neighbour}}, 0.1)
	if err != nil {
		t.Fatalf("DetectTopology() gap error = %v", err)
	}
	if _, ok := hasFinding(findings, "gap", 42); !ok {
		t.Fatalf("expected gap against parcel 42, findings = %#v", findings)
	}
}

func TestDetectTopologyReportsDanglingEdge(t *testing.T) {
	base := mustPolygon(t, polygonJSON(`[0,0],[10,0],[10,10],[0,10],[0,0]`))
	neighbour := mustPolygon(t, polygonJSON(`[10,0],[20,0],[20,10],[10,10],[10,0]`))
	// This triangle touches the neighbouring parcel only at [10,5], so the
	// boundary intersection has no usable shared edge.
	dangling := mustPolygon(t, polygonJSON(`[0,0],[10,5],[0,10],[0,0]`))

	findings, err := DetectTopology(base, dangling, []ParcelReference{{ID: 42, Polygon: neighbour}}, 0.1)
	if err != nil {
		t.Fatalf("DetectTopology() dangling-edge error = %v", err)
	}
	finding, ok := hasFinding(findings, "dangling_edge", 42)
	if !ok {
		t.Fatalf("expected dangling edge against parcel 42, findings = %#v", findings)
	}
	if finding.Magnitude <= 0 {
		t.Fatalf("dangling-edge magnitude = %v, want positive evidence", finding.Magnitude)
	}
}

func TestSnapToReferencesIncludesVertexAndEdgeEvidence(t *testing.T) {
	reference := mustPolygon(t, polygonJSON(`[0,0],[10,0],[10,10],[0,10],[0,0]`))
	// The lower-left candidate vertex is closest to [10,0], whereas its
	// upper-left vertex is closest to the reference's vertical edge.
	candidate := mustPolygon(t, polygonJSON(`[10.15,0.1],[11.5,0.1],[11.5,2],[10.15,2],[10.15,0.1]`))

	result, err := SnapToReferences(candidate, []Polygon{reference}, 0.25)
	if err != nil {
		t.Fatalf("SnapToReferences() error = %v", err)
	}
	if result.SuggestedGeoJSON == "" {
		t.Fatal("suggested geometry was not encoded as GeoJSON")
	}
	if _, err := ParsePolygon(result.SuggestedGeoJSON); err != nil {
		t.Fatalf("suggested geometry is not a valid polygon: %v", err)
	}
	var hasVertex, hasEdge bool
	for _, change := range result.Changes {
		if change.Before == change.After || change.DistanceM <= 0 {
			t.Fatalf("invalid snap evidence: %#v", change)
		}
		switch change.Kind {
		case "vertex":
			hasVertex = true
		case "edge":
			hasEdge = true
		}
	}
	if !hasVertex || !hasEdge {
		t.Fatalf("snap changes = %#v, want both vertex and edge evidence", result.Changes)
	}
}

func TestGeometryRejectsClockwiseAndGeographicInput(t *testing.T) {
	clockwise := polygonJSON(`[0,0],[0,10],[10,10],[10,0],[0,0]`)
	if _, err := ParsePolygon(clockwise); err == nil || !strings.Contains(err.Error(), "counter-clockwise") {
		t.Fatalf("ParsePolygon(clockwise) error = %v, want counter-clockwise validation error", err)
	}
	if err := ValidateCoordinateSystem("EPSG:4326"); err == nil {
		t.Fatal("ValidateCoordinateSystem(EPSG:4326) unexpectedly accepted a geographic CRS")
	}
	if err := ValidateCoordinateSystem("EPSG:3857"); err != nil {
		t.Fatalf("ValidateCoordinateSystem(EPSG:3857) error = %v", err)
	}
	selfIntersecting := polygonJSON(`[0,0],[10,10],[0,10],[10,0],[0,0]`)
	if _, err := ParsePolygon(selfIntersecting); err == nil {
		t.Fatal("ParsePolygon(self-intersecting) unexpectedly accepted invalid topology")
	}
}
