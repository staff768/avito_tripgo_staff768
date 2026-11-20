package create

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Usecase struct {
	tx       txManager
	trips    tripRepository
	history  historyRepository
	clock    clock
	observer observer
	logger   *slog.Logger
}

func New(tx txManager, trips tripRepository, history historyRepository, clock clock, observer observer, logger *slog.Logger) *Usecase {
	return &Usecase{tx: tx, trips: trips, history: history, clock: clock, observer: observer, logger: logger}
}

func (u *Usecase) Execute(ctx context.Context, params model.CreateTripParams) (model.Trip, error) {
	entity, err := domain.New(params, uuid.New(), u.clock.Now())
	if err != nil {
		return model.Trip{}, err
	}
	var result model.Trip
	err = u.tx.Do(ctx, func(ctx context.Context) error {
		var saveErr error
		result, saveErr = u.trips.Create(ctx, entity.Model())
		if saveErr != nil {
			return fmt.Errorf("save trip: %w", saveErr)
		}
		reason := "trip created"
		return u.history.Add(ctx, model.StatusChange{TripID: result.ID, ToStatus: result.Status, Reason: &reason, ChangedAt: result.StartedAt})
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("create trip: %w", err)
	}
	u.observer.Created(ctx)
	u.logger.InfoContext(ctx, "trip created", "trip_id", result.ID.String())
	return result, nil
}
