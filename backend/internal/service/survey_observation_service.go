package service

import (
	"errors"
	"strings"

	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/geometry"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
)

func (s *CadastralService) ImportObservation(req dto.ImportObservationRequest, actor Actor) (model.SurveyObservation, error) {
	if _, err := s.store.Parcels.Get(req.ParcelID); errors.Is(err, repository.ErrNotFound) {
		return model.SurveyObservation{}, notFound("parcel")
	} else if err != nil {
		return model.SurveyObservation{}, internal("load parcel failed", err)
	}
	if _, err := geometry.ParsePoint(req.PointGeoJSON); err != nil {
		return model.SurveyObservation{}, geoInvalid(err)
	}
	state := req.ObservationState
	if state == "" {
		state = "accepted"
	}
	item := model.SurveyObservation{
		ParcelID: req.ParcelID, ObservationCode: strings.TrimSpace(req.ObservationCode), PointGeoJSON: req.PointGeoJSON,
		ObservedAt: req.ObservedAt, Method: strings.TrimSpace(req.Method), HorizontalAccuracyM: req.HorizontalAccuracyM,
		SourceChecksum: strings.TrimSpace(req.SourceChecksum), ObservationState: state, QualityNote: strings.TrimSpace(req.QualityNote), ImportedBy: actor.ID,
	}
	err := s.store.Transaction(func(tx *repository.Store) error {
		if createErr := tx.Observations.Create(&item); createErr != nil {
			return createErr
		}
		return tx.Audits.Create(audit(actor, "observation.imported", "SurveyObservation", item.ID, &item.ParcelID, "{}", snapshot(item)))
	})
	if err != nil {
		return item, wrapCadastral(err, "import observation failed")
	}
	return item, nil
}

func (s *CadastralService) ListObservations(q dto.ObservationQuery) ([]model.SurveyObservation, dto.Pagination, error) {
	normalizePage(&q.Page, &q.PageSize)
	items, total, err := s.store.Observations.List(q)
	if err != nil {
		return nil, dto.Pagination{}, internal("list observations failed", err)
	}
	return items, dto.Pagination{Page: q.Page, PageSize: q.PageSize, Total: total}, nil
}

func (s *CadastralService) GetObservation(id uint) (model.SurveyObservation, error) {
	item, err := s.store.Observations.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("observation")
	}
	if err != nil {
		return item, internal("get observation failed", err)
	}
	return item, nil
}

func (s *CadastralService) TransitionObservation(id uint, req dto.ObservationTransitionRequest, actor Actor) (model.SurveyObservation, error) {
	item, err := s.store.Observations.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("observation")
	}
	if err != nil {
		return item, internal("get observation failed", err)
	}
	if item.Version != req.Version {
		return item, conflict("observation version does not match the current record", nil)
	}
	to := strings.TrimSpace(req.To)
	if !canTransitionObservation(item.ObservationState, to) {
		return item, conflict("observation state transition is not allowed", nil)
	}
	if err := requireAnyRole(actor, constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin); err != nil {
		return item, err
	}
	if to == "superseded" {
		if req.ReplacementObservationID == nil || *req.ReplacementObservationID == item.ID {
			return item, invalid("a different replacement_observation_id is required when superseding an observation", nil)
		}
		replacement, replacementErr := s.store.Observations.Get(*req.ReplacementObservationID)
		if errors.Is(replacementErr, repository.ErrNotFound) {
			return item, notFound("replacement observation")
		}
		if replacementErr != nil {
			return item, internal("load replacement observation failed", replacementErr)
		}
		if replacement.ParcelID != item.ParcelID || replacement.ObservationState == "superseded" {
			return item, conflict("replacement observation must be an active observation on the same parcel", nil)
		}
	} else if req.ReplacementObservationID != nil {
		return item, invalid("replacement_observation_id is only valid for superseded observations", nil)
	}

	before := item
	err = s.store.Transaction(func(tx *repository.Store) error {
		if transitionErr := tx.Observations.Transition(id, item.Version, to, req.ReplacementObservationID, strings.TrimSpace(req.QualityNote)); transitionErr != nil {
			return transitionErr
		}
		after := map[string]any{"observation_state": to, "version": item.Version + 1, "replaced_by": req.ReplacementObservationID, "quality_note": strings.TrimSpace(req.QualityNote)}
		return tx.Audits.Create(audit(actor, "observation.state_changed", "SurveyObservation", id, &item.ParcelID, snapshot(before), snapshot(after)))
	})
	if err != nil {
		return item, conflict("observation changed while transitioning", err)
	}
	item.ObservationState = to
	item.Version++
	item.ReplacedBy = req.ReplacementObservationID
	if note := strings.TrimSpace(req.QualityNote); note != "" {
		item.QualityNote = note
	}
	return item, nil
}
