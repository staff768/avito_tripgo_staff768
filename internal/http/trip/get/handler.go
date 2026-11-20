package get

import (
	"log/slog"
	"net/http"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/response"
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/convert"
)

type Handler struct {
	usecase usecase
	logger  *slog.Logger
}

func New(usecase usecase, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id api.TripId) {
	trip, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		response.Error(w, r, h.logger, err)
		return
	}

	response.JSON(w, r, h.logger, http.StatusOK, "application/json", convert.Trip(trip))
}
