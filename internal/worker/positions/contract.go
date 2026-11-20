//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package positions
package positions

import (
	"context"
	"time"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type staleTrips interface {
	ListStaleTrips(ctx context.Context, staleAfter time.Duration, limit int, after *model.StaleTripCursor) ([]model.StaleTrip, error)
}

type pushGateway interface {
	RequestPosition(ctx context.Context, trip model.StaleTrip) error
}

type Ticker interface {
	Ticks() <-chan time.Time
	Stop()
}
