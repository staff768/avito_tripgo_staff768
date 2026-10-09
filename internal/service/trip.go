package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/repository"
)

const (
	reasonTripCreated  = "trip created"
	reasonTripFinished = "trip finished"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type CreateTripParams struct {
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
}

type TripService struct {
	tx      TxManager
	trips   *repository.TripRepository
	history *repository.TripStatusHistoryRepository
}

func NewTripService(
	tx TxManager,
	trips *repository.TripRepository,
	history *repository.TripStatusHistoryRepository,
) *TripService {
	return &TripService{tx: tx, trips: trips, history: history}
}

func (s *TripService) CreateTrip(ctx context.Context, params CreateTripParams) (repository.Trip, error) {
	trip := repository.Trip{
		ID:             uuid.New(),
		UserID:         params.UserID,
		DriverID:       params.DriverID,
		StartLatitude:  params.StartLatitude,
		StartLongitude: params.StartLongitude,
		EndLatitude:    params.EndLatitude,
		EndLongitude:   params.EndLongitude,
		Price:          params.Price,
		Status:         repository.StatusActive,
		StartedAt:      time.Now().UTC(),
	}

	reason := reasonTripCreated

	var created repository.Trip

	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error

		created, err = s.trips.Create(ctx, trip)
		if err != nil {
			return err
		}

		return s.history.Add(ctx, repository.StatusChange{
			TripID:    created.ID,
			ToStatus:  created.Status,
			Reason:    &reason,
			ChangedAt: created.StartedAt,
		})
	})
	if err != nil {
		return repository.Trip{}, err
	}

	return created, nil
}

func (s *TripService) GetTrip(ctx context.Context, id uuid.UUID) (repository.Trip, error) {
	return s.trips.GetByID(ctx, id)
}

func (s *TripService) FinishTrip(ctx context.Context, id uuid.UUID) (repository.Trip, error) {
	finishedAt := time.Now().UTC()
	fromStatus := repository.StatusActive
	reason := reasonTripFinished

	var finished repository.Trip

	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error

		finished, err = s.trips.Finish(ctx, id, finishedAt)
		if err != nil {
			return err
		}

		return s.history.Add(ctx, repository.StatusChange{
			TripID:     finished.ID,
			FromStatus: &fromStatus,
			ToStatus:   finished.Status,
			Reason:     &reason,
			ChangedAt:  finishedAt,
		})
	})
	if err != nil {
		return repository.Trip{}, err
	}

	return finished, nil
}
