package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func detectTestConflicts(t *testing.T, svc *CadastralService, proposalID uint, key string) []model.TopologyConflict {
	t.Helper()
	found, err := svc.DetectConflicts(dto.DetectConflictRequest{ProposalID: proposalID, SnapToleranceM: 0.1}, key, testActor(701, constants.RoleGISAnalyst, "detect"))
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(found) == 0 {
		t.Fatalf("DetectConflicts() returned no conflicts")
	}
	return found
}

func driveToResolutionProposed(t *testing.T, svc *CadastralService, id uint, reviewer Actor) model.TopologyConflict {
	t.Helper()
	item, err := svc.TransitionConflict(id, dto.ConflictTransitionRequest{To: constants.ConflictConfirmed}, reviewer)
	if err != nil {
		t.Fatalf("confirm conflict: %v", err)
	}
	item, err = svc.TransitionConflict(item.ID, dto.ConflictTransitionRequest{To: constants.ConflictResolutionProposed}, reviewer)
	if err != nil {
		t.Fatalf("propose resolution: %v", err)
	}
	return item
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

func TestApplyConflictSuggestionRequiresReviewedSnapshot(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	surveyor := testActor(701, constants.RoleSurveyor, "parcel-create")
	base := createTestParcel(t, svc, "P-SNAP-BASE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), surveyor)
	createTestParcel(t, svc, "P-SNAP-NEIGHBOR", serviceTestPolygon(`[10,0],[20,0],[20,10],[10,10],[10,0]`), surveyor)
	proposal := createTestProposal(t, svc, base, serviceTestPolygon(`[0,0],[11,0],[11,10],[0,10],[0,0]`), surveyor)
	found := detectTestConflicts(t, svc, proposal.ID, "snapshot-review-detect")
	if len(found) != 1 {
		t.Fatalf("detected %d conflicts, want 1", len(found))
	}
	reviewer := testActor(702, constants.RoleReviewer, "review")

	var appErr *AppError
	if _, err := svc.ReviewConflictSuggestion(found[0].ID, reviewer); !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("review before resolution_proposed error = %v, want 409 %s", err, CodeConflict)
	}
	item := driveToResolutionProposed(t, svc, found[0].ID, reviewer)

	if _, err := svc.ReviewConflictSuggestion(item.ID, testActor(703, constants.RoleSurveyor, "forbidden-review")); !errors.As(err, &appErr) || appErr.Code != CodeForbidden || appErr.Status != 403 {
		t.Fatalf("surveyor review error = %v, want 403 %s", err, CodeForbidden)
	}

	review, err := svc.ReviewConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("ReviewConflictSuggestion() error = %v", err)
	}
	if !review.CanApply || !review.SnapshotMatches {
		t.Fatalf("review = %+v, want applicable with a matching snapshot", review)
	}
	if review.AreaDeltaSquareM != 10 {
		t.Fatalf("area delta = %v, want 10", review.AreaDeltaSquareM)
	}
	if review.SnapChangeCount != 0 || len(review.SnapChanges) != 0 {
		t.Fatalf("snap changes = %d, want 0", review.SnapChangeCount)
	}
	if len(review.RecheckConflicts) != 1 || !review.RecheckConflicts[0].Self {
		t.Fatalf("recheck conflicts = %+v, want the single self finding", review.RecheckConflicts)
	}
	if len(review.ResidualConflicts) != 0 {
		t.Fatalf("residual conflicts = %+v, want none", review.ResidualConflicts)
	}
	if len(review.Participants) != 2 {
		t.Fatalf("participants = %+v, want the base parcel and one neighbour", review.Participants)
	}
	for _, participant := range review.Participants {
		if participant.Status != "unchanged" {
			t.Fatalf("participant = %+v, want unchanged", participant)
		}
	}

	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{}, reviewer); !errors.As(err, &appErr) || appErr.Code != CodeInvalidInput || appErr.Status != 400 {
		t.Fatalf("apply without snapshot hash error = %v, want 400 %s", err, CodeInvalidInput)
	}
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: strings.Repeat("0", 64)}, reviewer); !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply with a stale snapshot error = %v, want 409 %s", err, CodeConflict)
	}
	persisted, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if persisted.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want resolution_proposed preserved", persisted.ConflictState)
	}

	derived, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: review.SnapshotHash}, reviewer)
	if err != nil {
		t.Fatalf("ApplyConflictSuggestion() error = %v", err)
	}
	if derived.ProposalState != constants.ProposalDraft || derived.ParcelID != base.ID || derived.Version != proposal.Version+1 {
		t.Fatalf("derived proposal = %#v, want a new draft version", derived)
	}
	if derived.AreaDeltaSquareM != 10 {
		t.Fatalf("derived area delta = %v, want 10", derived.AreaDeltaSquareM)
	}
	persisted, err = svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload resolved conflict: %v", err)
	}
	if persisted.ConflictState != constants.ConflictResolved || persisted.ResolvedBy == nil || *persisted.ResolvedBy != reviewer.ID {
		t.Fatalf("resolved conflict = %#v, want resolved by reviewer %d", persisted, reviewer.ID)
	}
}

