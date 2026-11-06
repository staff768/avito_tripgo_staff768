//go:build integration

package trip_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/postgres"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/postgres/transaction"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
	repository "github.com/staff768/avito_tripgo_staff768/internal/repository/trip"
)

type fixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	tx      *transaction.TxManager
	trips   *repository.TripRepository
	history *repository.TripStatusHistoryRepository
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	env, err := godotenv.Read("../../../.env")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal("read integration environment")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = env["DATABASE_URL"]
	}
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("parse integration database configuration")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	admin, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal("create integration connection")
	}
	t.Cleanup(admin.Close)
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("clean isolated schema: %v", err)
		}
	})
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	tracer, err := postgres.NewTracer(tracenoop.NewTracerProvider(), noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../migrations/*.sql")
	if err != nil || len(files) != 2 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(strings.TrimPrefix(string(data), "-- +goose Up"), "-- +goose Down")[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
	}
	tx := transaction.New(pool, 3*time.Second)
	return fixture{ctx: ctx, pool: pool, tx: tx, trips: repository.NewTripRepository(tx, 3*time.Second), history: repository.NewTripStatusHistoryRepository(tx, 3*time.Second)}
}

func newTrip(t *testing.T, driver uuid.UUID) model.Trip {
	t.Helper()
	entity, err := domain.New(model.CreateTripParams{UserID: uuid.New(), DriverID: driver, Price: 1450}, uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return entity.Model()
}

func TestRepositoryLifecycle(t *testing.T) {
	f := setup(t)
	original := newTrip(t, uuid.New())
	var created model.Trip
	err := f.tx.Do(f.ctx, func(ctx context.Context) error {
		var err error
		created, err = f.trips.Create(ctx, original)
		if err != nil {
			return err
		}
		return f.history.Add(ctx, model.StatusChange{TripID: created.ID, ToStatus: domain.StatusActive})
	})
	if err != nil {
		t.Fatal(err)
	}
	fetched, err := f.trips.GetByID(f.ctx, original.ID)
	if err != nil || fetched.ID != created.ID || fetched.Price != 1450 || fetched.Status != domain.StatusActive {
		t.Fatalf("fetch: %+v %v", fetched, err)
	}
	_, err = f.trips.Create(f.ctx, newTrip(t, original.DriverID))
	if !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("busy: %v", err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	finished, err := f.trips.Finish(f.ctx, created.ID, at)
	if err != nil || finished.Status != domain.StatusCompleted || finished.FinishedAt == nil || !finished.FinishedAt.Equal(at) {
		t.Fatalf("finish: %+v %v", finished, err)
	}
	_, err = f.trips.Finish(f.ctx, created.ID, at.Add(time.Hour))
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("repeat finish: %v", err)
	}
	unchanged, err := f.trips.GetByID(f.ctx, created.ID)
	if err != nil || !unchanged.FinishedAt.Equal(at) {
		t.Fatal("repeat finish changed timestamp")
	}
	for _, op := range []func() error{
		func() error { _, err := f.trips.GetByID(f.ctx, uuid.New()); return err },
		func() error { _, err := f.trips.Finish(f.ctx, uuid.New(), at); return err },
	} {
		if err := op(); !errors.Is(err, domain.ErrTripNotFound) {
			t.Fatalf("missing: %v", err)
		}
	}
	count, err := f.trips.CountActive(f.ctx)
	if err != nil || count != 0 {
		t.Fatalf("active=%d err=%v", count, err)
	}
}

func TestTransactionRollback(t *testing.T) {
	f := setup(t)
	trip := newTrip(t, uuid.New())
	err := f.tx.Do(f.ctx, func(ctx context.Context) error {
		if _, err := f.trips.Create(ctx, trip); err != nil {
			return err
		}
		return f.history.Add(ctx, model.StatusChange{TripID: trip.ID, ToStatus: "invalid"})
	})
	if err == nil {
		t.Fatal("expected history constraint failure")
	}
	for _, table := range []string{"trips", "trip_status_history"} {
		var count int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s contains partial transaction", table)
		}
	}
}

func TestFinishRollback(t *testing.T) {
	f := setup(t)
	trip, err := f.trips.Create(f.ctx, newTrip(t, uuid.New()))
	if err != nil {
		t.Fatal(err)
	}
	err = f.tx.Do(f.ctx, func(ctx context.Context) error {
		if _, err := f.trips.Finish(ctx, trip.ID, time.Now()); err != nil {
			return err
		}
		return f.history.Add(ctx, model.StatusChange{TripID: trip.ID, ToStatus: "invalid"})
	})
	if err == nil {
		t.Fatal("expected history constraint failure")
	}
	fetched, err := f.trips.GetByID(f.ctx, trip.ID)
	if err != nil || fetched.Status != domain.StatusActive || fetched.FinishedAt != nil {
		t.Fatalf("finish was not rolled back: %+v %v", fetched, err)
	}
}

func TestConcurrentOperations(t *testing.T) {
	f := setup(t)
	driver := uuid.New()
	trips := []model.Trip{newTrip(t, driver), newTrip(t, driver)}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, trip := range trips {
		wg.Go(func() { <-start; _, err := f.trips.Create(f.ctx, trip); results <- err })
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, domain.ErrDriverBusy):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("create successes=%d conflicts=%d", success, conflict)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(f.ctx, "SELECT id FROM trips WHERE driver_id=$1", driver).Scan(&id); err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	for range 2 {
		wg.Go(func() { <-start; _, err := f.trips.Finish(f.ctx, id, time.Now()); results <- err })
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict = 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, domain.ErrTripCompleted):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("finish successes=%d conflicts=%d", success, conflict)
	}
}

func TestNestedAndCanceledTransactions(t *testing.T) {
	f := setup(t)
	trip := newTrip(t, uuid.New())
	failure := fmt.Errorf("force rollback")
	err := f.tx.Do(f.ctx, func(ctx context.Context) error {
		return f.tx.Do(ctx, func(ctx context.Context) error {
			if _, err := f.trips.Create(ctx, trip); err != nil {
				return err
			}
			return failure
		})
	})
	if !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
	_, err = f.trips.GetByID(f.ctx, trip.ID)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatal("nested transaction committed")
	}
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if err := f.tx.Do(canceled, func(context.Context) error { t.Fatal("canceled transaction invoked callback"); return nil }); err == nil {
		t.Fatal("canceled transaction succeeded")
	}
}

func TestPanicRollback(t *testing.T) {
	f := setup(t)
	trip := newTrip(t, uuid.New())
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		if err := f.tx.Do(f.ctx, func(ctx context.Context) error {
			if _, err := f.trips.Create(ctx, trip); err != nil {
				return err
			}
			panic("rollback panic")
		}); err != nil {
			t.Fatal(err)
		}
	}()
	if recovered != "rollback panic" {
		t.Fatalf("panic was not propagated: %v", recovered)
	}
	_, err := f.trips.GetByID(f.ctx, trip.ID)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("panicked transaction committed: %v", err)
	}
}

func TestCanceledCallbackRollback(t *testing.T) {
	f := setup(t)
	trip := newTrip(t, uuid.New())
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	err := f.tx.Do(ctx, func(ctx context.Context) error {
		if _, err := f.trips.Create(ctx, trip); err != nil {
			return err
		}
		cancel()
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	_, err = f.trips.GetByID(f.ctx, trip.ID)
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("canceled transaction committed: %v", err)
	}
}

func TestMigrationsDown(t *testing.T) {
	f := setup(t)
	files, err := filepath.Glob("../../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := len(files) - 1; i >= 0; i-- {
		data, err := os.ReadFile(files[i])
		if err != nil {
			t.Fatal(err)
		}
		down := strings.Split(string(data), "-- +goose Down")[1]
		if _, err := f.pool.Exec(f.ctx, down); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("down left %d tables", count)
	}
}
