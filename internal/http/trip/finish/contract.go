//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type usecase interface {
	Execute(ctx context.Context, id uuid.UUID) (model.Trip, error)
}