func TestApplyConflictSuggestionBlockedWhenParcelVersionChanges(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	surveyor := testActor(711, constants.RoleSurveyor, "parcel-create")
	base := createTestParcel(t, svc, "P-DRIFT-BASE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), surveyor)
	createTestParcel(t, svc, "P-DRIFT-NEIGHBOR", serviceTestPolygon(`[10,0],[20,0],[20,10],[10,10],[10,0]`), surveyor)
	proposal := createTestProposal(t, svc, base, serviceTestPolygon(`[0,0],[11,0],[11,10],[0,10],[0,0]`), surveyor)
	found := detectTestConflicts(t, svc, proposal.ID, "parcel-drift-detect")
	reviewer := testActor(712, constants.RoleReviewer, "review")
	item := driveToResolutionProposed(t, svc, found[0].ID, reviewer)

	newBoundary := serviceTestPolygon(`[0,0],[9.5,0],[9.5,10],[0,10],[0,0]`)
	if _, err := svc.UpdateParcel(base.ID, dto.UpdateParcelRequest{BoundaryVersion: &base.BoundaryVersion, BoundaryGeoJSON: &newBoundary}, surveyor); err != nil {
		t.Fatalf("UpdateParcel() error = %v", err)
	}

	review, err := svc.ReviewConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("ReviewConflictSuggestion() error = %v", err)
	}
	if review.SnapshotMatches || review.CanApply {
		t.Fatalf("review = %+v, want a snapshot mismatch blocking the apply", review)
	}
	var baseStatus *dto.SuggestionReviewParticipant
	for index := range review.Participants {
		if review.Participants[index].ParcelID == base.ID {
			baseStatus = &review.Participants[index]
		}
	}
	if baseStatus == nil || baseStatus.Status != "changed" || baseStatus.StoredVersion != 1 || baseStatus.CurrentVersion != 2 {
		t.Fatalf("participants = %+v, want the base parcel changed v1 -> v2", review.Participants)
	}
	if len(review.Blockers) == 0 {
		t.Fatalf("blockers should name the changed parcel")
	}

	var appErr *AppError
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: review.SnapshotHash}, reviewer); !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply after parcel change error = %v, want 409 %s", err, CodeConflict)
	}
	details, ok := appErr.Details.(dto.SuggestionReview)
	if !ok || details.CanApply {
		t.Fatalf("error details = %#v, want the blocked suggestion review", appErr.Details)
	}
	persisted, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if persisted.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want resolution_proposed preserved", persisted.ConflictState)
	}
}

