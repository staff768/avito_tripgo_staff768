package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/staff768/avito_tripgo_staff768/internal/config"
	transport "github.com/staff768/avito_tripgo_staff768/internal/http"
	"github.com/staff768/avito_tripgo_staff768/internal/http/health"
	"github.com/staff768/avito_tripgo_staff768/internal/http/middleware"
	createhttp "github.com/staff768/avito_tripgo_staff768/internal/http/trip/create"
	finishhttp "github.com/staff768/avito_tripgo_staff768/internal/http/trip/finish"
	gethttp "github.com/staff768/avito_tripgo_staff768/internal/http/trip/get"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/clock"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/postgres"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/postgres/transaction"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/telemetry"
	repository "github.com/staff768/avito_tripgo_staff768/internal/repository/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/server"
	"github.com/staff768/avito_tripgo_staff768/internal/usecase/trip/create"
	"github.com/staff768/avito_tripgo_staff768/internal/usecase/trip/finish"
	"github.com/staff768/avito_tripgo_staff768/internal/usecase/trip/get"
)

type Container struct {
	Server    *http.Server
	Logger    *slog.Logger
	pool      *pgxpool.Pool
	telemetry *telemetry.Telemetry
}

func New(ctx context.Context, cfg *config.Config) (_ *Container, resultErr error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return nil, err
	}
	logger := telemetry.NewLogger(os.Stdout, level, cfg.OTelServiceName)
	signals, err := telemetry.New(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, signals.Shutdown(ctx))
		}
	}()
	tracer, err := postgres.NewTracer(signals.Tracer, signals.Meter)
	if err != nil {
		return nil, err
	}
	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	pool, err := postgres.NewPool(connectCtx, cfg, tracer)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			pool.Close()
		}
	}()
	if err := pool.Ping(connectCtx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	tx := transaction.New(pool, cfg.DatabaseQueryTimeout)
	trips := repository.NewTripRepository(tx, cfg.DatabaseQueryTimeout)
	history := repository.NewTripStatusHistoryRepository(tx, cfg.DatabaseQueryTimeout)
	if err := signals.RegisterActiveTrips(trips); err != nil {
		return nil, err
	}
	createUC := create.New(tx, trips, history, clock.Clock{}, signals, logger)
	getUC := get.New(trips)
	finishUC := finish.New(tx, trips, history, clock.Clock{}, signals, logger)
	router := transport.New(createhttp.New(createUC, logger), gethttp.New(getUC, logger), finishhttp.New(finishUC, logger), health.New(pool, cfg.DatabaseQueryTimeout, logger), logger)
	stack, err := middleware.New(logger, signals.Tracer, signals.Meter, signals.Propagator)
	if err != nil {
		return nil, err
	}
	return &Container{Server: server.New(cfg, stack.Wrap(router)), Logger: logger, pool: pool, telemetry: signals}, nil
}

func (c *Container) Close(ctx context.Context) error {
	shutdownErr := c.Server.Shutdown(ctx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, c.Server.Close())
	}
	telemetryErr := c.telemetry.Shutdown(ctx)
	closed := make(chan struct{})
	go func() { c.pool.Close(); close(closed) }()
	select {
	case <-closed:
	case <-ctx.Done():
		return errors.Join(shutdownErr, telemetryErr, ctx.Err())
	}
	return errors.Join(shutdownErr, telemetryErr)
}
