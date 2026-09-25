package dto

type ConflictQuery struct {
	ProposalID *uint
	State      string
	Type       string
	Page       int
	PageSize   int
}

type DetectConflictRequest struct {
	ProposalID     uint    `json:"proposal_id" validate:"required,gt=0"`
	SnapToleranceM float64 `json:"snap_tolerance_m" validate:"omitempty,gt=0,lte=1000"`
}

type ConflictTransitionRequest struct {
	To string `json:"to" validate:"required"`
}

// RecheckParticipant pins one participating parcel to the boundary version and
// geometry the reviewer saw when the suggestion was rechecked.
type RecheckParticipant struct {
	ParcelID        uint   `json:"parcel_id" validate:"required,gt=0"`
	ParcelCode      string `json:"parcel_code" validate:"required,min=2,max=64"`
	BoundaryVersion uint   `json:"boundary_version" validate:"required,gt=0"`
	GeometryHash    string `json:"geometry_hash" validate:"required,min=16,max=128"`
}

// RecheckResidualConflict is a freshly recalculated topology finding that the
// snapped suggestion would still leave behind.
type RecheckResidualConflict struct {
	ParcelIDs        []uint  `json:"parcel_ids"`
	ConflictType     string  `json:"conflict_type"`
	MagnitudeSquareM float64 `json:"magnitude_square_m"`
	Explanation      string  `json:"explanation"`
}

// RecheckSuggestionResponse is the verifiable review evidence computed against
// the current parcel version and every active neighbouring parcel. The
// participant snapshot must be echoed back when the suggestion is applied.
type RecheckSuggestionResponse struct {
	ConflictID           uint                      `json:"conflict_id"`
	ProposalID           uint                      `json:"proposal_id"`
	ParcelID             uint                      `json:"parcel_id"`
	ParcelCode           string                    `json:"parcel_code"`
	BaseVersion          uint                      `json:"base_version"`
	CoordinateSystem     string                    `json:"coordinate_system"`
	SnapToleranceM       float64                   `json:"snap_tolerance_m"`
	AreaDeltaSquareM     float64                   `json:"area_delta_square_m"`
	MovedCoordinateCount int                       `json:"moved_coordinate_count"`
	ResidualConflicts    []RecheckResidualConflict `json:"residual_conflicts"`
	Participants         []RecheckParticipant      `json:"participants"`
	SnapshotHash         string                    `json:"snapshot_hash"`
	DetectionStale       bool                      `json:"detection_stale"`
	Applicable           bool                      `json:"applicable"`
}

// ParticipantChange describes one parcel whose version or geometry moved
// between the rechecked snapshot and the current stored state.
type ParticipantChange struct {
	ParcelID                 uint   `json:"parcel_id"`
	ParcelCode               string `json:"parcel_code"`
	SubmittedBoundaryVersion uint   `json:"submitted_boundary_version"`
	CurrentBoundaryVersion   uint   `json:"current_boundary_version"`
	GeometryChanged          bool   `json:"geometry_changed"`
}

// SuggestionApplyRejection is the structured 409 payload returned when a
// suggestion cannot be applied; the conflict keeps its previous state.
type SuggestionApplyRejection struct {
	Reason              string                    `json:"reason"`
	ChangedParticipants []ParticipantChange       `json:"changed_participants,omitempty"`
	AddedParticipants   []RecheckParticipant      `json:"added_participants,omitempty"`
	RemovedParticipants []RecheckParticipant      `json:"removed_participants,omitempty"`
	ResidualConflicts   []RecheckResidualConflict `json:"residual_conflicts,omitempty"`
	CurrentSnapshotHash string                    `json:"current_snapshot_hash"`
}

type ApplySuggestionRequest struct {
	Rationale    string               `json:"rationale" validate:"max=2000"`
	SnapshotHash string               `json:"snapshot_hash" validate:"required,min=16,max=128"`
	Participants []RecheckParticipant `json:"participants" validate:"required,min=1,max=500,dive"`
}
