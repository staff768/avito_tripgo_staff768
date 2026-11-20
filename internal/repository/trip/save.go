package trip

import (
	"context"
	"fmt"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func (r *TripRepository) Create(ctx context.Context, trip model.Trip) (model.Trip, error) {
	query, args, err := builder.
		Insert("trips").
		Columns(insertTripColumns...).
		Values(
			trip.ID,
			trip.UserID,
			trip.DriverID,
			trip.StartLatitude,
			trip.StartLongitude,
			trip.EndLatitude,
			trip.EndLongitude,
			trip.Price,
			trip.Status,
			trip.StartedAt,
			trip.FinishedAt,
		).
		Suffix(returningTrip).
		ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build insert trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	created, err := scanTrip(r.db.Executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if isUniqueViolation(err, driverActiveIndex) {
			return model.Trip{}, domain.ErrDriverBusy
		}
		return model.Trip{}, fmt.Errorf("insert trip: %w", err)
	}

	return created, nil
}
