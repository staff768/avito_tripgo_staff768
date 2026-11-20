package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/staff768/avito_tripgo_staff768/internal/config"
	"github.com/staff768/avito_tripgo_staff768/internal/container"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/telemetry"
)

func main() {
	if err := run(); err != nil {
		telemetry.NewLogger(os.Stdout, slog.LevelError, "trip-service").Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- app.Server.ListenAndServe() }()
	app.Logger.InfoContext(ctx, "service started", "address", cfg.HTTPAddr)
	select {
	case <-ctx.Done():
	case err = <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if closeErr := app.Close(shutdownCtx); closeErr != nil {
		return errors.Join(err, fmt.Errorf("shutdown within %s: %w", cfg.ShutdownTimeout, closeErr))
	}
	app.Logger.InfoContext(shutdownCtx, "service stopped")
	return err
}
