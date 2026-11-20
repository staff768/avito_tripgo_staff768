//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"
	"time"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type (
	tripRepository interface {
		Create(ctx context.Context, trip model.Trip) (model.Trip, error)
	}
	txManager interface {
		Do(ctx context.Context, fn func(context.Context) error) error
	}
	historyRepository interface {
		Add(ctx context.Context, change model.StatusChange) error
	}
	observer interface {
		Created(ctx context.Context)
	}
	clock interface{ Now() time.Time }
)
