package trip

import (
	"time"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func ValidatePosition(position model.Position, now time.Time) error {
	if !validCoordinates(position.Latitude, position.Longitude) {
		return &ValidationError{Field: "coordinates", Reason: "are out of the allowed range"}
	}
	if position.RecordedAt.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "recorded_at", Reason: "must not be more than one minute in the future"}
	}
	return nil
}
