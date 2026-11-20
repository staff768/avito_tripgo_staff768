package finish

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

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	var result model.Trip
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		current, err := u.trips.GetByID(ctx, id)
		if err != nil {
			return fmt.Errorf("fetch trip: %w", err)
		}
		entity := domain.Restore(current)
		if err := entity.Finish(u.clock.Now()); err != nil {
			return err
		}
		result, err = u.trips.Finish(ctx, id, *entity.Model().FinishedAt)
		if err != nil {
			return fmt.Errorf("save finished trip: %w", err)
		}
		from, reason := domain.StatusActive, "trip finished"
		return u.history.Add(ctx, model.StatusChange{TripID: result.ID, FromStatus: &from, ToStatus: result.Status, Reason: &reason, ChangedAt: *result.FinishedAt})
	})
	if err != nil {
		return model.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	u.observer.Completed(ctx)
	u.logger.InfoContext(ctx, "trip completed", "trip_id", result.ID.String())
	return result, nil
}
