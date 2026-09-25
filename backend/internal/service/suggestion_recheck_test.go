package service

import (
	"errors"
	"strings"
	"testing"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
)

// The pentagon proposal snaps two vertices back onto the base boundary and
// still leaves a 5 m² triangular overlap with the neighbour, so detection
// records a conflict whose suggestion carries real snap evidence.
const pentagonProposalBoundary = `[0,0],[10.05,0],[11,5],[10.05,10],[0,10],[0,0]`

// The rectangle proposal sits 1 m beyond the snap tolerance, so detection
// records a 10 m² overlap that no snapping can remove.
const rectangleProposalBoundary = `[0,0],[11,0],[11,10],[0,10],[0,0]`

// setupSuggestionFixture builds a base parcel with one neighbour, a proposal,
// and runs detection so the first finding can be walked to
// resolution_proposed for recheck/apply tests.
func setupSuggestionFixture(t *testing.T, proposalBoundary string) (*CadastralService, model.LandParcel, model.LandParcel, model.TopologyConflict) {
	t.Helper()
	svc, _ := newCadastralTestService(t)
	surveyor := testActor(601, constants.RoleSurveyor, "suggestion-fixture-parcels")
	base := createTestParcel(t, svc, "P-SNAP-BASE", serviceTestPolygon(`[0,0],[10,0],[10,10],[0,10],[0,0]`), surveyor)
	neighbour := createTestParcel(t, svc, "P-SNAP-NEIGHBOR", serviceTestPolygon(`[10,0],[20,0],[20,10],[10,10],[10,0]`), surveyor)
	proposal := createTestProposal(t, svc, base, serviceTestPolygon(proposalBoundary), surveyor)

	found, err := svc.DetectConflicts(dto.DetectConflictRequest{ProposalID: proposal.ID, SnapToleranceM: 0.1}, "suggestion-fixture-"+t.Name(), testActor(602, constants.RoleGISAnalyst, "suggestion-fixture-detect"))
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(found) == 0 {
		t.Fatalf("DetectConflicts() returned no conflicts for boundary %s", proposalBoundary)
	}
	reviewer := testActor(603, constants.RoleReviewer, "suggestion-fixture-review")
	confirmed, err := svc.TransitionConflict(found[0].ID, dto.ConflictTransitionRequest{To: constants.ConflictConfirmed}, reviewer)
	if err != nil {
		t.Fatalf("confirm conflict: %v", err)
	}
	proposed, err := svc.TransitionConflict(confirmed.ID, dto.ConflictTransitionRequest{To: constants.ConflictResolutionProposed}, reviewer)
	if err != nil {
		t.Fatalf("propose resolution: %v", err)
	}
	return svc, base, neighbour, proposed
}

// moveNeighbourAway rewrites the neighbour boundary so the stored snapped
// suggestion no longer conflicts with anything.
func moveNeighbourAway(t *testing.T, svc *CadastralService, neighbour model.LandParcel) model.LandParcel {
	t.Helper()
	shifted := serviceTestPolygon(`[11.5,0],[21,0],[21,10],[11.5,10],[11.5,0]`)
	updated, err := svc.UpdateParcel(neighbour.ID, dto.UpdateParcelRequest{BoundaryGeoJSON: &shifted, BoundaryVersion: &neighbour.BoundaryVersion}, testActor(601, constants.RoleSurveyor, "neighbour-move-away"))
	if err != nil {
		t.Fatalf("move neighbour away: %v", err)
	}
	return updated
}

