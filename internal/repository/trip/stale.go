package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

const staleTripsSQL = `
	SELECT id, driver_id, COALESCE(last_position_at, started_at)
	FROM trips
	WHERE status = 'active'
	  AND COALESCE(last_position_at, started_at) < now() - $1::interval`

const staleTripsOrder = `
	ORDER BY COALESCE(last_position_at, started_at), id`

const firstStaleTripsSQL = staleTripsSQL + staleTripsOrder + `
	LIMIT $2`

const nextStaleTripsSQL = staleTripsSQL + `
	  AND (COALESCE(last_position_at, started_at), id) > ($2, $3)` + staleTripsOrder + `
	LIMIT $4`

func (r *TripRepository) ListStaleTrips(ctx context.Context, staleAfter time.Duration, limit int, after *model.StaleTripCursor) ([]model.StaleTrip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args := firstStaleTripsSQL, []any{staleAfter, limit}
	if after != nil {
		query, args = nextStaleTripsSQL, []any{staleAfter, after.LastActivityAt, after.ID, limit}
	}
	rows, err := r.db.Executor(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query stale trips: %w", err)
	}
	trips, err := pgx.CollectRows(rows, pgx.RowToStructByPos[model.StaleTrip])
	if err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	return trips, nil
}
