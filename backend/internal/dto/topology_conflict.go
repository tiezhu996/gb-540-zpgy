package dto

import "cadastral-boundary-topology-resolution/backend/internal/geometry"

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

type ApplySuggestionRequest struct {
	Rationale string `json:"rationale" validate:"max=2000"`
	// SnapshotHash is the review-step snapshot the reviewer confirmed. The
	// service recomputes it from the current participating parcels and refuses
	// to apply when the reviewed snapshot is stale.
	SnapshotHash string `json:"snapshot_hash" validate:"required,len=64"`
}

// SuggestionReviewParticipant reports how one parcel from the detection
// snapshot compares with the currently stored parcel at review time.
type SuggestionReviewParticipant struct {
	ParcelID       uint   `json:"parcel_id"`
	ParcelCode     string `json:"parcel_code"`
	StoredVersion  uint   `json:"stored_version"`
	CurrentVersion uint   `json:"current_version"`
	Status         string `json:"status"` // unchanged | changed | added | removed | unverified
}

// SuggestionReviewConflict is one recomputed topology finding for the stored
// snapped boundary against the current parcels.
type SuggestionReviewConflict struct {
	ConflictType     string  `json:"conflict_type"`
	MagnitudeSquareM float64 `json:"magnitude_square_m"`
	ParcelIDs        []uint  `json:"parcel_ids"`
	Explanation      string  `json:"explanation"`
	Self             bool    `json:"self"`
}

// SuggestionReview is the verifiable recheck a reviewer sees before a stored
// suggestion may be applied. CanApply is true only when the detection snapshot
// still matches every participating parcel and no residual conflict remains.
type SuggestionReview struct {
	ConflictID         uint                          `json:"conflict_id"`
	ProposalID         uint                          `json:"proposal_id"`
	ParcelID           uint                          `json:"parcel_id"`
	AreaDeltaSquareM   float64                       `json:"area_delta_square_m"`
	SnapChangeCount    int                           `json:"snap_change_count"`
	SnapChanges        []geometry.SnapChange         `json:"snap_changes"`
	SnapshotHash       string                        `json:"snapshot_hash"`
	DetectionInputHash string                        `json:"detection_input_hash"`
	SnapshotMatches    bool                          `json:"snapshot_matches"`
	Participants       []SuggestionReviewParticipant `json:"participants"`
	RecheckConflicts   []SuggestionReviewConflict    `json:"recheck_conflicts"`
	ResidualConflicts  []SuggestionReviewConflict    `json:"residual_conflicts"`
	CanApply           bool                          `json:"can_apply"`
	Blockers           []string                      `json:"blockers"`
}
