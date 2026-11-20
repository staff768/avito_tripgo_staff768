package trip_test

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func validParams() model.CreateTripParams {
	return model.CreateTripParams{UserID: uuid.New(), DriverID: uuid.New(), StartLatitude: 90, StartLongitude: -180, EndLatitude: -90, EndLongitude: 180, Price: 0}
}

func TestCreateAndFinish(t *testing.T) {
	params, id := validParams(), uuid.New()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("offset", 3600))
	trip, err := domain.New(params, id, at)
	if err != nil {
		t.Fatal(err)
	}
	created := trip.Model()
	if created.ID != id || created.DriverID != params.DriverID || created.Status != domain.StatusActive || created.FinishedAt != nil || !created.StartedAt.Equal(at) || created.StartedAt.Location() != time.UTC {
		t.Fatalf("unexpected created trip: %+v", created)
	}
	finishedAt := at.Add(time.Hour)
	if err := trip.Finish(finishedAt); err != nil {
		t.Fatal(err)
	}
	completed := trip.Model()
	if completed.Status != domain.StatusCompleted || completed.FinishedAt == nil || !completed.FinishedAt.Equal(finishedAt) || !completed.UpdatedAt.Equal(finishedAt) {
		t.Fatalf("unexpected completed trip: %+v", completed)
	}
	if completed.FinishedAt.Location() != time.UTC || completed.UpdatedAt.Location() != time.UTC {
		t.Fatal("finish timestamps must be normalized to UTC")
	}
	if err := trip.Finish(finishedAt.Add(time.Hour)); !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(trip.Model(), completed) {
		t.Fatal("repeated finish modified trip")
	}
	restoredCompleted := domain.Restore(completed)
	*completed.FinishedAt = at
	if restoredCompleted.Model().FinishedAt.Equal(at) {
		t.Fatal("restore retained mutable external state")
	}

	if trip.Model().FinishedAt.Equal(at) {
		t.Fatal("model exposes mutable domain state")
	}
	restored := domain.Restore(created)
	if !reflect.DeepEqual(restored.Model(), created) {
		t.Fatal("restore changed model")
	}
}

func TestValidation(t *testing.T) {
	const startField = "start_point"
	const endField = "end_point"
	cases := []struct {
		name   string
		change func(*model.CreateTripParams)
		field  string
	}{
		{"user", func(p *model.CreateTripParams) { p.UserID = uuid.Nil }, "user_id"},
		{"driver", func(p *model.CreateTripParams) { p.DriverID = uuid.Nil }, "driver_id"},
		{"start latitude", func(p *model.CreateTripParams) { p.StartLatitude = 91 }, startField},
		{"start longitude", func(p *model.CreateTripParams) { p.StartLongitude = -181 }, startField},
		{"end latitude", func(p *model.CreateTripParams) { p.EndLatitude = -91 }, endField},
		{"end longitude", func(p *model.CreateTripParams) { p.EndLongitude = 181 }, endField},
		{"nan", func(p *model.CreateTripParams) { p.StartLatitude = math.NaN() }, startField},
		{"infinity", func(p *model.CreateTripParams) { p.EndLongitude = math.Inf(1) }, endField},
		{"price", func(p *model.CreateTripParams) { p.Price = -1 }, "price"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := validParams()
			tc.change(&params)
			_, err := domain.New(params, uuid.New(), time.Now())
			var validation *domain.ValidationError
			if !errors.As(err, &validation) || validation.Field != tc.field || validation.Reason == "" || validation.Error() == "" {
				t.Fatalf("got %v", err)
			}
		})
	}
}
