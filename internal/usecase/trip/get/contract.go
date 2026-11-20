//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package get_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type (
	tripRepository interface {
		GetByID(ctx context.Context, id uuid.UUID) (model.Trip, error)
	}
)
