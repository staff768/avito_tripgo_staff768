package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func (r *TripRepository) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error) {
	finished, err := r.finishActive(ctx, id, finishedAt)
	if err == nil {
		return finished, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, err
	}

	if _, err := r.GetByID(ctx, id); err != nil {
		return model.Trip{}, err
	}

	return model.Trip{}, domain.ErrTripCompleted
}

func (r *TripRepository) finishActive(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error) {
	query, args, err := builder.
		Update("trips").
		Set(statusColumn, StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": id, statusColumn: StatusActive}).
		Suffix(returningTrip).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build update trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	trip, err := scanTrip(r.db.Executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		return model.Trip{}, fmt.Errorf("update trip: %w", err)
	}

	return trip, nil
}
