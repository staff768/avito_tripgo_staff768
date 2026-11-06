package trip

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s %s", e.Field, e.Reason) }

type Trip struct{ data model.Trip }

func New(params model.CreateTripParams, id uuid.UUID, at time.Time) (Trip, error) {
	if err := Validate(params); err != nil {
		return Trip{}, err
	}
	return Trip{data: model.Trip{
		ID: id, UserID: params.UserID, DriverID: params.DriverID,
		StartLatitude: params.StartLatitude, StartLongitude: params.StartLongitude,
		EndLatitude: params.EndLatitude, EndLongitude: params.EndLongitude,
		Price: params.Price, Status: StatusActive, StartedAt: at.UTC(),
		CreatedAt: at.UTC(), UpdatedAt: at.UTC(),
	}}, nil
}

func Restore(data model.Trip) Trip {
	if data.FinishedAt != nil {
		at := *data.FinishedAt
		data.FinishedAt = &at
	}
	return Trip{data: data}
}

func (t Trip) Model() model.Trip {
	data := t.data
	if data.FinishedAt != nil {
		at := *data.FinishedAt
		data.FinishedAt = &at
	}
	return data
}

func (t *Trip) Finish(at time.Time) error {
	if t.data.Status != StatusActive {
		return ErrTripCompleted
	}
	at = at.UTC()
	t.data.Status = StatusCompleted
	t.data.FinishedAt = &at
	t.data.UpdatedAt = at
	return nil
}

func Validate(p model.CreateTripParams) error {
	switch {
	case p.UserID == uuid.Nil:
		return &ValidationError{Field: "user_id", Reason: "must be a non-empty uuid"}
	case p.DriverID == uuid.Nil:
		return &ValidationError{Field: "driver_id", Reason: "must be a non-empty uuid"}
	case !validCoordinates(p.StartLatitude, p.StartLongitude):
		return &ValidationError{Field: "start_point", Reason: "is out of the allowed range"}
	case !validCoordinates(p.EndLatitude, p.EndLongitude):
		return &ValidationError{Field: "end_point", Reason: "is out of the allowed range"}
	case p.Price < 0:
		return &ValidationError{Field: "price", Reason: "must be greater than or equal to 0"}
	default:
		return nil
	}
}

func validCoordinates(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
