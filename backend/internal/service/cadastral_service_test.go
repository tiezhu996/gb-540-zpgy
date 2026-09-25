package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func serviceTestPolygon(points string) string {
	return `{"type":"Polygon","coordinates":[[` + points + `]]}`
}

func newCadastralTestService(t *testing.T) (*CadastralService, *repository.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.LandParcel{}, &model.SurveyObservation{}, &model.BoundaryProposal{}, &model.TopologyConflict{}, &model.TopologyDetectionRun{}); err != nil {
		t.Fatalf("migrate SQLite: %v", err)
	}
	store := repository.NewStore(db)
	return NewCadastralService(store), store
}

func testActor(id uint, role, requestID string) Actor {
	return Actor{ID: id, Username: fmt.Sprintf("user-%d", id), Role: role, RequestID: requestID}
}

func createTestParcel(t *testing.T, svc *CadastralService, code, boundary string, actor Actor) model.LandParcel {
	t.Helper()
	parcel, err := svc.CreateParcel(dto.CreateParcelRequest{
		ParcelCode: code, Name: code, BoundaryGeoJSON: boundary,
		CoordinateSystem: "EPSG:3857", OwnerOrg: "test survey office",
	}, actor)
	if err != nil {
		t.Fatalf("CreateParcel(%s) error = %v", code, err)
	}
	return parcel
}

func createTestProposal(t *testing.T, svc *CadastralService, parcel model.LandParcel, boundary string, actor Actor) model.BoundaryProposal {
	t.Helper()
	proposal, err := svc.CreateProposal(dto.CreateProposalRequest{
		ParcelID: parcel.ID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: boundary,
		SnapToleranceM: 0.1, Rationale: "survey evidence supports the adjusted boundary",
	}, actor)
	if err != nil {
		t.Fatalf("CreateProposal() error = %v", err)
	}
	return proposal
}

func TestDetectConflictsIsIdempotentAndRejectsRequestKeyReuse(t *testing.T) {
	svc, store := newCadastralTestService(t)
	surveyor := testActor(101, constants.RoleSurveyor, "parcel-create")
	base := createTestParcel(t, svc, "P-BASE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), surveyor)
	createTestParcel(t, svc, "P-NEIGHBOR", serviceTestPolygon(`[10,0],[20,0],[20,10],[10,10],[10,0]`), testActor(102, constants.RoleSurveyor, "neighbour-create"))
	proposal := createTestProposal(t, svc, base, serviceTestPolygon(`[0,0],[11,0],[11,10],[0,10],[0,0]`), testActor(101, constants.RoleSurveyor, "proposal-create"))

	detectionActor := testActor(101, constants.RoleGISAnalyst, "detect-first")
	request := dto.DetectConflictRequest{ProposalID: proposal.ID, SnapToleranceM: 0.1}
	first, err := svc.DetectConflicts(request, "detection-idempotency-key", detectionActor)
	if err != nil {
		t.Fatalf("first DetectConflicts() error = %v", err)
	}
	if len(first) == 0 || first[0].ConflictType != constants.ConflictOverlap {
		t.Fatalf("first DetectConflicts() = %#v, want an overlap", first)
	}
	second, err := svc.DetectConflicts(request, "detection-idempotency-key", testActor(101, constants.RoleGISAnalyst, "detect-replay"))
	if err != nil {
		t.Fatalf("replayed DetectConflicts() error = %v", err)
	}
	if len(second) != len(first) || second[0].ID != first[0].ID {
		t.Fatalf("replayed result = %#v, first result = %#v", second, first)
	}
	var runCount int64
	if err := store.DB.Model(&model.TopologyDetectionRun{}).Count(&runCount).Error; err != nil {
		t.Fatalf("count detection runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("detection run count = %d, want 1", runCount)
	}
	_, err = svc.DetectConflicts(dto.DetectConflictRequest{ProposalID: proposal.ID, SnapToleranceM: 0.2}, "detection-idempotency-key", detectionActor)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("changed request with same key error = %v, want 409 %s", err, CodeConflict)
	}
}

func TestProposalReviewRequiresIndependentReviewer(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	author := testActor(201, constants.RoleSurveyor, "author-create")
	parcel := createTestParcel(t, svc, "P-REVIEW", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), author)
	proposal := createTestProposal(t, svc, parcel, serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), author)

	validated, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalValidated), Version: proposal.Version}, testActor(201, constants.RoleSurveyor, "author-validate"))
	if err != nil {
		t.Fatalf("validate proposal: %v", err)
	}
	_, err = svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalSubmitted), Version: validated.Version}, testActor(202, constants.RoleSurveyor, "other-author-submit"))
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden || appErr.Status != 403 {
		t.Fatalf("non-author authoring transition error = %v, want 403 %s", err, CodeForbidden)
	}
	submitted, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalSubmitted), Version: validated.Version}, testActor(201, constants.RoleSurveyor, "author-submit"))
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	_, err = svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalReviewed), Version: submitted.Version}, testActor(201, constants.RoleReviewer, "self-review"))
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden || appErr.Status != 403 {
		t.Fatalf("author self-review error = %v, want 403 %s", err, CodeForbidden)
	}
	_, err = svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalReviewed), Version: submitted.Version}, testActor(201, constants.RoleAdmin, "admin-self-review"))
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden || appErr.Status != 403 {
		t.Fatalf("administrator self-review error = %v, want 403 %s", err, CodeForbidden)
	}
	_, err = svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalReviewed), Version: submitted.Version}, testActor(202, constants.RoleSurveyor, "unprivileged-review"))
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden || appErr.Status != 403 {
		t.Fatalf("non-reviewer review error = %v, want 403 %s", err, CodeForbidden)
	}
	reviewed, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalReviewed), Version: submitted.Version}, testActor(203, constants.RoleReviewer, "independent-review"))
	if err != nil {
		t.Fatalf("independent reviewer transition: %v", err)
	}
	if reviewed.ProposalState != constants.ProposalReviewed || reviewed.ReviewedBy == nil || *reviewed.ReviewedBy != 203 {
		t.Fatalf("reviewed proposal = %#v, want reviewer 203", reviewed)
	}
	accepted, err := svc.TransitionProposal(proposal.ID, dto.ProposalTransitionRequest{To: string(constants.ProposalAccepted), Version: reviewed.Version}, testActor(203, constants.RoleReviewer, "independent-accept"))
	if err != nil {
		t.Fatalf("independent reviewer acceptance: %v", err)
	}
	if accepted.ProposalState != constants.ProposalAccepted || accepted.Version != reviewed.Version+1 {
		t.Fatalf("accepted proposal = %#v, want accepted state and incremented version", accepted)
	}
}