func TestApplyConflictSuggestionBlockedWhenNeighbourAdded(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	surveyor := testActor(721, constants.RoleSurveyor, "parcel-create")
	base := createTestParcel(t, svc, "P-ADD-BASE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), surveyor)
	createTestParcel(t, svc, "P-ADD-NEIGHBOR", serviceTestPolygon(`[10,0],[20,0],[20,10],[10,10],[10,0]`), surveyor)
	proposal := createTestProposal(t, svc, base, serviceTestPolygon(`[0,0],[11,0],[11,10],[0,10],[0,0]`), surveyor)
	found := detectTestConflicts(t, svc, proposal.ID, "neighbour-added-detect")
	reviewer := testActor(722, constants.RoleReviewer, "review")
	item := driveToResolutionProposed(t, svc, found[0].ID, reviewer)

	added := createTestParcel(t, svc, "P-ADD-NEW", serviceTestPolygon(`[0,20],[10,20],[10,30],[0,30],[0,20]`), surveyor)
	review, err := svc.ReviewConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("ReviewConflictSuggestion() error = %v", err)
	}
	if review.SnapshotMatches || review.CanApply {
		t.Fatalf("review = %+v, want a snapshot mismatch blocking the apply", review)
	}
	var addedStatus *dto.SuggestionReviewParticipant
	for index := range review.Participants {
		if review.Participants[index].ParcelID == added.ID {
			addedStatus = &review.Participants[index]
		}
	}
	if addedStatus == nil || addedStatus.Status != "added" {
		t.Fatalf("participants = %+v, want the new neighbour marked added", review.Participants)
	}

	var appErr *AppError
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: review.SnapshotHash}, reviewer); !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply after neighbour addition error = %v, want 409 %s", err, CodeConflict)
	}
	persisted, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if persisted.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want resolution_proposed preserved", persisted.ConflictState)
	}
}

func TestApplyConflictSuggestionBlockedByResidualConflicts(t *testing.T) {
	svc, _ := newCadastralTestService(t)
	surveyor := testActor(731, constants.RoleSurveyor, "parcel-create")
	base := createTestParcel(t, svc, "P-RESID-BASE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), surveyor)
	east := createTestParcel(t, svc, "P-RESID-EAST", serviceTestPolygon(`[10,0],[20,0],[20,10],[10,10],[10,0]`), surveyor)
	north := createTestParcel(t, svc, "P-RESID-NORTH", serviceTestPolygon(`[0,10],[10,10],[10,20],[0,20],[0,10]`), surveyor)
	proposal := createTestProposal(t, svc, base, serviceTestPolygon(`[0,0],[11,0],[11,11],[0,11],[0,0]`), surveyor)
	found := detectTestConflicts(t, svc, proposal.ID, "residual-detect")
	if len(found) != 2 {
		t.Fatalf("detected %d conflicts, want 2", len(found))
	}
	reviewer := testActor(732, constants.RoleReviewer, "review")
	var target model.TopologyConflict
	for _, item := range found {
		var ids []uint
		if err := json.Unmarshal([]byte(item.ParcelIDs), &ids); err != nil {
			t.Fatalf("parse participants: %v", err)
		}
		if len(ids) == 2 && ids[1] == east.ID {
			target = item
		}
	}
	if target.ID == 0 {
		t.Fatalf("no conflict recorded for the east neighbour")
	}
	item := driveToResolutionProposed(t, svc, target.ID, reviewer)

	review, err := svc.ReviewConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("ReviewConflictSuggestion() error = %v", err)
	}
	if !review.SnapshotMatches {
		t.Fatalf("review = %+v, want a matching snapshot", review)
	}
	if review.CanApply {
		t.Fatalf("review = %+v, want blocked by the residual overlap", review)
	}
	if len(review.RecheckConflicts) != 2 {
		t.Fatalf("recheck conflicts = %+v, want both overlaps", review.RecheckConflicts)
	}
	if len(review.ResidualConflicts) != 1 {
		t.Fatalf("residual conflicts = %+v, want the north overlap", review.ResidualConflicts)
	}
	residual := review.ResidualConflicts[0]
	if residual.ConflictType != string(constants.ConflictOverlap) || residual.MagnitudeSquareM != 10 {
		t.Fatalf("residual = %+v, want a 10 m² overlap", residual)
	}
	if len(residual.ParcelIDs) != 2 || residual.ParcelIDs[1] != north.ID {
		t.Fatalf("residual parcels = %v, want the base and north parcel %d", residual.ParcelIDs, north.ID)
	}

	var appErr *AppError
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: review.SnapshotHash}, reviewer); !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply with residual conflicts error = %v, want 409 %s", err, CodeConflict)
	}
	details, ok := appErr.Details.(dto.SuggestionReview)
	if !ok || len(details.ResidualConflicts) != 1 {
		t.Fatalf("error details = %#v, want the residual review", appErr.Details)
	}
	persisted, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if persisted.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want resolution_proposed preserved", persisted.ConflictState)
	}
}
