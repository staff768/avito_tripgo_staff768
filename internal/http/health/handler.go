package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/response"
)

type Handler struct {
	db      pinger
	timeout time.Duration
	logger  *slog.Logger
}

func New(db pinger, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{db: db, timeout: timeout, logger: logger}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, h.logger, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		response.JSON(w, r, h.logger, http.StatusServiceUnavailable, "application/json", api.HealthResponse{Status: api.Unavailable})
		return
	}
	response.JSON(w, r, h.logger, http.StatusOK, "application/json", api.HealthResponse{Status: api.Ok})
}
