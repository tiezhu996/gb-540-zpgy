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

type ApplySuggestionRequest struct {
	Rationale string `json:"rationale" validate:"max=2000"`
}
