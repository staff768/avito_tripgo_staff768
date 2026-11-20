package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type repository interface {
	AdvanceLastPosition(ctx context.Context, id uuid.UUID, at time.Time) error
	InsertPosition(ctx context.Context, position model.Position) (model.Position, error)
	CheckExists(ctx context.Context, id uuid.UUID) error
	ListPositions(ctx context.Context, id uuid.UUID) ([]model.Position, error)
}

type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}

type (
	queue interface {
		Cancel(ctx context.Context, id uuid.UUID)
	}
	observer interface{ PositionSaved(ctx context.Context) }
	clock    interface{ Now() time.Time }
)
