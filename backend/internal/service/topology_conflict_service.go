package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/geometry"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
)

func (s *CadastralService) DetectConflicts(req dto.DetectConflictRequest, idempotencyKey string, actor Actor) ([]model.TopologyConflict, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 128 {
		return nil, invalid("Idempotency-Key must contain between 1 and 128 characters", nil)
	}
	proposal, err := s.store.Proposals.Get(req.ProposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, notFound("proposal")
	}
	if err != nil {
		return nil, internal("load proposal failed", err)
	}
	tolerance := req.SnapToleranceM
	if tolerance == 0 {
		tolerance = proposal.SnapToleranceM
	}
	requestHash := geometry.Hash(strconv.FormatUint(uint64(req.ProposalID), 10), fmt.Sprintf("%.6f", tolerance))
	if existing, findErr := s.store.DetectionRuns.GetByActorKey(actor.ID, key); findErr == nil {
		return s.replayDetectionRun(existing, requestHash)
	} else if !errors.Is(findErr, repository.ErrNotFound) {
		return nil, internal("check conflict idempotency failed", findErr)
	}

	parcel, err := s.store.Parcels.Get(proposal.ParcelID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, notFound("parcel")
	}
	if err != nil {
		return nil, internal("load parcel failed", err)
	}
	base, baseErr := geometry.ParsePolygon(parcel.BoundaryGeoJSON)
	if baseErr != nil {
		return nil, geoInvalid(baseErr)
	}
	proposed, proposedErr := geometry.ParsePolygon(proposal.ProposedGeoJSON)
	if proposedErr != nil {
		return nil, geoInvalid(proposedErr)
	}
	neighbourModels, neighbourErr := s.store.Parcels.ListActiveByCoordinateSystem(parcel.CoordinateSystem, parcel.ID)
	if neighbourErr != nil {
		return nil, internal("load neighbouring parcels failed", neighbourErr)
	}
	neighbours := make([]geometry.ParcelReference, 0, len(neighbourModels))
	references := []geometry.Polygon{base}
	for _, neighbour := range neighbourModels {
		polygon, parseErr := geometry.ParsePolygon(neighbour.BoundaryGeoJSON)
		if parseErr != nil {
			return nil, internal(fmt.Sprintf("stored geometry for neighbouring parcel %d is invalid", neighbour.ID), parseErr)
		}
		neighbours = append(neighbours, geometry.ParcelReference{ID: neighbour.ID, Polygon: polygon})
		references = append(references, polygon)
	}
	sort.Slice(neighbours, func(i, j int) bool { return neighbours[i].ID < neighbours[j].ID })
	snapped, snapErr := geometry.SnapToReferences(proposed, references, tolerance)
	if snapErr != nil {
		return nil, geoInvalid(snapErr)
	}
	findings, detectErr := geometry.DetectTopology(base, snapped.Polygon, neighbours, tolerance)
	if detectErr != nil {
		return nil, internal("detect topology conflicts failed", detectErr)
	}
	inputHash := detectionInputHash(parcel, proposal, neighbourModels, tolerance)
	suggestedJSON, marshalErr := json.Marshal(map[string]any{
		"action": "review_snapped_boundary", "snapped_geojson": json.RawMessage(snapped.SuggestedGeoJSON), "snap_changes": snapped.Changes,
		"tolerance_m": tolerance, "coordinate_system": parcel.CoordinateSystem, "algorithm_version": geometry.AlgorithmVersion, "topology_input_hash": inputHash,
		"participants": suggestionParticipants(parcel, neighbourModels),
	})
	if marshalErr != nil {
		return nil, internal("encode topology suggestion failed", marshalErr)
	}
	items := make([]model.TopologyConflict, 0, len(findings))
	now := time.Now().UTC()
	for _, finding := range findings {
		participantIDs := uniqueSortedIDs(append([]uint{parcel.ID}, finding.ParcelIDs...))
		parcelIDs, encodeErr := json.Marshal(participantIDs)
		if encodeErr != nil {
			return nil, internal("encode topology participants failed", encodeErr)
		}
		items = append(items, model.TopologyConflict{
			ProposalID: proposal.ID, ParcelIDs: string(parcelIDs), ConflictType: constants.ConflictType(finding.ConflictType), GeometryGeoJSON: finding.Geometry,
			MagnitudeSquareM: finding.Magnitude, Severity: conflictSeverity(finding.ConflictType, finding.Magnitude, tolerance), AlgorithmVersion: geometry.AlgorithmVersion,
			InputHash: inputHash, ConflictState: constants.ConflictDetected, SuggestedResolutionJSON: string(suggestedJSON), Explanation: finding.Explanation, DetectedAt: now,
		})
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		for index := range items {
			if createErr := tx.Conflicts.Create(&items[index]); createErr != nil {
				return createErr
			}
			if auditErr := tx.Audits.Create(audit(actor, "conflict.detected", "TopologyConflict", items[index].ID, &parcel.ID, "{}", snapshot(items[index]))); auditErr != nil {
				return auditErr
			}
		}
		resultIDs := make([]uint, 0, len(items))
		for _, item := range items {
			resultIDs = append(resultIDs, item.ID)
		}
		encodedIDs, encodeErr := json.Marshal(resultIDs)
		if encodeErr != nil {
			return encodeErr
		}
		run := model.TopologyDetectionRun{ProposalID: proposal.ID, ActorID: actor.ID, IdempotencyKey: key, RequestHash: requestHash, InputHash: inputHash, ResultIDs: string(encodedIDs)}
		if createErr := tx.DetectionRuns.Create(&run); createErr != nil {
			return createErr
		}
		return tx.Audits.Create(audit(actor, "conflict.detection_completed", "TopologyDetectionRun", run.ID, &proposal.ID, "{}", snapshot(run)))
	})
	if err != nil {
		if existing, findErr := s.store.DetectionRuns.GetByActorKey(actor.ID, key); findErr == nil {
			return s.replayDetectionRun(existing, requestHash)
		}
		return nil, wrapCadastral(err, "detect conflicts failed")
	}
	return items, nil
}

