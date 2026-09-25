package model

import (
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
)

// BoundaryProposal is a versioned internal suggestion, not a cadastral title.
type BoundaryProposal struct {
	ID               uint                    `gorm:"primaryKey" json:"id"`
	ParcelID         uint                    `gorm:"not null;index" json:"parcel_id"`
	BaseVersion      uint                    `gorm:"not null" json:"base_version"`
	ProposedGeoJSON  string                  `gorm:"column:proposed_geojson;type:text;not null" json:"proposed_geojson"`
	ObservationIDs   string                  `gorm:"type:text" json:"observation_ids"`
	SnapToleranceM   float64                 `gorm:"not null" json:"snap_tolerance_m"`
	AreaDeltaSquareM float64                 `gorm:"not null" json:"area_delta_square_m"`
	ProposalState    constants.ProposalState `gorm:"size:24;not null;index" json:"proposal_state"`
	Rationale        string                  `gorm:"size:2000" json:"rationale"`
	Version          uint                    `gorm:"not null;default:1" json:"version"`
	CreatedBy        uint                    `gorm:"not null" json:"created_by"`
	ReviewedBy       *uint                   `json:"reviewed_by"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}
