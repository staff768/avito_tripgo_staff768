//go:build integration

package trip_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/infra/clock"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
	repository "github.com/staff768/avito_tripgo_staff768/internal/repository/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/usecase/trip/positions"
)

type positionEffects struct {
	cancelled []uuid.UUID
	saved     int
}

func (e *positionEffects) Cancel(_ context.Context, id uuid.UUID) {
	e.cancelled = append(e.cancelled, id)
}
func (e *positionEffects) PositionSaved(context.Context) { e.saved++ }

type failingPositionRepository struct {
	*repository.TripRepository
	err error
}

func (r failingPositionRepository) InsertPosition(ctx context.Context, point model.Position) (model.Position, error) {
	saved, err := r.TripRepository.InsertPosition(ctx, point)
	if err != nil {
		return model.Position{}, err
	}
	return saved, r.err
}

func TestPositionsTransactionAndOrder(t *testing.T) {
	f := setup(t)
	effects := &positionEffects{}
	usecase := positions.New(f.tx, f.trips, effects, effects, clock.Clock{})
	trip, err := f.trips.Create(f.ctx, newTrip(t, uuid.New()))
	if err != nil {
		t.Fatal(err)
	}
	latest := time.Now().UTC().Truncate(time.Microsecond)
	earlier := latest.Add(-time.Hour)
	ids := make([]int64, 0, 3)
	for _, at := range []time.Time{latest, earlier, earlier} {
		point, err := usecase.Save(f.ctx, model.Position{TripID: trip.ID, Latitude: 59.93, Longitude: 30.33, RecordedAt: at})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, point.ID)
	}
	forced := errors.New("fail after inserting point")
	failing := positions.New(f.tx, failingPositionRepository{TripRepository: f.trips, err: forced}, effects, effects, clock.Clock{})
	_, err = failing.Save(f.ctx, model.Position{TripID: trip.ID, RecordedAt: latest.Add(time.Second)})
	if !errors.Is(err, forced) {
		t.Fatalf("save failure = %v", err)
	}
	fetched, err := f.trips.GetByID(f.ctx, trip.ID)
	if err != nil || fetched.LastPositionAt == nil || !fetched.LastPositionAt.Equal(latest) {
		t.Fatalf("last position = %v, error = %v", fetched.LastPositionAt, err)
	}
	if _, err := f.trips.Finish(f.ctx, trip.ID, latest); err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Save(f.ctx, model.Position{TripID: trip.ID, RecordedAt: latest})
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("completed trip error = %v", err)
	}
	points, err := usecase.List(f.ctx, trip.ID)
	if err != nil || len(points) != 3 {
		t.Fatalf("list points = %v, error = %v", points, err)
	}
	if points[0].ID != ids[1] || points[1].ID != ids[2] || points[2].ID != ids[0] {
		t.Fatalf("unexpected route order: %v", points)
	}
	if effects.saved != 3 || !slices.Equal(effects.cancelled, []uuid.UUID{trip.ID, trip.ID, trip.ID}) {
		t.Fatalf("side effects = %+v", effects)
	}
}

func TestStaleTrips(t *testing.T) {
	f := setup(t)
	var now time.Time
	if err := f.pool.QueryRow(f.ctx, "SELECT now()").Scan(&now); err != nil {
		t.Fatal(err)
	}
	effects := &positionEffects{}
	usecase := positions.New(f.tx, f.trips, effects, effects, clock.Clock{})
	withPosition := createTripAt(t, f, now.Add(-5*time.Minute))
	withoutPosition := createTripAt(t, f, now.Add(-3*time.Minute))
	justStarted := createTripAt(t, f, now)
	fresh := createTripAt(t, f, now.Add(-6*time.Minute))
	completed := createTripAt(t, f, now.Add(-7*time.Minute))

	for _, position := range []model.Position{
		{TripID: withPosition.ID, RecordedAt: now.Add(-2 * time.Minute)},
		{TripID: fresh.ID, RecordedAt: now},
	} {
		if _, err := usecase.Save(f.ctx, position); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.trips.Finish(f.ctx, completed.ID, now); err != nil {
		t.Fatal(err)
	}

	want := []model.StaleTrip{
		{ID: withoutPosition.ID, DriverID: withoutPosition.DriverID, LastActivityAt: withoutPosition.StartedAt},
		{ID: withPosition.ID, DriverID: withPosition.DriverID, LastActivityAt: now.Add(-2 * time.Minute)},
	}
	got, err := f.trips.ListStaleTrips(f.ctx, time.Minute, 100, nil)
	if err != nil || !slices.EqualFunc(got, want, sameStaleTrip) {
		t.Fatalf("stale trips = %v, want %v, error = %v (recent trip %v)", got, want, err, justStarted.ID)
	}
}

func TestStaleTripCursorWithSameTimestamps(t *testing.T) {
	f := setup(t)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	const size = 100
	want := make([]model.StaleTrip, 250)
	for i := range want {
		trip := createTripAt(t, f, at)
		want[i] = model.StaleTrip{ID: trip.ID, DriverID: trip.DriverID, LastActivityAt: at}
	}
	slices.SortFunc(want, func(a, b model.StaleTrip) int { return bytes.Compare(a.ID[:], b.ID[:]) })

	var cursor *model.StaleTripCursor
	offset := 0
	for _, count := range []int{100, 100, 50} {
		batch, err := f.trips.ListStaleTrips(f.ctx, time.Minute, size, cursor)
		if err != nil || !slices.EqualFunc(batch, want[offset:offset+count], sameStaleTrip) {
			t.Fatalf("page at offset %d: got %d rows, want %d in UUID order, error = %v", offset, len(batch), count, err)
		}
		last := batch[len(batch)-1]
		cursor = &model.StaleTripCursor{LastActivityAt: last.LastActivityAt, ID: last.ID}
		offset += count
	}
	batch, err := f.trips.ListStaleTrips(f.ctx, time.Minute, size, cursor)
	if err != nil || len(batch) != 0 {
		t.Fatalf("expected empty page after last cursor, got %d rows, error = %v", len(batch), err)
	}
}

func sameStaleTrip(a, b model.StaleTrip) bool {
	return a.ID == b.ID && a.DriverID == b.DriverID && a.LastActivityAt.Equal(b.LastActivityAt)
}

func createTripAt(t *testing.T, f fixture, at time.Time) model.Trip {
	t.Helper()
	trip := newTrip(t, uuid.New())
	trip.StartedAt = at
	created, err := f.trips.Create(f.ctx, trip)
	if err != nil {
		t.Fatal(err)
	}
	return created
}