func (s *CadastralService) ListConflicts(q dto.ConflictQuery) ([]model.TopologyConflict, dto.Pagination, error) {
	normalizePage(&q.Page, &q.PageSize)
	items, total, err := s.store.Conflicts.List(q)
	if err != nil {
		return nil, dto.Pagination{}, internal("list conflicts failed", err)
	}
	return items, dto.Pagination{Page: q.Page, PageSize: q.PageSize, Total: total}, nil
}

func (s *CadastralService) GetConflict(id uint) (model.TopologyConflict, error) {
	item, err := s.store.Conflicts.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("conflict")
	}
	if err != nil {
		return item, internal("get conflict failed", err)
	}
	return item, nil
}

func (s *CadastralService) TransitionConflict(id uint, req dto.ConflictTransitionRequest, actor Actor) (model.TopologyConflict, error) {
	item, err := s.store.Conflicts.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("conflict")
	}
	if err != nil {
		return item, internal("get conflict failed", err)
	}
	if !constants.CanConflictTransition(item.ConflictState, req.To) {
		return item, conflict("conflict state transition is not allowed", nil)
	}
	var resolved *uint
	if req.To == constants.ConflictResolved || req.To == constants.ConflictClosed {
		resolved = &actor.ID
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		if transitionErr := tx.Conflicts.Transition(id, item.ConflictState, req.To, resolved); transitionErr != nil {
			return transitionErr
		}
		return tx.Audits.Create(audit(actor, "conflict.state_changed", "TopologyConflict", id, nil, snapshot(item), snapshot(map[string]any{"state": req.To})))
	})
	if err != nil {
		return item, conflict("conflict changed while transitioning", err)
	}
	item.ConflictState = req.To
	item.ResolvedBy = resolved
	return item, nil
}

