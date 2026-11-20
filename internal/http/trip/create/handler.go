package create

import (
	"log/slog"
	"net/http"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/response"
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/convert"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Handler struct {
	usecase usecase
	logger  *slog.Logger
}

func New(usecase usecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	data, detail := decodeTripData(w, r)
	if detail != "" {
		response.Error(w, r, h.logger, &domain.ValidationError{Field: "body", Reason: detail})
		return
	}

	trip, err := h.usecase.Execute(r.Context(), model.CreateTripParams{
		UserID: data.UserId, DriverID: data.DriverId,
		StartLatitude: data.StartPoint.Latitude, StartLongitude: data.StartPoint.Longitude,
		EndLatitude: data.EndPoint.Latitude, EndLongitude: data.EndPoint.Longitude, Price: data.Price,
	})
	if err != nil {
		response.Error(w, r, h.logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
	response.JSON(w, r, h.logger, http.StatusCreated, "application/json", convert.Trip(trip))
}