func TestRecheckSuggestionComputesEvidenceAgainstCurrentParcels(t *testing.T) {
	svc, base, neighbour, item := setupSuggestionFixture(t, pentagonProposalBoundary)
	reviewer := testActor(603, constants.RoleReviewer, "recheck-evidence")

	if _, err := svc.RecheckConflictSuggestion(item.ID, testActor(604, constants.RoleSurveyor, "recheck-forbidden")); err == nil {
		t.Fatal("RecheckConflictSuggestion as surveyor unexpectedly succeeded")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != CodeForbidden || appErr.Status != 403 {
			t.Fatalf("recheck as surveyor error = %v, want 403 %s", err, CodeForbidden)
		}
	}

	recheck, err := svc.RecheckConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("RecheckConflictSuggestion() error = %v", err)
	}
	if recheck.MovedCoordinateCount != 2 {
		t.Fatalf("moved coordinate count = %d, want 2 snapped vertices", recheck.MovedCoordinateCount)
	}
	if recheck.AreaDeltaSquareM < 4.99 || recheck.AreaDeltaSquareM > 5.01 {
		t.Fatalf("area delta = %v, want approximately 5 m² for the snapped pentagon", recheck.AreaDeltaSquareM)
	}
	if recheck.Applicable || len(recheck.ResidualConflicts) == 0 {
		t.Fatalf("recheck = %#v, want the recalculated 5 m² overlap to block applicability", recheck)
	}
	var overlap *dto.RecheckResidualConflict
	for index := range recheck.ResidualConflicts {
		if recheck.ResidualConflicts[index].ConflictType == string(constants.ConflictOverlap) {
			overlap = &recheck.ResidualConflicts[index]
		}
	}
	if overlap == nil || overlap.MagnitudeSquareM < 4.99 || overlap.MagnitudeSquareM > 5.01 {
		t.Fatalf("residual conflicts = %#v, want an overlap of approximately 5 m²", recheck.ResidualConflicts)
	}
	if recheck.BaseVersion != base.BoundaryVersion || recheck.ParcelCode != base.ParcelCode {
		t.Fatalf("recheck parcel = %s version %d, want %s version %d", recheck.ParcelCode, recheck.BaseVersion, base.ParcelCode, base.BoundaryVersion)
	}
	if len(recheck.Participants) != 2 || recheck.Participants[0].ParcelID != base.ID || recheck.Participants[1].ParcelID != neighbour.ID {
		t.Fatalf("participants = %#v, want base and neighbour parcels", recheck.Participants)
	}
	if recheck.SnapshotHash == "" || recheck.DetectionStale {
		t.Fatalf("snapshot hash = %q, detection stale = %v; want a fresh stable snapshot", recheck.SnapshotHash, recheck.DetectionStale)
	}
}

func TestApplySuggestionRequiresMatchingRecheckSnapshot(t *testing.T) {
	svc, base, neighbour, item := setupSuggestionFixture(t, pentagonProposalBoundary)
	reviewer := testActor(603, constants.RoleReviewer, "apply-snapshot")
	moveNeighbourAway(t, svc, neighbour)

	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{}, reviewer); err == nil {
		t.Fatal("ApplyConflictSuggestion without a snapshot unexpectedly succeeded")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != CodeInvalidInput || appErr.Status != 400 {
			t.Fatalf("apply without snapshot error = %v, want 400 %s", err, CodeInvalidInput)
		}
	}

	recheck, err := svc.RecheckConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("RecheckConflictSuggestion() error = %v", err)
	}
	if !recheck.Applicable || len(recheck.ResidualConflicts) != 0 {
		t.Fatalf("recheck after moving the neighbour = %#v, want a clean applicable snapshot", recheck)
	}
	tampered := append([]dto.RecheckParticipant(nil), recheck.Participants...)
	tampered[0].BoundaryVersion++
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: recheck.SnapshotHash, Participants: tampered}, reviewer); err == nil {
		t.Fatal("ApplyConflictSuggestion with a tampered snapshot unexpectedly succeeded")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != CodeInvalidInput || appErr.Status != 400 {
			t.Fatalf("apply with tampered snapshot error = %v, want 400 %s", err, CodeInvalidInput)
		}
	}

	derived, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: recheck.SnapshotHash, Participants: recheck.Participants}, reviewer)
	if err != nil {
		t.Fatalf("ApplyConflictSuggestion() error = %v", err)
	}
	if derived.ProposalState != constants.ProposalDraft || derived.ParcelID != base.ID || derived.BaseVersion != base.BoundaryVersion {
		t.Fatalf("derived proposal = %#v, want a draft on parcel %d base version %d", derived, base.ID, base.BoundaryVersion)
	}
	if derived.AreaDeltaSquareM < 4.99 || derived.AreaDeltaSquareM > 5.01 {
		t.Fatalf("derived area delta = %v, want approximately 5 m²", derived.AreaDeltaSquareM)
	}
	reloaded, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if reloaded.ConflictState != constants.ConflictResolved || reloaded.ResolvedBy == nil || *reloaded.ResolvedBy != reviewer.ID {
		t.Fatalf("conflict after apply = %#v, want resolved by reviewer %d", reloaded, reviewer.ID)
	}
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: recheck.SnapshotHash, Participants: recheck.Participants}, reviewer); err == nil {
		t.Fatal("second ApplyConflictSuggestion unexpectedly succeeded")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
			t.Fatalf("second apply error = %v, want 409 %s", err, CodeConflict)
		}
	}
}

