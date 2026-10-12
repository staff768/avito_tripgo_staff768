package app

import (
	"context"
	"time"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/service"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type App struct {
	api.Unimplemented

	db        Pinger
	dbTimeout time.Duration
	trips     *service.TripService
}

func New(db Pinger, dbTimeout time.Duration, trips *service.TripService) *App {
	return &App{db: db, dbTimeout: dbTimeout, trips: trips}
}