func (s *CadastralService) ReviewConflictSuggestion(id uint, actor Actor) (dto.SuggestionReview, error) {
	if err := requireAnyRole(actor, constants.RoleReviewer, constants.RoleAdmin); err != nil {
		return dto.SuggestionReview{}, err
	}
	item, err := s.store.Conflicts.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.SuggestionReview{}, notFound("conflict")
	}
	if err != nil {
		return dto.SuggestionReview{}, internal("get conflict failed", err)
	}
	if item.ConflictState != constants.ConflictResolutionProposed {
		return dto.SuggestionReview{}, conflict("a suggestion can only be reviewed from resolution_proposed", nil)
	}
	proposal, err := s.store.Proposals.Get(item.ProposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.SuggestionReview{}, notFound("proposal")
	}
	if err != nil {
		return dto.SuggestionReview{}, internal("load proposal failed", err)
	}
	return s.recheckSuggestion(item, proposal)
}

func (s *CadastralService) ApplyConflictSuggestion(id uint, req dto.ApplySuggestionRequest, actor Actor) (model.BoundaryProposal, error) {
	if err := requireAnyRole(actor, constants.RoleReviewer, constants.RoleAdmin); err != nil {
		return model.BoundaryProposal{}, err
	}
	item, err := s.store.Conflicts.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.BoundaryProposal{}, notFound("conflict")
	}
	if err != nil {
		return model.BoundaryProposal{}, internal("get conflict failed", err)
	}
	if item.ConflictState != constants.ConflictResolutionProposed {
		return model.BoundaryProposal{}, conflict("a suggestion can only be applied from resolution_proposed", nil)
	}
	proposal, err := s.store.Proposals.Get(item.ProposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.BoundaryProposal{}, notFound("proposal")
	}
	if err != nil {
		return model.BoundaryProposal{}, internal("load proposal failed", err)
	}
	submittedHash := strings.TrimSpace(req.SnapshotHash)
	if submittedHash == "" {
		return model.BoundaryProposal{}, invalid("snapshot_hash is required; run the suggestion review before applying", nil)
	}
	review, err := s.recheckSuggestion(item, proposal)
	if err != nil {
		return model.BoundaryProposal{}, err
	}
	if review.SnapshotHash != submittedHash {
		return model.BoundaryProposal{}, withDetails(conflict("the reviewed snapshot is stale; run the suggestion review again", nil), review)
	}
	if !review.CanApply {
		return model.BoundaryProposal{}, withDetails(conflict("the suggestion no longer matches the current parcels or leaves residual conflicts; the conflict stays resolution_proposed", nil), review)
	}
	suggestion, err := parseStoredSuggestion(item)
	if err != nil {
		return model.BoundaryProposal{}, err
	}
	polygon, parseErr := geometry.ParsePolygon(string(suggestion.SnappedGeoJSON))
	if parseErr != nil {
		return model.BoundaryProposal{}, internal("stored suggested geometry is invalid", parseErr)
	}
	parcel, parcelErr := s.store.Parcels.Get(proposal.ParcelID)
	if errors.Is(parcelErr, repository.ErrNotFound) {
		return model.BoundaryProposal{}, notFound("parcel")
	}
	if parcelErr != nil {
		return model.BoundaryProposal{}, internal("load parcel failed", parcelErr)
	}
	rationale := strings.TrimSpace(req.Rationale)
	if rationale == "" {
		rationale = fmt.Sprintf("Applied deterministic suggestion from topology conflict %d.", item.ID)
	}
	derived := model.BoundaryProposal{
		ParcelID: proposal.ParcelID, BaseVersion: parcel.BoundaryVersion, ProposedGeoJSON: string(suggestion.SnappedGeoJSON), ObservationIDs: proposal.ObservationIDs,
		SnapToleranceM: proposal.SnapToleranceM, AreaDeltaSquareM: polygon.Area - parcel.AreaSquareM, ProposalState: constants.ProposalDraft,
		Rationale: rationale, Version: proposal.Version + 1, CreatedBy: actor.ID,
	}
	resolvedBy := actor.ID
	err = s.store.Transaction(func(tx *repository.Store) error {
		if createErr := tx.Proposals.Create(&derived); createErr != nil {
			return createErr
		}
		if transitionErr := tx.Conflicts.Transition(item.ID, item.ConflictState, constants.ConflictResolved, &resolvedBy); transitionErr != nil {
			return transitionErr
		}
		if auditErr := tx.Audits.Create(audit(actor, "proposal.created_from_conflict", "BoundaryProposal", derived.ID, &derived.ParcelID, "{}", snapshot(derived))); auditErr != nil {
			return auditErr
		}
		return tx.Audits.Create(audit(actor, "conflict.suggestion_applied", "TopologyConflict", item.ID, &derived.ID, snapshot(item), snapshot(map[string]any{"conflict_state": constants.ConflictResolved, "proposal_id": derived.ID, "snapshot_hash": review.SnapshotHash})))
	})
	if err != nil {
		return model.BoundaryProposal{}, wrapCadastral(err, "apply conflict suggestion failed")
	}
	return derived, nil
}

