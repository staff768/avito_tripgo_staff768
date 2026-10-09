package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/staff768/avito_tripgo_staff768/internal/app"
	"github.com/staff768/avito_tripgo_staff768/internal/config"
	"github.com/staff768/avito_tripgo_staff768/internal/postgres"
	"github.com/staff768/avito_tripgo_staff768/internal/repository"
	"github.com/staff768/avito_tripgo_staff768/internal/server"
	"github.com/staff768/avito_tripgo_staff768/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()

	pool, err := postgres.NewPool(connectCtx, cfg)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(connectCtx); err != nil {
		log.Fatalf("failed to ping db: %v", err)
	}

	txManager := postgres.NewTxManager(pool, cfg.DatabaseQueryTimeout)
	trips := repository.NewTripRepository(txManager, cfg.DatabaseQueryTimeout)
	history := repository.NewTripStatusHistoryRepository(txManager, cfg.DatabaseQueryTimeout)
	tripService := service.NewTripService(txManager, trips, history)

	a := app.New(pool, cfg.DatabaseQueryTimeout, tripService)
	srv := server.New(cfg, a.Routes())

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown timeout %s exceeded, forcing exit: %v", cfg.ShutdownTimeout, err)
	}
}
