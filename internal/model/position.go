package model

import (
	"time"

	"github.com/google/uuid"
)

type Position struct {
	ID         int64
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

type StaleTrip struct {
	ID             uuid.UUID
	DriverID       uuid.UUID
	LastActivityAt time.Time
}

type StaleTripCursor struct {
	LastActivityAt time.Time
	ID             uuid.UUID
}