// recheckSuggestion re-verifies a stored snapped-boundary suggestion against
// the current parcel version and every current neighbouring parcel. It reports
// the area delta and snap-move count from the stored evidence, recomputes the
// topology findings, and diffs the detection-time participant snapshot against
// the live parcels so a reviewer can see exactly what would be applied.
func (s *CadastralService) recheckSuggestion(item model.TopologyConflict, proposal model.BoundaryProposal) (dto.SuggestionReview, error) {
	review := dto.SuggestionReview{
		ConflictID: item.ID, ProposalID: proposal.ID, ParcelID: proposal.ParcelID, DetectionInputHash: item.InputHash,
		SnapChanges: []geometry.SnapChange{}, Participants: []dto.SuggestionReviewParticipant{},
		RecheckConflicts: []dto.SuggestionReviewConflict{}, ResidualConflicts: []dto.SuggestionReviewConflict{}, Blockers: []string{},
	}
	suggestion, err := parseStoredSuggestion(item)
	if err != nil {
		return review, err
	}
	polygon, parseErr := geometry.ParsePolygon(string(suggestion.SnappedGeoJSON))
	if parseErr != nil {
		return review, internal("stored suggested geometry is invalid", parseErr)
	}
	parcel, parcelErr := s.store.Parcels.Get(proposal.ParcelID)
	if errors.Is(parcelErr, repository.ErrNotFound) {
		return review, notFound("parcel")
	}
	if parcelErr != nil {
		return review, internal("load parcel failed", parcelErr)
	}
	base, baseErr := geometry.ParsePolygon(parcel.BoundaryGeoJSON)
	if baseErr != nil {
		return review, geoInvalid(baseErr)
	}
	neighbourModels, neighbourErr := s.store.Parcels.ListActiveByCoordinateSystem(parcel.CoordinateSystem, parcel.ID)
	if neighbourErr != nil {
		return review, internal("load neighbouring parcels failed", neighbourErr)
	}
	neighbours := make([]geometry.ParcelReference, 0, len(neighbourModels))
	for _, neighbour := range neighbourModels {
		neighbourPolygon, neighbourParseErr := geometry.ParsePolygon(neighbour.BoundaryGeoJSON)
		if neighbourParseErr != nil {
			return review, internal(fmt.Sprintf("stored geometry for neighbouring parcel %d is invalid", neighbour.ID), neighbourParseErr)
		}
		neighbours = append(neighbours, geometry.ParcelReference{ID: neighbour.ID, Polygon: neighbourPolygon})
	}
	findings, detectErr := geometry.DetectTopology(base, polygon, neighbours, suggestion.ToleranceM)
	if detectErr != nil {
		return review, internal("recheck topology conflicts failed", detectErr)
	}
	review.AreaDeltaSquareM = polygon.Area - parcel.AreaSquareM
	if suggestion.SnapChanges != nil {
		review.SnapChanges = suggestion.SnapChanges
	}
	review.SnapChangeCount = len(suggestion.SnapChanges)
	review.SnapshotHash = detectionInputHash(parcel, proposal, neighbourModels, suggestion.ToleranceM)
	review.SnapshotMatches = review.SnapshotHash == item.InputHash
	review.Participants, review.Blockers = diffSuggestionParticipants(suggestion, parcel, neighbourModels, review.SnapshotMatches)
	selfKey, keyErr := conflictParticipantKey(item)
	if keyErr != nil {
		return review, internal("stored conflict participants are invalid", keyErr)
	}
	for _, finding := range findings {
		participantIDs := uniqueSortedIDs(append([]uint{parcel.ID}, finding.ParcelIDs...))
		entry := dto.SuggestionReviewConflict{
			ConflictType: finding.ConflictType, MagnitudeSquareM: finding.Magnitude, ParcelIDs: participantIDs,
			Explanation: finding.Explanation, Self: participantKey(finding.ConflictType, participantIDs) == selfKey,
		}
		review.RecheckConflicts = append(review.RecheckConflicts, entry)
		if !entry.Self {
			review.ResidualConflicts = append(review.ResidualConflicts, entry)
			review.Blockers = append(review.Blockers, fmt.Sprintf("residual %s of %.2f remains with parcel(s) %s", entry.ConflictType, entry.MagnitudeSquareM, joinParcelIDs(entry.ParcelIDs)))
		}
	}
	review.CanApply = review.SnapshotMatches && len(review.ResidualConflicts) == 0
	return review, nil
}

