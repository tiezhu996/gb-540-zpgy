package dto

type ProposalQuery struct {
	ParcelID *uint
	State    string
	Page     int
	PageSize int
}

type CreateProposalRequest struct {
	ParcelID        uint    `json:"parcel_id" validate:"required,gt=0"`
	BaseVersion     uint    `json:"base_version" validate:"required,gt=0"`
	ProposedGeoJSON string  `json:"proposed_geojson" validate:"required"`
	ObservationIDs  []uint  `json:"observation_ids" validate:"max=200"`
	SnapToleranceM  float64 `json:"snap_tolerance_m" validate:"required,gt=0,lte=1000"`
	Rationale       string  `json:"rationale" validate:"max=2000"`
}

type ProposalTransitionRequest struct {
	To        string `json:"to" validate:"required"`
	Version   uint   `json:"version" validate:"required,gt=0"`
	Rationale string `json:"rationale" validate:"max=2000"`
}
