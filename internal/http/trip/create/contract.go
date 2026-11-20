//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type usecase interface {
	Execute(ctx context.Context, params model.CreateTripParams) (model.Trip, error)
}