// suggestionParticipant is the detection-time snapshot of one parcel the
// topology inputs depended on. It lets a later recheck attribute a changed
// input hash to specific parcels.
type suggestionParticipant struct {
	ParcelID        uint   `json:"parcel_id"`
	ParcelCode      string `json:"parcel_code"`
	BoundaryVersion uint   `json:"boundary_version"`
	BoundaryHash    string `json:"boundary_hash"`
}

type storedSuggestion struct {
	Action           string                  `json:"action"`
	SnappedGeoJSON   json.RawMessage         `json:"snapped_geojson"`
	SnapChanges      []geometry.SnapChange   `json:"snap_changes"`
	ToleranceM       float64                 `json:"tolerance_m"`
	CoordinateSystem string                  `json:"coordinate_system"`
	AlgorithmVersion string                  `json:"algorithm_version"`
	InputHash        string                  `json:"topology_input_hash"`
	Participants     []suggestionParticipant `json:"participants"`
}

func parseStoredSuggestion(item model.TopologyConflict) (storedSuggestion, error) {
	var suggestion storedSuggestion
	if err := json.Unmarshal([]byte(item.SuggestedResolutionJSON), &suggestion); err != nil || len(suggestion.SnappedGeoJSON) == 0 {
		return storedSuggestion{}, conflict("the conflict has no usable snapped-boundary suggestion", err)
	}
	return suggestion, nil
}

// detectionInputHash hashes exactly the inputs DetectConflicts used, in the
// same order, so a recheck can prove the participating parcels are unchanged.
func detectionInputHash(parcel model.LandParcel, proposal model.BoundaryProposal, neighbours []model.LandParcel, tolerance float64) string {
	hashParts := []string{parcel.BoundaryGeoJSON, proposal.ProposedGeoJSON, parcel.CoordinateSystem, fmt.Sprintf("%.6f", tolerance), geometry.AlgorithmVersion}
	for _, neighbour := range neighbours {
		hashParts = append(hashParts, strconv.FormatUint(uint64(neighbour.ID), 10), neighbour.BoundaryGeoJSON)
	}
	return geometry.Hash(hashParts...)
}

