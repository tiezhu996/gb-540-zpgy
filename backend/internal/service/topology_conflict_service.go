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
	hashParts := []string{parcel.BoundaryGeoJSON, proposal.ProposedGeoJSON, parcel.CoordinateSystem, fmt.Sprintf("%.6f", tolerance), geometry.AlgorithmVersion}
	for _, neighbour := range neighbourModels {
		polygon, parseErr := geometry.ParsePolygon(neighbour.BoundaryGeoJSON)
		if parseErr != nil {
			return nil, internal(fmt.Sprintf("stored geometry for neighbouring parcel %d is invalid", neighbour.ID), parseErr)
		}
		neighbours = append(neighbours, geometry.ParcelReference{ID: neighbour.ID, Polygon: polygon})
		references = append(references, polygon)
		hashParts = append(hashParts, strconv.FormatUint(uint64(neighbour.ID), 10), neighbour.BoundaryGeoJSON)
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
	inputHash := geometry.Hash(hashParts...)
	suggestedJSON, marshalErr := json.Marshal(map[string]any{
		"action": "review_snapped_boundary", "snapped_geojson": json.RawMessage(snapped.SuggestedGeoJSON), "snap_changes": snapped.Changes,
		"tolerance_m": tolerance, "coordinate_system": parcel.CoordinateSystem, "algorithm_version": geometry.AlgorithmVersion, "topology_input_hash": inputHash,
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

// conflictSuggestion is the stored deterministic suggestion produced during
// detection. It is evidence only; nothing is applied without a fresh recheck.
type conflictSuggestion struct {
	SnappedGeoJSON    json.RawMessage       `json:"snapped_geojson"`
	SnapChanges       []geometry.SnapChange `json:"snap_changes"`
	ToleranceM        float64               `json:"tolerance_m"`
	CoordinateSystem  string                `json:"coordinate_system"`
	TopologyInputHash string                `json:"topology_input_hash"`
}

// suggestionContext loads everything a recheck or apply must verify: the
// conflict, its proposal, the current parcel, and every active neighbour.
type suggestionContext struct {
	conflict   model.TopologyConflict
	proposal   model.BoundaryProposal
	parcel     model.LandParcel
	suggestion conflictSuggestion
	snapped    geometry.Polygon
	base       geometry.Polygon
	neighbours []model.LandParcel
	tolerance  float64
}

func (s *CadastralService) loadSuggestionContext(conflictID uint) (suggestionContext, error) {
	var ctx suggestionContext
	item, err := s.store.Conflicts.Get(conflictID)
	if errors.Is(err, repository.ErrNotFound) {
		return ctx, notFound("conflict")
	}
	if err != nil {
		return ctx, internal("get conflict failed", err)
	}
	ctx.conflict = item
	proposal, err := s.store.Proposals.Get(item.ProposalID)
	if errors.Is(err, repository.ErrNotFound) {
		return ctx, notFound("proposal")
	}
	if err != nil {
		return ctx, internal("load proposal failed", err)
	}
	ctx.proposal = proposal
	if err := json.Unmarshal([]byte(item.SuggestedResolutionJSON), &ctx.suggestion); err != nil || len(ctx.suggestion.SnappedGeoJSON) == 0 {
		return ctx, conflict("the conflict has no usable snapped-boundary suggestion", err)
	}
	parcel, err := s.store.Parcels.Get(proposal.ParcelID)
	if errors.Is(err, repository.ErrNotFound) {
		return ctx, notFound("parcel")
	}
	if err != nil {
		return ctx, internal("load parcel failed", err)
	}
	ctx.parcel = parcel
	base, err := geometry.ParsePolygon(parcel.BoundaryGeoJSON)
	if err != nil {
		return ctx, internal("stored parcel geometry is invalid", err)
	}
	ctx.base = base
	snapped, err := geometry.ParsePolygon(string(ctx.suggestion.SnappedGeoJSON))
	if err != nil {
		return ctx, internal("stored suggested geometry is invalid", err)
	}
	ctx.snapped = snapped
	neighbours, err := s.store.Parcels.ListActiveByCoordinateSystem(parcel.CoordinateSystem, parcel.ID)
	if err != nil {
		return ctx, internal("load neighbouring parcels failed", err)
	}
	ctx.neighbours = neighbours
	ctx.tolerance = ctx.suggestion.ToleranceM
	if ctx.tolerance <= 0 {
		ctx.tolerance = proposal.SnapToleranceM
	}
	return ctx, nil
}

// RecheckConflictSuggestion recomputes the stored suggestion against the
// current parcel version and every active neighbouring parcel. It writes
// nothing; the returned participant snapshot is the only snapshot the apply
// endpoint will accept.
func (s *CadastralService) RecheckConflictSuggestion(id uint, actor Actor) (dto.RecheckSuggestionResponse, error) {
	if err := requireAnyRole(actor, constants.RoleReviewer, constants.RoleAdmin); err != nil {
		return dto.RecheckSuggestionResponse{}, err
	}
	ctx, err := s.loadSuggestionContext(id)
	if err != nil {
		return dto.RecheckSuggestionResponse{}, err
	}
	if ctx.conflict.ConflictState != constants.ConflictResolutionProposed {
		return dto.RecheckSuggestionResponse{}, conflict("a suggestion can only be rechecked from resolution_proposed", nil)
	}
	residuals, err := residualConflicts(ctx)
	if err != nil {
		return dto.RecheckSuggestionResponse{}, err
	}
	participants := suggestionParticipants(ctx.parcel, ctx.neighbours)
	return dto.RecheckSuggestionResponse{
		ConflictID:           ctx.conflict.ID,
		ProposalID:           ctx.proposal.ID,
		ParcelID:             ctx.parcel.ID,
		ParcelCode:           ctx.parcel.ParcelCode,
		BaseVersion:          ctx.parcel.BoundaryVersion,
		CoordinateSystem:     ctx.parcel.CoordinateSystem,
		SnapToleranceM:       ctx.tolerance,
		AreaDeltaSquareM:     ctx.snapped.Area - ctx.parcel.AreaSquareM,
		MovedCoordinateCount: len(ctx.suggestion.SnapChanges),
		ResidualConflicts:    residuals,
		Participants:         participants,
		SnapshotHash:         suggestionSnapshotHash(participants),
		DetectionStale:       detectionSnapshotStale(ctx),
		Applicable:           len(residuals) == 0,
	}, nil
}

// ApplyConflictSuggestion generates the draft proposal and resolves the
// conflict only when the submitted recheck snapshot still matches the current
// participating parcels and the recalculated residual conflicts are empty. Any
// drift keeps the conflict in resolution_proposed and reports the parcels,
// conflict types, and magnitudes that blocked the apply.
func (s *CadastralService) ApplyConflictSuggestion(id uint, req dto.ApplySuggestionRequest, actor Actor) (model.BoundaryProposal, error) {
	if err := requireAnyRole(actor, constants.RoleReviewer, constants.RoleAdmin); err != nil {
		return model.BoundaryProposal{}, err
	}
	ctx, err := s.loadSuggestionContext(id)
	if err != nil {
		return model.BoundaryProposal{}, err
	}
	if ctx.conflict.ConflictState != constants.ConflictResolutionProposed {
		return model.BoundaryProposal{}, conflict("a suggestion can only be applied from resolution_proposed", nil)
	}
	submittedHash := strings.TrimSpace(req.SnapshotHash)
	if submittedHash == "" {
		return model.BoundaryProposal{}, invalid("a recheck snapshot hash is required before a suggestion can be applied", nil)
	}
	if suggestionSnapshotHash(req.Participants) != submittedHash {
		return model.BoundaryProposal{}, invalid("the submitted participant snapshot does not match its snapshot hash", nil)
	}
	participants := suggestionParticipants(ctx.parcel, ctx.neighbours)
	currentHash := suggestionSnapshotHash(participants)
	residuals, err := residualConflicts(ctx)
	if err != nil {
		return model.BoundaryProposal{}, err
	}
	if submittedHash != currentHash {
		changed, added, removed := diffParticipants(req.Participants, participants)
		details := dto.SuggestionApplyRejection{
			Reason: "participants_changed", ChangedParticipants: changed, AddedParticipants: added, RemovedParticipants: removed,
			ResidualConflicts: residuals, CurrentSnapshotHash: currentHash,
		}
		return model.BoundaryProposal{}, conflictDetails(participantDriftMessage(changed, added, removed, residuals), details)
	}
	if len(residuals) > 0 {
		details := dto.SuggestionApplyRejection{Reason: "residual_conflicts", ResidualConflicts: residuals, CurrentSnapshotHash: currentHash}
		return model.BoundaryProposal{}, conflictDetails(residualConflictMessage(residuals, participants), details)
	}
	rationale := strings.TrimSpace(req.Rationale)
	if rationale == "" {
		rationale = fmt.Sprintf("Applied deterministic suggestion from topology conflict %d after recheck snapshot %s.", ctx.conflict.ID, currentHash[:12])
	}
	derived := model.BoundaryProposal{
		ParcelID: ctx.proposal.ParcelID, BaseVersion: ctx.parcel.BoundaryVersion, ProposedGeoJSON: string(ctx.suggestion.SnappedGeoJSON), ObservationIDs: ctx.proposal.ObservationIDs,
		SnapToleranceM: ctx.proposal.SnapToleranceM, AreaDeltaSquareM: ctx.snapped.Area - ctx.parcel.AreaSquareM, ProposalState: constants.ProposalDraft,
		Rationale: rationale, Version: ctx.proposal.Version + 1, CreatedBy: actor.ID,
	}
	resolvedBy := actor.ID
	err = s.store.Transaction(func(tx *repository.Store) error {
		if createErr := tx.Proposals.Create(&derived); createErr != nil {
			return createErr
		}
		if transitionErr := tx.Conflicts.Transition(ctx.conflict.ID, ctx.conflict.ConflictState, constants.ConflictResolved, &resolvedBy); transitionErr != nil {
			return transitionErr
		}
		if auditErr := tx.Audits.Create(audit(actor, "proposal.created_from_conflict", "BoundaryProposal", derived.ID, &derived.ParcelID, "{}", snapshot(derived))); auditErr != nil {
			return auditErr
		}
		return tx.Audits.Create(audit(actor, "conflict.suggestion_applied", "TopologyConflict", ctx.conflict.ID, &derived.ID, snapshot(ctx.conflict), snapshot(map[string]any{"conflict_state": constants.ConflictResolved, "proposal_id": derived.ID, "snapshot_hash": currentHash})))
	})
	if err != nil {
		return model.BoundaryProposal{}, wrapCadastral(err, "apply conflict suggestion failed")
	}
	return derived, nil
}

// suggestionParticipants pins the target parcel and every active neighbour to
// the boundary version and geometry hash used for snapshot comparison.
func suggestionParticipants(parcel model.LandParcel, neighbours []model.LandParcel) []dto.RecheckParticipant {
	participants := make([]dto.RecheckParticipant, 0, len(neighbours)+1)
	participants = append(participants, dto.RecheckParticipant{ParcelID: parcel.ID, ParcelCode: parcel.ParcelCode, BoundaryVersion: parcel.BoundaryVersion, GeometryHash: geometry.Hash(parcel.BoundaryGeoJSON)})
	for _, neighbour := range neighbours {
		participants = append(participants, dto.RecheckParticipant{ParcelID: neighbour.ID, ParcelCode: neighbour.ParcelCode, BoundaryVersion: neighbour.BoundaryVersion, GeometryHash: geometry.Hash(neighbour.BoundaryGeoJSON)})
	}
	sort.Slice(participants, func(i, j int) bool { return participants[i].ParcelID < participants[j].ParcelID })
	return participants
}

func suggestionSnapshotHash(participants []dto.RecheckParticipant) string {
	ordered := append([]dto.RecheckParticipant(nil), participants...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ParcelID < ordered[j].ParcelID })
	parts := make([]string, 0, len(ordered)*4)
	for _, participant := range ordered {
		parts = append(parts, strconv.FormatUint(uint64(participant.ParcelID), 10), participant.ParcelCode, strconv.FormatUint(uint64(participant.BoundaryVersion), 10), participant.GeometryHash)
	}
	return geometry.Hash(parts...)
}

// residualConflicts reruns topology detection for the stored snapped boundary
// against the current parcel and neighbours, so stale suggestions can never be
// applied blindly.
func residualConflicts(ctx suggestionContext) ([]dto.RecheckResidualConflict, error) {
	references := make([]geometry.ParcelReference, 0, len(ctx.neighbours))
	for _, neighbour := range ctx.neighbours {
		polygon, err := geometry.ParsePolygon(neighbour.BoundaryGeoJSON)
		if err != nil {
			return nil, internal(fmt.Sprintf("stored geometry for neighbouring parcel %d is invalid", neighbour.ID), err)
		}
		references = append(references, geometry.ParcelReference{ID: neighbour.ID, Polygon: polygon})
	}
	findings, err := geometry.DetectTopology(ctx.base, ctx.snapped, references, ctx.tolerance)
	if err != nil {
		return nil, internal("recalculate residual topology conflicts failed", err)
	}
	residuals := make([]dto.RecheckResidualConflict, 0, len(findings))
	for _, finding := range findings {
		residuals = append(residuals, dto.RecheckResidualConflict{
			ParcelIDs: uniqueSortedIDs(append([]uint{ctx.parcel.ID}, finding.ParcelIDs...)), ConflictType: finding.ConflictType,
			MagnitudeSquareM: finding.Magnitude, Explanation: finding.Explanation,
		})
	}
	return residuals, nil
}

// detectionSnapshotStale recomputes the detection-time input hash so the
// reviewer can see whether the original detection inputs still hold.
func detectionSnapshotStale(ctx suggestionContext) bool {
	if ctx.suggestion.TopologyInputHash == "" {
		return false
	}
	hashParts := []string{ctx.parcel.BoundaryGeoJSON, ctx.proposal.ProposedGeoJSON, ctx.parcel.CoordinateSystem, fmt.Sprintf("%.6f", ctx.tolerance), geometry.AlgorithmVersion}
	for _, neighbour := range ctx.neighbours {
		hashParts = append(hashParts, strconv.FormatUint(uint64(neighbour.ID), 10), neighbour.BoundaryGeoJSON)
	}
	return geometry.Hash(hashParts...) != ctx.suggestion.TopologyInputHash
}

func diffParticipants(submitted, current []dto.RecheckParticipant) (changed []dto.ParticipantChange, added, removed []dto.RecheckParticipant) {
	submittedByID := make(map[uint]dto.RecheckParticipant, len(submitted))
	for _, participant := range submitted {
		submittedByID[participant.ParcelID] = participant
	}
	currentByID := make(map[uint]dto.RecheckParticipant, len(current))
	for _, participant := range current {
		currentByID[participant.ParcelID] = participant
	}
	for _, participant := range current {
		before, ok := submittedByID[participant.ParcelID]
		if !ok {
			added = append(added, participant)
			continue
		}
		if before.BoundaryVersion != participant.BoundaryVersion || before.GeometryHash != participant.GeometryHash {
			changed = append(changed, dto.ParticipantChange{
				ParcelID: participant.ParcelID, ParcelCode: participant.ParcelCode, SubmittedBoundaryVersion: before.BoundaryVersion,
				CurrentBoundaryVersion: participant.BoundaryVersion, GeometryChanged: before.GeometryHash != participant.GeometryHash,
			})
		}
	}
	for _, participant := range submitted {
		if _, ok := currentByID[participant.ParcelID]; !ok {
			removed = append(removed, participant)
		}
	}
	return changed, added, removed
}

func participantDriftMessage(changed []dto.ParticipantChange, added, removed []dto.RecheckParticipant, residuals []dto.RecheckResidualConflict) string {
	segments := make([]string, 0, len(changed)+len(added)+len(removed)+1)
	for _, change := range changed {
		segment := fmt.Sprintf("parcel %s version %d→%d", change.ParcelCode, change.SubmittedBoundaryVersion, change.CurrentBoundaryVersion)
		if change.GeometryChanged {
			segment += " (geometry changed)"
		}
		segments = append(segments, segment)
	}
	for _, participant := range added {
		segments = append(segments, fmt.Sprintf("parcel %s joined the participant set at version %d", participant.ParcelCode, participant.BoundaryVersion))
	}
	for _, participant := range removed {
		segments = append(segments, fmt.Sprintf("parcel %s left the participant set", participant.ParcelCode))
	}
	message := "participant snapshot changed after the recheck; the conflict stays resolution_proposed: " + strings.Join(segments, "; ")
	if summary := residualSummary(residuals); summary != "" {
		message += "; residual conflicts now: " + summary
	}
	return message
}

func residualConflictMessage(residuals []dto.RecheckResidualConflict, participants []dto.RecheckParticipant) string {
	codes := make(map[uint]string, len(participants))
	for _, participant := range participants {
		codes[participant.ParcelID] = participant.ParcelCode
	}
	segments := make([]string, 0, len(residuals))
	for _, residual := range residuals {
		labels := make([]string, 0, len(residual.ParcelIDs))
		for _, id := range residual.ParcelIDs {
			if code, ok := codes[id]; ok {
				labels = append(labels, code)
			} else {
				labels = append(labels, fmt.Sprintf("#%d", id))
			}
		}
		segments = append(segments, fmt.Sprintf("%s %.2f m² (parcels %s)", residual.ConflictType, residual.MagnitudeSquareM, strings.Join(labels, ", ")))
	}
	return "recalculation still finds residual topology conflicts; the conflict stays resolution_proposed: " + strings.Join(segments, "; ")
}

func residualSummary(residuals []dto.RecheckResidualConflict) string {
	if len(residuals) == 0 {
		return ""
	}
	segments := make([]string, 0, len(residuals))
	for _, residual := range residuals {
		segments = append(segments, fmt.Sprintf("%s %.2f m²", residual.ConflictType, residual.MagnitudeSquareM))
	}
	return strings.Join(segments, "; ")
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
	return &AppError{CodeForbidden, http.StatusForbidden, "role is not permitted for this operation", nil, nil}
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
	return &AppError{CodeInvalidInput, http.StatusUnprocessableEntity, "geometry or coordinate system is invalid: " + err.Error(), err, nil}
}

func wrapCadastral(err error, message string) error {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return err
	}
	return internal(message, err)
}
