package model

import (
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
)

// TopologyConflict is an immutable detection result for a proposal snapshot.
type TopologyConflict struct {
	ID                      uint                   `gorm:"primaryKey" json:"id"`
	ProposalID              uint                   `gorm:"not null;index" json:"proposal_id"`
	ParcelIDs               string                 `gorm:"type:text;not null" json:"parcel_ids"`
	ConflictType            constants.ConflictType `gorm:"size:32;not null;index" json:"conflict_type"`
	GeometryGeoJSON         string                 `gorm:"column:geometry_geojson;type:text" json:"geometry_geojson"`
	MagnitudeSquareM        float64                `gorm:"not null" json:"magnitude_square_m"`
	Severity                string                 `gorm:"size:16;not null" json:"severity"`
	AlgorithmVersion        string                 `gorm:"size:40;not null" json:"algorithm_version"`
	InputHash               string                 `gorm:"size:128;not null;index" json:"input_hash"`
	ConflictState           string                 `gorm:"size:32;not null;index" json:"conflict_state"`
	SuggestedResolutionJSON string                 `gorm:"type:text" json:"suggested_resolution_json"`
	Explanation             string                 `gorm:"size:2000" json:"explanation"`
	DetectedAt              time.Time              `gorm:"not null" json:"detected_at"`
	ResolvedBy              *uint                  `json:"resolved_by"`
}

// TopologyDetectionRun binds an actor and Idempotency-Key to immutable
// detection results so an interrupted client can replay the same operation.
type TopologyDetectionRun struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProposalID     uint      `gorm:"not null;index" json:"proposal_id"`
	ActorID        uint      `gorm:"not null;uniqueIndex:idx_detection_actor_key" json:"actor_id"`
	IdempotencyKey string    `gorm:"size:128;not null;uniqueIndex:idx_detection_actor_key" json:"idempotency_key"`
	RequestHash    string    `gorm:"size:128;not null" json:"request_hash"`
	InputHash      string    `gorm:"size:128;not null;index" json:"input_hash"`
	ResultIDs      string    `gorm:"type:text;not null" json:"result_ids"`
	CreatedAt      time.Time `json:"created_at"`
}
