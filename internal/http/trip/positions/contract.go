//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package positions
package positions

import (
	"context"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type usecase interface {
	Save(ctx context.Context, position model.Position) (model.Position, error)
	List(ctx context.Context, id uuid.UUID) ([]model.Position, error)
}
