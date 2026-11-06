package trip

import (
	"context"

	"github.com/staff768/avito_tripgo_staff768/internal/infra/postgres"
)

type executor interface {
	Executor(ctx context.Context) postgres.Querier
}