func suggestionParticipants(parcel model.LandParcel, neighbours []model.LandParcel) []suggestionParticipant {
	participants := make([]suggestionParticipant, 0, len(neighbours)+1)
	participants = append(participants, suggestionParticipant{ParcelID: parcel.ID, ParcelCode: parcel.ParcelCode, BoundaryVersion: parcel.BoundaryVersion, BoundaryHash: geometry.Hash(parcel.BoundaryGeoJSON)})
	for _, neighbour := range neighbours {
		participants = append(participants, suggestionParticipant{ParcelID: neighbour.ID, ParcelCode: neighbour.ParcelCode, BoundaryVersion: neighbour.BoundaryVersion, BoundaryHash: geometry.Hash(neighbour.BoundaryGeoJSON)})
	}
	sort.Slice(participants, func(i, j int) bool { return participants[i].ParcelID < participants[j].ParcelID })
	return participants
}

// diffSuggestionParticipants compares the detection-time participant manifest
// with the live parcels and returns the per-parcel status list plus
// human-readable blockers for every difference found.
func diffSuggestionParticipants(suggestion storedSuggestion, parcel model.LandParcel, neighbours []model.LandParcel, snapshotMatches bool) ([]dto.SuggestionReviewParticipant, []string) {
	current := suggestionParticipants(parcel, neighbours)
	blockers := []string{}
	if suggestion.CoordinateSystem != "" && suggestion.CoordinateSystem != parcel.CoordinateSystem {
		blockers = append(blockers, fmt.Sprintf("parcel coordinate system changed from %s to %s since detection", suggestion.CoordinateSystem, parcel.CoordinateSystem))
	}
	if suggestion.AlgorithmVersion != "" && suggestion.AlgorithmVersion != geometry.AlgorithmVersion {
		blockers = append(blockers, fmt.Sprintf("topology algorithm changed from %s to %s since detection", suggestion.AlgorithmVersion, geometry.AlgorithmVersion))
	}
	if len(suggestion.Participants) == 0 {
		// Suggestions stored before participant tracking only carry the input
		// hash: a match still proves the parcels, a mismatch cannot be
		// attributed to specific parcels.
		result := make([]dto.SuggestionReviewParticipant, 0, len(current))
		status := "unchanged"
		if !snapshotMatches {
			status = "unverified"
			blockers = append(blockers, "detection snapshot predates participant tracking; re-run detection to identify the changed parcels")
		}
		for _, participant := range current {
			result = append(result, dto.SuggestionReviewParticipant{ParcelID: participant.ParcelID, ParcelCode: participant.ParcelCode, CurrentVersion: participant.BoundaryVersion, Status: status})
		}
		return result, blockers
	}
	storedByID := make(map[uint]suggestionParticipant, len(suggestion.Participants))
	for _, participant := range suggestion.Participants {
		storedByID[participant.ParcelID] = participant
	}
	result := make([]dto.SuggestionReviewParticipant, 0, len(current)+1)
	for _, participant := range current {
		recorded, ok := storedByID[participant.ParcelID]
		if !ok {
			result = append(result, dto.SuggestionReviewParticipant{ParcelID: participant.ParcelID, ParcelCode: participant.ParcelCode, CurrentVersion: participant.BoundaryVersion, Status: "added"})
			blockers = append(blockers, fmt.Sprintf("parcel %s (#%d) is a new active neighbour outside the detection snapshot", participant.ParcelCode, participant.ParcelID))
			continue
		}
		delete(storedByID, participant.ParcelID)
		if recorded.BoundaryHash != participant.BoundaryHash || recorded.BoundaryVersion != participant.BoundaryVersion {
			result = append(result, dto.SuggestionReviewParticipant{ParcelID: participant.ParcelID, ParcelCode: participant.ParcelCode, StoredVersion: recorded.BoundaryVersion, CurrentVersion: participant.BoundaryVersion, Status: "changed"})
			blockers = append(blockers, fmt.Sprintf("parcel %s (#%d) boundary changed since detection (version %d -> %d)", participant.ParcelCode, participant.ParcelID, recorded.BoundaryVersion, participant.BoundaryVersion))
			continue
		}
		result = append(result, dto.SuggestionReviewParticipant{ParcelID: participant.ParcelID, ParcelCode: participant.ParcelCode, StoredVersion: recorded.BoundaryVersion, CurrentVersion: participant.BoundaryVersion, Status: "unchanged"})
	}
	removed := make([]suggestionParticipant, 0, len(storedByID))
	for _, recorded := range storedByID {
		removed = append(removed, recorded)
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].ParcelID < removed[j].ParcelID })
	for _, recorded := range removed {
		result = append(result, dto.SuggestionReviewParticipant{ParcelID: recorded.ParcelID, ParcelCode: recorded.ParcelCode, StoredVersion: recorded.BoundaryVersion, Status: "removed"})
		blockers = append(blockers, fmt.Sprintf("parcel %s (#%d) from the detection snapshot is no longer an active neighbour", recorded.ParcelCode, recorded.ParcelID))
	}
	return result, blockers
}

