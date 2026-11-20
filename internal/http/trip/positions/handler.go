package positions

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/response"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Handler struct {
	usecase usecase
	logger  *slog.Logger
}

func New(usecase usecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request, id api.TripId) {
	data, err := decodePosition(w, r, id)
	if err != nil {
		response.Error(w, r, h.logger, err)
		return
	}
	saved, err := h.usecase.Save(r.Context(), data)
	if err != nil {
		response.Error(w, r, h.logger, err)
		return
	}
	response.JSON(w, r, h.logger, http.StatusCreated, "application/json", toAPI(saved))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request, id api.TripId) {
	points, err := h.usecase.List(r.Context(), id)
	if err != nil {
		response.Error(w, r, h.logger, err)
		return
	}
	result := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(points))}
	for _, point := range points {
		result.Positions = append(result.Positions, toAPI(point))
	}
	response.JSON(w, r, h.logger, http.StatusOK, "application/json", result)
}

func decodePosition(w http.ResponseWriter, r *http.Request, id api.TripId) (model.Position, error) {
	var data struct {
		Coordinates struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"coordinates"`
		RecordedAt *time.Time `json:"recorded_at"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		return model.Position{}, &domain.ValidationError{Field: "body", Reason: "must be a valid position document"}
	}
	if data.Coordinates.Latitude == nil || data.Coordinates.Longitude == nil || data.RecordedAt == nil {
		return model.Position{}, &domain.ValidationError{Field: "body", Reason: "must include latitude, longitude and recorded_at"}
	}
	return model.Position{
		TripID: id, Latitude: *data.Coordinates.Latitude, Longitude: *data.Coordinates.Longitude,
		RecordedAt: data.RecordedAt.UTC(),
	}, nil
}

func toAPI(position model.Position) api.TripPosition {
	return api.TripPosition{
		Id: position.ID, TripId: position.TripID, RecordedAt: position.RecordedAt.UTC(),
		Coordinates: api.Coordinates{Latitude: position.Latitude, Longitude: position.Longitude},
	}
}
