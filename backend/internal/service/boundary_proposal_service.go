package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/geometry"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
)

func (s *CadastralService) CreateProposal(req dto.CreateProposalRequest, actor Actor) (model.BoundaryProposal, error) {
	parcel, err := s.store.Parcels.Get(req.ParcelID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.BoundaryProposal{}, notFound("parcel")
	}
	if err != nil {
		return model.BoundaryProposal{}, internal("load parcel failed", err)
	}
	if req.BaseVersion != parcel.BoundaryVersion {
		return model.BoundaryProposal{}, conflict("proposal base version does not match the parcel boundary", nil)
	}
	proposed, parseErr := geometry.ParsePolygon(req.ProposedGeoJSON)
	if parseErr != nil {
		return model.BoundaryProposal{}, geoInvalid(parseErr)
	}
	for _, id := range req.ObservationIDs {
		obs, obsErr := s.store.Observations.Get(id)
		if obsErr != nil || obs.ParcelID != req.ParcelID {
			return model.BoundaryProposal{}, invalid("all observations must belong to the selected parcel", obsErr)
		}
	}
	obsJSON, err := json.Marshal(req.ObservationIDs)
	if err != nil {
		return model.BoundaryProposal{}, internal("encode proposal observations failed", err)
	}
	item := model.BoundaryProposal{
		ParcelID: req.ParcelID, BaseVersion: req.BaseVersion, ProposedGeoJSON: req.ProposedGeoJSON, ObservationIDs: string(obsJSON),
		SnapToleranceM: req.SnapToleranceM, AreaDeltaSquareM: proposed.Area - parcel.AreaSquareM, ProposalState: constants.ProposalDraft,
		Rationale: strings.TrimSpace(req.Rationale), Version: 1, CreatedBy: actor.ID,
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		if createErr := tx.Proposals.Create(&item); createErr != nil {
			return createErr
		}
		return tx.Audits.Create(audit(actor, "proposal.created", "BoundaryProposal", item.ID, &item.ParcelID, "{}", snapshot(item)))
	})
	if err != nil {
		return item, wrapCadastral(err, "create proposal failed")
	}
	return item, nil
}

func (s *CadastralService) ListProposals(q dto.ProposalQuery) ([]model.BoundaryProposal, dto.Pagination, error) {
	normalizePage(&q.Page, &q.PageSize)
	items, total, err := s.store.Proposals.List(q)
	if err != nil {
		return nil, dto.Pagination{}, internal("list proposals failed", err)
	}
	return items, dto.Pagination{Page: q.Page, PageSize: q.PageSize, Total: total}, nil
}

func (s *CadastralService) GetProposal(id uint) (model.BoundaryProposal, error) {
	item, err := s.store.Proposals.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("proposal")
	}
	if err != nil {
		return item, internal("get proposal failed", err)
	}
	return item, nil
}

func (s *CadastralService) TransitionProposal(id uint, req dto.ProposalTransitionRequest, actor Actor) (model.BoundaryProposal, error) {
	item, err := s.store.Proposals.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("proposal")
	}
	if err != nil {
		return item, internal("get proposal failed", err)
	}
	to := constants.ProposalState(req.To)
	if !to.Valid() || !constants.CanProposalTransition(item.ProposalState, to) {
		return item, conflict("proposal state transition is not allowed", nil)
	}
	if err := authorizeProposalTransition(item, to, actor); err != nil {
		return item, err
	}
	updates := map[string]any{}
	if req.Rationale != "" {
		updates["rationale"] = req.Rationale
	}
	if isProposalReviewTransition(to) {
		updates["reviewed_by"] = actor.ID
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		if transitionErr := tx.Proposals.Transition(id, item.Version, item.ProposalState, to, updates); transitionErr != nil {
			return transitionErr
		}
		return tx.Audits.Create(audit(actor, "proposal.state_changed", "BoundaryProposal", id, &item.ParcelID, snapshot(item), snapshot(map[string]any{"state": to, "version": item.Version + 1})))
	})
	if err != nil {
		return item, conflict("proposal changed while transitioning", err)
	}
	item.ProposalState = to
	item.Version++
	if isProposalReviewTransition(to) {
		item.ReviewedBy = &actor.ID
	}
	return item, nil
}

func authorizeProposalTransition(item model.BoundaryProposal, to constants.ProposalState, actor Actor) error {
	isAuthor := actor.ID == item.CreatedBy
	switch to {
	case constants.ProposalValidated, constants.ProposalSubmitted, constants.ProposalDraft:
		if isAuthor || actor.Role == constants.RoleAdmin {
			return nil
		}
		return &AppError{CodeForbidden, http.StatusForbidden, "only the proposal author or an administrator may advance this authoring transition", nil}
	case constants.ProposalReviewed, constants.ProposalAccepted, constants.ProposalRejected, constants.ProposalRevision:
		if actor.Role != constants.RoleReviewer && actor.Role != constants.RoleAdmin {
			return &AppError{CodeForbidden, http.StatusForbidden, "only a reviewer or administrator may perform review transitions", nil}
		}
		if isAuthor {
			return &AppError{CodeForbidden, http.StatusForbidden, "proposal author cannot review their own proposal", nil}
		}
		return nil
	default:
		return conflict("proposal state transition is not allowed", nil)
	}
}

func isProposalReviewTransition(to constants.ProposalState) bool {
	return to == constants.ProposalReviewed || to == constants.ProposalAccepted || to == constants.ProposalRejected || to == constants.ProposalRevision
}
