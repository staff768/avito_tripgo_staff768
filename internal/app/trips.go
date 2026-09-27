package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/repository"
	"github.com/staff768/avito_tripgo_staff768/internal/service"
)

const maxRequestBody = 1 << 20

var tripDataRequiredFields = []string{"user_id", "driver_id", "start_point", "end_point", "price"}

func (a *App) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	data, detail := decodeTripData(w, r)
	if detail != "" {
		writeProblem(w, r, codeInvalidRequest, detail)
		return
	}

	if detail := validateTripData(data); detail != "" {
		writeProblem(w, r, codeInvalidRequest, detail)
		return
	}

	trip, err := a.trips.CreateTrip(r.Context(), service.CreateTripParams{
		UserID:         data.UserId,
		DriverID:       data.DriverId,
		StartLatitude:  data.StartPoint.Latitude,
		StartLongitude: data.StartPoint.Longitude,
		EndLatitude:    data.EndPoint.Latitude,
		EndLongitude:   data.EndPoint.Longitude,
		Price:          data.Price,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
	writeJSON(w, http.StatusCreated, toAPITrip(trip))
}

func (a *App) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := a.trips.GetTrip(r.Context(), tripID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (a *App) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := a.trips.FinishTrip(r.Context(), tripID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func decodeTripData(w http.ResponseWriter, r *http.Request) (api.TripData, string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		return api.TripData{}, "Request body could not be read"
	}

	var data api.TripData

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return api.TripData{}, "Request body is not a valid trip document"
	}

	present := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &present); err != nil {
		return api.TripData{}, "Request body is not a valid trip document"
	}

	for _, field := range tripDataRequiredFields {
		if _, ok := present[field]; !ok {
			return api.TripData{}, field + " is required"
		}
	}

	return data, ""
}

func validateTripData(data api.TripData) string {
	switch {
	case data.UserId == uuid.Nil:
		return "user_id must be a non-empty uuid"
	case data.DriverId == uuid.Nil:
		return "driver_id must be a non-empty uuid"
	case !validCoordinates(data.StartPoint):
		return "start_point is out of the allowed range"
	case !validCoordinates(data.EndPoint):
		return "end_point is out of the allowed range"
	case data.Price < 0:
		return "price must be greater than or equal to 0"
	default:
		return ""
	}
}

func validCoordinates(point api.Coordinates) bool {
	return point.Latitude >= -90 && point.Latitude <= 90 &&
		point.Longitude >= -180 && point.Longitude <= 180
}

func toAPITrip(trip repository.Trip) api.Trip {
	return api.Trip{
		Id:       trip.ID,
		UserId:   trip.UserID,
		DriverId: trip.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  trip.StartLatitude,
			Longitude: trip.StartLongitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndLatitude,
			Longitude: trip.EndLongitude,
		},
		Price:      trip.Price,
		Status:     api.TripStatus(trip.Status),
		StartedAt:  trip.StartedAt.UTC(),
		FinishedAt: utc(trip.FinishedAt),
	}
}

func utc(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}

	return ptr(at.UTC())
}