func TestApplySuggestionRejectsParticipantVersionDrift(t *testing.T) {
	svc, _, neighbour, item := setupSuggestionFixture(t, pentagonProposalBoundary)
	reviewer := testActor(603, constants.RoleReviewer, "apply-drift")
	moved := moveNeighbourAway(t, svc, neighbour)

	recheck, err := svc.RecheckConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("RecheckConflictSuggestion() error = %v", err)
	}
	if !recheck.Applicable {
		t.Fatalf("recheck after moving the neighbour = %#v, want an applicable snapshot", recheck)
	}

	renamed := "renamed neighbouring parcel"
	if _, err := svc.UpdateParcel(neighbour.ID, dto.UpdateParcelRequest{Name: &renamed, BoundaryVersion: &moved.BoundaryVersion}, testActor(601, constants.RoleSurveyor, "neighbour-rename")); err != nil {
		t.Fatalf("rename neighbour: %v", err)
	}

	_, err = svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: recheck.SnapshotHash, Participants: recheck.Participants}, reviewer)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply after version drift error = %v, want 409 %s", err, CodeConflict)
	}
	if !strings.Contains(appErr.Message, "P-SNAP-NEIGHBOR") || !strings.Contains(appErr.Message, "resolution_proposed") {
		t.Fatalf("drift message = %q, want the parcel code and the preserved state", appErr.Message)
	}
	details, ok := appErr.Details.(dto.SuggestionApplyRejection)
	if !ok {
		t.Fatalf("drift details = %#v, want SuggestionApplyRejection", appErr.Details)
	}
	if details.Reason != "participants_changed" || len(details.ChangedParticipants) != 1 {
		t.Fatalf("drift details = %#v, want one changed participant", details)
	}
	change := details.ChangedParticipants[0]
	if change.ParcelCode != "P-SNAP-NEIGHBOR" || change.SubmittedBoundaryVersion != moved.BoundaryVersion || change.CurrentBoundaryVersion != moved.BoundaryVersion+1 || change.GeometryChanged {
		t.Fatalf("changed participant = %#v, want neighbour version %d→%d without geometry change", change, moved.BoundaryVersion, moved.BoundaryVersion+1)
	}
	reloaded, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if reloaded.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want it preserved as resolution_proposed", reloaded.ConflictState)
	}

	fresh, err := svc.RecheckConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("re-recheck after drift: %v", err)
	}
	if fresh.SnapshotHash == recheck.SnapshotHash {
		t.Fatal("snapshot hash did not change after a participant version bump")
	}
	if _, err := svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: fresh.SnapshotHash, Participants: fresh.Participants}, reviewer); err != nil {
		t.Fatalf("apply with a fresh snapshot error = %v", err)
	}
}