func TestCreateParcelRejectsSelfIntersectingGeometryWith422(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	_, err := svc.CreateParcel(dto.CreateParcelRequest{
		ParcelCode:       "P-BOWTIE",
		Name:             "self intersecting fixture",
		BoundaryGeoJSON:  serviceTestPolygon(`[0,0],[10,10],[0,10],[10,0],[0,0]`),
		CoordinateSystem: "EPSG:3857",
	}, testActor(401, constants.RoleSurveyor, "invalid-geometry"))
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeInvalidInput || appErr.Status != 422 {
		t.Fatalf("CreateParcel(self-intersecting) error = %v, want 422 %s", err, CodeInvalidInput)
	}
}

func TestSupersedingObservationRecordsReplacement(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	actor := testActor(501, constants.RoleSurveyor, "observation-import")
	parcel := createTestParcel(t, svc, "P-OBS", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), actor)

	first, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: parcel.ID, ObservationCode: "OBS-ORIGINAL", PointGeoJSON: `{"type":"Point","coordinates":[1,1]}`,
		ObservedAt: time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC), Method: "total_station", HorizontalAccuracyM: 0.02, SourceChecksum: "checksum-original",
	}, actor)
	if err != nil {
		t.Fatalf("import original observation: %v", err)
	}
	replacement, err := svc.ImportObservation(dto.ImportObservationRequest{
		ParcelID: parcel.ID, ObservationCode: "OBS-REPLACEMENT", PointGeoJSON: `{"type":"Point","coordinates":[1.02,1]}`,
		ObservedAt: time.Date(2026, 8, 22, 9, 1, 0, 0, time.UTC), Method: "total_station", HorizontalAccuracyM: 0.01, SourceChecksum: "checksum-replacement",
	}, actor)
	if err != nil {
		t.Fatalf("import replacement observation: %v", err)
	}

	superseded, err := svc.TransitionObservation(first.ID, dto.ObservationTransitionRequest{
		To: "superseded", Version: first.Version, ReplacementObservationID: &replacement.ID, QualityNote: "superseded by a more accurate repeat observation",
	}, testActor(501, constants.RoleSurveyor, "observation-supersede"))
	if err != nil {
		t.Fatalf("supersede observation: %v", err)
	}
	if superseded.ObservationState != "superseded" || superseded.ReplacedBy == nil || *superseded.ReplacedBy != replacement.ID {
		t.Fatalf("superseded observation = %#v, want replacement %d", superseded, replacement.ID)
	}
	persisted, err := svc.GetObservation(first.ID)
	if err != nil {
		t.Fatalf("reload superseded observation: %v", err)
	}
	if persisted.Version != first.Version+1 || persisted.ReplacedBy == nil || *persisted.ReplacedBy != replacement.ID {
		t.Fatalf("persisted observation = %#v, want incremented version and replacement", persisted)
	}
}
