package model

import "time"

// LandParcel is an offline cadastral boundary reference. It never represents
// a legal registration decision.
type LandParcel struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ParcelCode       string    `gorm:"size:64;not null;uniqueIndex" json:"parcel_code"`
	Name             string    `gorm:"size:160;not null" json:"name"`
	BoundaryGeoJSON  string    `gorm:"column:boundary_geojson;type:text;not null" json:"boundary_geojson"`
	CoordinateSystem string    `gorm:"size:40;not null" json:"coordinate_system"`
	AreaSquareM      float64   `gorm:"not null" json:"area_square_m"`
	BoundaryVersion  uint      `gorm:"not null;default:1" json:"boundary_version"`
	OwnerOrg         string    `gorm:"size:160" json:"owner_org"`
	ParcelState      string    `gorm:"size:24;not null;default:active" json:"parcel_state"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