// conflictParticipantKey identifies the finding a conflict record represents
// so the recheck can tell the conflict being resolved apart from residual
// findings that would also apply to the snapped boundary.
func conflictParticipantKey(item model.TopologyConflict) (string, error) {
	var parcelIDs []uint
	if err := json.Unmarshal([]byte(item.ParcelIDs), &parcelIDs); err != nil {
		return "", err
	}
	return participantKey(string(item.ConflictType), parcelIDs), nil
}

func participantKey(conflictType string, parcelIDs []uint) string {
	ids := uniqueSortedIDs(append([]uint{}, parcelIDs...))
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return conflictType + "|" + strings.Join(parts, ",")
}

func joinParcelIDs(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, "#"+strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

func (s *CadastralService) replayDetectionRun(run model.TopologyDetectionRun, requestHash string) ([]model.TopologyConflict, error) {
	if run.RequestHash != requestHash {
		return nil, conflict("Idempotency-Key has already been used with a different request", nil)
	}
	var resultIDs []uint
	if err := json.Unmarshal([]byte(run.ResultIDs), &resultIDs); err != nil {
		return nil, internal("stored idempotency result is invalid", err)
	}
	items, err := s.store.Conflicts.ListByIDs(resultIDs)
	if err != nil {
		return nil, internal("load idempotent detection result failed", err)
	}
	return items, nil
}

func canTransitionObservation(from, to string) bool {
	return (from == "accepted" && (to == "rejected" || to == "superseded")) || (from == "rejected" && to == "accepted")
}

func requireAnyRole(actor Actor, roles ...string) error {
	for _, role := range roles {
		if actor.Role == role {
			return nil
		}
	}
	return &AppError{Code: CodeForbidden, Status: http.StatusForbidden, Message: "role is not permitted for this operation"}
}

func conflictSeverity(kind string, magnitude, tolerance float64) string {
	if kind == string(constants.ConflictOverlap) && magnitude > 100 {
		return "high"
	}
	if kind == string(constants.ConflictGap) && magnitude > tolerance*10 {
		return "high"
	}
	return "medium"
}

func uniqueSortedIDs(ids []uint) []uint {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	output := ids[:0]
	for _, id := range ids {
		if len(output) == 0 || output[len(output)-1] != id {
			output = append(output, id)
		}
	}
	return output
}

func geoInvalid(err error) error {
	return &AppError{Code: CodeInvalidInput, Status: http.StatusUnprocessableEntity, Message: "geometry or coordinate system is invalid: " + err.Error(), Err: err}
}

func wrapCadastral(err error, message string) error {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return err
	}
	return internal(message, err)
}