func TestApplySuggestionReportsGeometryDriftWithResiduals(t *testing.T) {
	svc, _, neighbour, item := setupSuggestionFixture(t, pentagonProposalBoundary)
	reviewer := testActor(603, constants.RoleReviewer, "apply-geometry-drift")
	moved := moveNeighbourAway(t, svc, neighbour)

	recheck, err := svc.RecheckConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("RecheckConflictSuggestion() error = %v", err)
	}
	if !recheck.Applicable {
		t.Fatalf("recheck after moving the neighbour = %#v, want an applicable snapshot", recheck)
	}

	shiftedBack := serviceTestPolygon(`[10.5,0],[20,0],[20,10],[10.5,10],[10.5,0]`)
	if _, err := svc.UpdateParcel(neighbour.ID, dto.UpdateParcelRequest{BoundaryGeoJSON: &shiftedBack, BoundaryVersion: &moved.BoundaryVersion}, testActor(601, constants.RoleSurveyor, "neighbour-shift-back")); err != nil {
		t.Fatalf("shift neighbour boundary back: %v", err)
	}

	_, err = svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: recheck.SnapshotHash, Participants: recheck.Participants}, reviewer)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply after geometry drift error = %v, want 409 %s", err, CodeConflict)
	}
	details, ok := appErr.Details.(dto.SuggestionApplyRejection)
	if !ok || details.Reason != "participants_changed" {
		t.Fatalf("geometry drift details = %#v, want participants_changed", appErr.Details)
	}
	if len(details.ChangedParticipants) != 1 || !details.ChangedParticipants[0].GeometryChanged {
		t.Fatalf("changed participants = %#v, want the neighbour flagged as geometry changed", details.ChangedParticipants)
	}
	if len(details.ResidualConflicts) == 0 {
		t.Fatal("geometry drift details carry no residual conflicts, want the recalculated overlap")
	}
	var overlap *dto.RecheckResidualConflict
	for index := range details.ResidualConflicts {
		if details.ResidualConflicts[index].ConflictType == string(constants.ConflictOverlap) {
			overlap = &details.ResidualConflicts[index]
		}
	}
	if overlap == nil || overlap.MagnitudeSquareM < 1.24 || overlap.MagnitudeSquareM > 1.26 {
		t.Fatalf("residual conflicts = %#v, want an overlap of approximately 1.25 m²", details.ResidualConflicts)
	}
	reloaded, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if reloaded.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want it preserved as resolution_proposed", reloaded.ConflictState)
	}
}

func TestApplySuggestionRejectsResidualConflicts(t *testing.T) {
	svc, base, neighbour, item := setupSuggestionFixture(t, rectangleProposalBoundary)
	reviewer := testActor(603, constants.RoleReviewer, "apply-residual")

	recheck, err := svc.RecheckConflictSuggestion(item.ID, reviewer)
	if err != nil {
		t.Fatalf("RecheckConflictSuggestion() error = %v", err)
	}
	if recheck.Applicable || len(recheck.ResidualConflicts) == 0 {
		t.Fatalf("recheck = %#v, want residual conflicts for an unsnapped 1 m overlap", recheck)
	}
	var residualOverlap *dto.RecheckResidualConflict
	for index := range recheck.ResidualConflicts {
		if recheck.ResidualConflicts[index].ConflictType == string(constants.ConflictOverlap) {
			residualOverlap = &recheck.ResidualConflicts[index]
		}
	}
	if residualOverlap == nil || residualOverlap.MagnitudeSquareM < 9.99 || residualOverlap.MagnitudeSquareM > 10.01 {
		t.Fatalf("residual conflicts = %#v, want an overlap of approximately 10 m²", recheck.ResidualConflicts)
	}
	if len(residualOverlap.ParcelIDs) != 2 || residualOverlap.ParcelIDs[0] != base.ID || residualOverlap.ParcelIDs[1] != neighbour.ID {
		t.Fatalf("residual overlap parcels = %#v, want base and neighbour", residualOverlap.ParcelIDs)
	}

	_, err = svc.ApplyConflictSuggestion(item.ID, dto.ApplySuggestionRequest{SnapshotHash: recheck.SnapshotHash, Participants: recheck.Participants}, reviewer)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict || appErr.Status != 409 {
		t.Fatalf("apply with residual conflicts error = %v, want 409 %s", err, CodeConflict)
	}
	if !strings.Contains(appErr.Message, "overlap") || !strings.Contains(appErr.Message, "P-SNAP-NEIGHBOR") {
		t.Fatalf("residual message = %q, want the conflict type and parcel code", appErr.Message)
	}
	details, ok := appErr.Details.(dto.SuggestionApplyRejection)
	if !ok || details.Reason != "residual_conflicts" || len(details.ResidualConflicts) == 0 {
		t.Fatalf("residual details = %#v, want residual_conflicts with findings", appErr.Details)
	}
	reloaded, err := svc.GetConflict(item.ID)
	if err != nil {
		t.Fatalf("reload conflict: %v", err)
	}
	if reloaded.ConflictState != constants.ConflictResolutionProposed {
		t.Fatalf("conflict state = %s, want it preserved as resolution_proposed", reloaded.ConflictState)
	}
}
