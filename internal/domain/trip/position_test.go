package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func TestValidatePosition(t *testing.T) {
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		point model.Position
		field string
	}{
		{name: "inclusive bounds", point: model.Position{Latitude: -90, Longitude: 180, RecordedAt: now.Add(time.Minute)}},
		{name: "late delivery", point: model.Position{RecordedAt: now.Add(-time.Hour)}},
		{name: "latitude", point: model.Position{Latitude: -90.01, RecordedAt: now}, field: "coordinates"},
		{name: "longitude", point: model.Position{Longitude: 180.01, RecordedAt: now}, field: "coordinates"},
		{name: "too far in future", point: model.Position{RecordedAt: now.Add(time.Minute + time.Nanosecond)}, field: "recorded_at"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := trip.ValidatePosition(tc.point, now)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var validation *trip.ValidationError
			if !errors.As(err, &validation) || validation.Field != tc.field {
				t.Fatalf("validation error = %v, want field %s", err, tc.field)
			}
		})
	}
}
