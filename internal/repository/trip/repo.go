package trip

import (
	"strings"
	"time"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
)

const (
	StatusActive    = domain.StatusActive
	StatusCompleted = domain.StatusCompleted
)

const statusColumn = "status"

const driverActiveIndex = "trips_driver_active_uidx"

var tripColumns = []string{
	"id",
	"user_id",
	"driver_id",
	"start_latitude",
	"start_longitude",
	"end_latitude",
	"end_longitude",
	"price",
	statusColumn,
	"started_at",
	"finished_at",
	"created_at",
	"updated_at",
}

var insertTripColumns = []string{
	"id",
	"user_id",
	"driver_id",
	"start_latitude",
	"start_longitude",
	"end_latitude",
	"end_longitude",
	"price",
	statusColumn,
	"started_at",
	"finished_at",
}

var returningTrip = "RETURNING " + strings.Join(tripColumns, ", ")

type TripRepository struct {
	db           executor
	queryTimeout time.Duration
}

func NewTripRepository(db executor, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{db: db, queryTimeout: queryTimeout}
}
