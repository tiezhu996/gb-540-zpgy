package repository

import (
	"errors"
	"fmt"
	"strings"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"gorm.io/gorm"
)

// SurveyObservationRepository persists survey evidence independently of proposals.
type SurveyObservationRepository struct{ db *gorm.DB }

func (r *SurveyObservationRepository) Create(item *model.SurveyObservation) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create survey observation: %w", err)
	}
	return nil
}

func (r *SurveyObservationRepository) Get(id uint) (model.SurveyObservation, error) {
	var item model.SurveyObservation
	if err := r.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("get observation: %w", err)
	}
	return item, nil
}

func (r *SurveyObservationRepository) List(q dto.ObservationQuery) ([]model.SurveyObservation, int64, error) {
	db := r.db.Model(&model.SurveyObservation{})
	if q.ParcelID != nil {
		db = db.Where("parcel_id = ?", *q.ParcelID)
	}
	if q.State != "" {
		db = db.Where("observation_state = ?", q.State)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count observations: %w", err)
	}
	var items []model.SurveyObservation
	if err := db.Order("observed_at DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list observations: %w", err)
	}
	return items, total, nil
}

func (r *SurveyObservationRepository) Transition(id, version uint, to string, replacementID *uint, note string) error {
	updates := map[string]any{"observation_state": to, "version": gorm.Expr("version + 1")}
	if replacementID != nil {
		updates["replaced_by"] = replacementID
	}
	if strings.TrimSpace(note) != "" {
		updates["quality_note"] = note
	}
	result := r.db.Model(&model.SurveyObservation{}).Where("id = ? AND version = ?", id, version).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transition survey observation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("observation version changed: %w", gorm.ErrInvalidTransaction)
	}
	return nil
}
