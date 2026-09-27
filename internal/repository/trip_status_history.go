package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/postgres"
)

type StatusChange struct {
	TripID     uuid.UUID
	FromStatus *string
	ToStatus   string
	Reason     *string
	ChangedAt  time.Time
}

type TripStatusHistoryRepository struct {
	db           postgres.Executor
	queryTimeout time.Duration
}

func NewTripStatusHistoryRepository(db postgres.Executor, queryTimeout time.Duration) *TripStatusHistoryRepository {
	return &TripStatusHistoryRepository{db: db, queryTimeout: queryTimeout}
}

func (r *TripStatusHistoryRepository) Add(ctx context.Context, change StatusChange) error {
	columns := []string{"trip_id", "from_status", "to_status", "reason"}
	values := []any{change.TripID, change.FromStatus, change.ToStatus, change.Reason}

	if !change.ChangedAt.IsZero() {
		columns = append(columns, "changed_at")
		values = append(values, change.ChangedAt)
	}

	query, args, err := builder.
		Insert("trip_status_history").
		Columns(columns...).
		Values(values...).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip status history: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := r.db.Executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}

	return nil
}
