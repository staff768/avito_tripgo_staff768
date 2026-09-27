package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/staff768/avito_tripgo_staff768/internal/postgres"
)

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

const driverActiveIndex = "trips_driver_active_uidx"

type Trip struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

var tripColumns = []string{
	"id",
	"user_id",
	"driver_id",
	"start_latitude",
	"start_longitude",
	"end_latitude",
	"end_longitude",
	"price",
	"status",
	"started_at",
	"finished_at",
	"created_at",
	"updated_at",
}

var insertTripColumns = []string{
	"id",
	"user_id",
	"driver_id",
	"start_latitude",
	"start_longitude",
	"end_latitude",
	"end_longitude",
	"price",
	"status",
	"started_at",
	"finished_at",
}

var returningTrip = "RETURNING " + strings.Join(tripColumns, ", ")

type TripRepository struct {
	db           postgres.Executor
	queryTimeout time.Duration
}

func NewTripRepository(db postgres.Executor, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{db: db, queryTimeout: queryTimeout}
}

func (r *TripRepository) Create(ctx context.Context, trip Trip) (Trip, error) {
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
		return Trip{}, fmt.Errorf("build insert trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	created, err := scanTrip(r.db.Executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if isUniqueViolation(err, driverActiveIndex) {
			return Trip{}, ErrDriverBusy
		}
		return Trip{}, fmt.Errorf("insert trip: %w", err)
	}

	return created, nil
}

func (r *TripRepository) GetByID(ctx context.Context, id uuid.UUID) (Trip, error) {
	query, args, err := builder.
		Select(tripColumns...).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	trip, err := scanTrip(r.db.Executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Trip{}, ErrTripNotFound
		}
		return Trip{}, fmt.Errorf("select trip: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (Trip, error) {
	finished, err := r.finishActive(ctx, id, finishedAt)
	if err == nil {
		return finished, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, err
	}

	if _, err := r.GetByID(ctx, id); err != nil {
		return Trip{}, err
	}

	return Trip{}, ErrTripCompleted
}

func (r *TripRepository) finishActive(ctx context.Context, id uuid.UUID, finishedAt time.Time) (Trip, error) {
	query, args, err := builder.
		Update("trips").
		Set("status", StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": id, "status": StatusActive}).
		Suffix(returningTrip).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build update trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	trip, err := scanTrip(r.db.Executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		return Trip{}, fmt.Errorf("update trip: %w", err)
	}

	return trip, nil
}

func scanTrip(row pgx.Row) (Trip, error) {
	var trip Trip
	err := row.Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartLatitude,
		&trip.StartLongitude,
		&trip.EndLatitude,
		&trip.EndLongitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
		&trip.CreatedAt,
		&trip.UpdatedAt,
	)
	if err != nil {
		return Trip{}, err
	}

	return trip, nil
}
