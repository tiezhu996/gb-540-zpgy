package model

import "time"

// SurveyObservation retains raw point evidence imported for one parcel.
type SurveyObservation struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	ParcelID            uint      `gorm:"not null;index" json:"parcel_id"`
	ObservationCode     string    `gorm:"size:80;not null;uniqueIndex" json:"observation_code"`
	PointGeoJSON        string    `gorm:"column:point_geojson;type:text;not null" json:"point_geojson"`
	ObservedAt          time.Time `gorm:"not null" json:"observed_at"`
	Method              string    `gorm:"size:48;not null" json:"method"`
	HorizontalAccuracyM float64   `gorm:"not null" json:"horizontal_accuracy_m"`
	SourceChecksum      string    `gorm:"size:128;not null" json:"source_checksum"`
	ObservationState    string    `gorm:"size:24;not null;default:accepted" json:"observation_state"`
	Version             uint      `gorm:"not null;default:1" json:"version"`
	ReplacedBy          *uint     `json:"replaced_by"`
	QualityNote         string    `gorm:"size:1000" json:"quality_note"`
	ImportedBy          uint      `gorm:"not null" json:"imported_by"`
}
