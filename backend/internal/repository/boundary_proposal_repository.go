package repository

import (
	"errors"
	"fmt"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"gorm.io/gorm"
)

// BoundaryProposalRepository persists versioned internal proposals.
type BoundaryProposalRepository struct{ db *gorm.DB }

func (r *BoundaryProposalRepository) Create(item *model.BoundaryProposal) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create boundary proposal: %w", err)
	}
	return nil
}

func (r *BoundaryProposalRepository) Get(id uint) (model.BoundaryProposal, error) {
	var item model.BoundaryProposal
	if err := r.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, fmt.Errorf("get proposal: %w", err)
	}
	return item, nil
}

func (r *BoundaryProposalRepository) List(q dto.ProposalQuery) ([]model.BoundaryProposal, int64, error) {
	db := r.db.Model(&model.BoundaryProposal{})
	if q.ParcelID != nil {
		db = db.Where("parcel_id = ?", *q.ParcelID)
	}
	if q.State != "" {
		db = db.Where("proposal_state = ?", q.State)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count proposals: %w", err)
	}
	var items []model.BoundaryProposal
	if err := db.Order("updated_at DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list proposals: %w", err)
	}
	return items, total, nil
}

func (r *BoundaryProposalRepository) Transition(id, version uint, from, to constants.ProposalState, updates map[string]any) error {
	updates["proposal_state"] = to
	updates["version"] = gorm.Expr("version + 1")
	result := r.db.Model(&model.BoundaryProposal{}).Where("id = ? AND version = ? AND proposal_state = ?", id, version, from).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transition proposal: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("proposal state or version changed: %w", gorm.ErrInvalidTransaction)
	}
	return nil
}
