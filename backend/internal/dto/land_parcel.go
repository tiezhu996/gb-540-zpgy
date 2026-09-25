package dto

type ParcelQuery struct {
	Keyword  string
	State    string
	Page     int
	PageSize int
}

type CreateParcelRequest struct {
	ParcelCode       string `json:"parcel_code" validate:"required,min=2,max=64"`
	Name             string `json:"name" validate:"required,min=2,max=160"`
	BoundaryGeoJSON  string `json:"boundary_geojson" validate:"required"`
	CoordinateSystem string `json:"coordinate_system" validate:"required"`
	OwnerOrg         string `json:"owner_org" validate:"max=160"`
	ParcelState      string `json:"parcel_state" validate:"omitempty,oneof=active archived review"`
}

type UpdateParcelRequest struct {
	Name             *string `json:"name" validate:"omitempty,min=2,max=160"`
	BoundaryGeoJSON  *string `json:"boundary_geojson"`
	CoordinateSystem *string `json:"coordinate_system"`
	OwnerOrg         *string `json:"owner_org" validate:"omitempty,max=160"`
	ParcelState      *string `json:"parcel_state" validate:"omitempty,oneof=active archived review"`
	BoundaryVersion  *uint   `json:"boundary_version" validate:"omitempty,gt=0"`
}
