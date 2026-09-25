package repository

import (
	"errors"
	"fmt"
	"strings"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"gorm.io/gorm"
)

// LandParcelRepository owns persistence for the LandParcel aggregate.
type LandParcelRepository struct{ db *gorm.DB }

func (r *LandParcelRepository) Create(item *model.LandParcel) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create land parcel: %w", err)
	}
	return nil
}

func (r *LandParcelRepository) Get(id uint) (model.LandParcel, error) {
	var item model.LandParcel
	if err := r.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("get land parcel: %w", err)
	}
	return item, nil
}

func (r *LandParcelRepository) GetByCode(code string) (model.LandParcel, error) {
	var item model.LandParcel
	if err := r.db.Where("parcel_code = ?", code).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("get parcel by code: %w", err)
	}
	return item, nil
}

func (r *LandParcelRepository) List(q dto.ParcelQuery) ([]model.LandParcel, int64, error) {
	db := r.db.Model(&model.LandParcel{})
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("parcel_code LIKE ? OR name LIKE ? OR owner_org LIKE ?", like, like, like)
	}
	if q.State != "" {
		db = db.Where("parcel_state = ?", q.State)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count parcels: %w", err)
	}
	var items []model.LandParcel
	if err := db.Order("updated_at DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list parcels: %w", err)
	}
	return items, total, nil
}

func (r *LandParcelRepository) ListActiveByCoordinateSystem(coordinateSystem string, excludeID uint) ([]model.LandParcel, error) {
	var items []model.LandParcel
	err := r.db.Where("coordinate_system = ? AND parcel_state = ? AND id <> ?", coordinateSystem, "active", excludeID).Order("id ASC").Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list adjacent parcels: %w", err)
	}
	return items, nil
}

func (r *LandParcelRepository) Update(item model.LandParcel, version uint) error {
	updates := map[string]any{
		"name": item.Name, "boundary_geojson": item.BoundaryGeoJSON, "coordinate_system": item.CoordinateSystem,
		"owner_org": item.OwnerOrg, "parcel_state": item.ParcelState, "area_square_m": item.AreaSquareM,
		"boundary_version": gorm.Expr("boundary_version + 1"),
	}
	result := r.db.Model(&model.LandParcel{}).Where("id = ? AND boundary_version = ?", item.ID, version).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update parcel: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("parcel version changed: %w", gorm.ErrInvalidTransaction)
	}
	return nil
}
