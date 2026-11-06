//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type (
	tripRepository interface {
		GetByID(ctx context.Context, id uuid.UUID) (model.Trip, error)
		Finish(ctx context.Context, id uuid.UUID, at time.Time) (model.Trip, error)
	}
	txManager interface {
		Do(ctx context.Context, fn func(context.Context) error) error
	}
	historyRepository interface {
		Add(ctx context.Context, change model.StatusChange) error
	}
	observer interface {
		Completed(ctx context.Context)
	}
	clock interface{ Now() time.Time }
)
