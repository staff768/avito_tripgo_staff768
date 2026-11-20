package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func (r *TripRepository) AdvanceLastPosition(ctx context.Context, id uuid.UUID, at time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := r.db.Executor(ctx).Exec(ctx, `
		UPDATE trips
		SET last_position_at = GREATEST(last_position_at, $2)
		WHERE id = $1 AND status = 'active'`, id, at)
	if err != nil {
		return fmt.Errorf("update last position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if err := r.CheckExists(ctx, id); err != nil {
			return err
		}
		return domain.ErrTripCompleted
	}
	return nil
}

func (r *TripRepository) CheckExists(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var exists bool
	err := r.db.Executor(ctx).QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM trips WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check trip exists: %w", err)
	}
	if !exists {
		return domain.ErrTripNotFound
	}
	return nil
}

func (r *TripRepository) InsertPosition(ctx context.Context, position model.Position) (model.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	row := r.db.Executor(ctx).QueryRow(ctx, `
		INSERT INTO trip_positions (trip_id, latitude, longitude, recorded_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, trip_id, latitude, longitude, recorded_at`,
		position.TripID, position.Latitude, position.Longitude, position.RecordedAt)
	saved, err := scanPosition(row)
	if err != nil {
		return model.Position{}, fmt.Errorf("insert position: %w", err)
	}
	return saved, nil
}

func (r *TripRepository) ListPositions(ctx context.Context, id uuid.UUID) ([]model.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.db.Executor(ctx).Query(ctx, `
		SELECT id, trip_id, latitude, longitude, recorded_at
		FROM trip_positions
		WHERE trip_id = $1
		ORDER BY recorded_at, id`, id)
	if err != nil {
		return nil, fmt.Errorf("query trip positions: %w", err)
	}
	positions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Position, error) {
		return scanPosition(row)
	})
	if err != nil {
		return nil, fmt.Errorf("read trip positions: %w", err)
	}
	return positions, nil
}

func scanPosition(row pgx.Row) (model.Position, error) {
	var position model.Position
	err := row.Scan(&position.ID, &position.TripID, &position.Latitude, &position.Longitude, &position.RecordedAt)
	return position, err
}
