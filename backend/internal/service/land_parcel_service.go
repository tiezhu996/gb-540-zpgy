package service

import (
	"errors"
	"strings"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/geometry"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
)

// CadastralService coordinates the entity-specific cadastral services over a
// single transaction-capable store. Entity operations live in their own files.
type CadastralService struct{ store *repository.Store }

func NewCadastralService(store *repository.Store) *CadastralService {
	return &CadastralService{store: store}
}

func (s *CadastralService) CreateParcel(req dto.CreateParcelRequest, actor Actor) (model.LandParcel, error) {
	if err := geometry.ValidateCoordinateSystem(req.CoordinateSystem); err != nil {
		return model.LandParcel{}, geoInvalid(err)
	}
	polygon, err := geometry.ParsePolygon(req.BoundaryGeoJSON)
	if err != nil {
		return model.LandParcel{}, geoInvalid(err)
	}
	state := req.ParcelState
	if state == "" {
		state = "active"
	}
	item := model.LandParcel{
		ParcelCode: strings.TrimSpace(req.ParcelCode), Name: strings.TrimSpace(req.Name), BoundaryGeoJSON: req.BoundaryGeoJSON,
		CoordinateSystem: strings.ToUpper(strings.TrimSpace(req.CoordinateSystem)), AreaSquareM: polygon.Area, BoundaryVersion: 1,
		OwnerOrg: strings.TrimSpace(req.OwnerOrg), ParcelState: state,
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		if _, findErr := tx.Parcels.GetByCode(item.ParcelCode); findErr == nil {
			return conflict("parcel code already exists", nil)
		} else if !errors.Is(findErr, repository.ErrNotFound) {
			return findErr
		}
		if createErr := tx.Parcels.Create(&item); createErr != nil {
			return createErr
		}
		return tx.Audits.Create(audit(actor, "parcel.created", "LandParcel", item.ID, &item.ID, "{}", snapshot(item)))
	})
	if err != nil {
		return item, wrapCadastral(err, "create parcel failed")
	}
	return item, nil
}

func (s *CadastralService) ListParcels(q dto.ParcelQuery) ([]model.LandParcel, dto.Pagination, error) {
	normalizePage(&q.Page, &q.PageSize)
	items, total, err := s.store.Parcels.List(q)
	if err != nil {
		return nil, dto.Pagination{}, internal("list parcels failed", err)
	}
	return items, dto.Pagination{Page: q.Page, PageSize: q.PageSize, Total: total}, nil
}

func (s *CadastralService) GetParcel(id uint) (model.LandParcel, error) {
	item, err := s.store.Parcels.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return item, notFound("parcel")
	}
	if err != nil {
		return item, internal("get parcel failed", err)
	}
	return item, nil
}

func (s *CadastralService) UpdateParcel(id uint, req dto.UpdateParcelRequest, actor Actor) (model.LandParcel, error) {
	before, err := s.store.Parcels.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return before, notFound("parcel")
	}
	if err != nil {
		return before, internal("get parcel failed", err)
	}
	version := before.BoundaryVersion
	if req.BoundaryVersion != nil {
		version = *req.BoundaryVersion
	}
	after := before
	if req.Name != nil {
		after.Name = strings.TrimSpace(*req.Name)
	}
	if req.OwnerOrg != nil {
		after.OwnerOrg = strings.TrimSpace(*req.OwnerOrg)
	}
	if req.ParcelState != nil {
		after.ParcelState = *req.ParcelState
	}
	if req.CoordinateSystem != nil {
		if validateErr := geometry.ValidateCoordinateSystem(*req.CoordinateSystem); validateErr != nil {
			return before, geoInvalid(validateErr)
		}
		after.CoordinateSystem = strings.ToUpper(strings.TrimSpace(*req.CoordinateSystem))
	}
	if req.BoundaryGeoJSON != nil {
		polygon, parseErr := geometry.ParsePolygon(*req.BoundaryGeoJSON)
		if parseErr != nil {
			return before, geoInvalid(parseErr)
		}
		after.BoundaryGeoJSON = *req.BoundaryGeoJSON
		after.AreaSquareM = polygon.Area
	}
	err = s.store.Transaction(func(tx *repository.Store) error {
		if updateErr := tx.Parcels.Update(after, version); updateErr != nil {
			return updateErr
		}
		return tx.Audits.Create(audit(actor, "parcel.updated", "LandParcel", id, &id, snapshot(before), snapshot(after)))
	})
	if err != nil {
		return before, wrapCadastral(err, "update parcel failed")
	}
	after.BoundaryVersion = version + 1
	after.UpdatedAt = time.Now()
	return after, nil
}
