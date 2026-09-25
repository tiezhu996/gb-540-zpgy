package dto

import "time"

type ObservationQuery struct {
	ParcelID *uint
	State    string
	Page     int
	PageSize int
}

type ImportObservationRequest struct {
	ParcelID            uint      `json:"parcel_id" validate:"required,gt=0"`
	ObservationCode     string    `json:"observation_code" validate:"required,min=2,max=80"`
	PointGeoJSON        string    `json:"point_geojson" validate:"required"`
	ObservedAt          time.Time `json:"observed_at" validate:"required"`
	Method              string    `json:"method" validate:"required,min=2,max=48"`
	HorizontalAccuracyM float64   `json:"horizontal_accuracy_m" validate:"required,gt=0,lte=1000"`
	SourceChecksum      string    `json:"source_checksum" validate:"required,min=8,max=128"`
	ObservationState    string    `json:"observation_state" validate:"omitempty,oneof=accepted rejected superseded"`
	QualityNote         string    `json:"quality_note" validate:"max=1000"`
}

type ObservationTransitionRequest struct {
	To                       string `json:"to" validate:"required,oneof=accepted rejected superseded"`
	Version                  uint   `json:"version" validate:"required,gt=0"`
	ReplacementObservationID *uint  `json:"replacement_observation_id" validate:"omitempty,gt=0"`
	QualityNote              string `json:"quality_note" validate:"max=1000"`
}
