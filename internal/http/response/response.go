package response

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/http/domainerror"
)

func Error(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, problem := domainerror.Problem(err)
	instance := r.URL.Path
	problem.Instance = &instance
	span := trace.SpanFromContext(r.Context())
	span.RecordError(err)
	span.SetStatus(codes.Error, problem.Code)
	span.SetAttributes(attribute.String("error.type", problem.Code))
	if status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", "error", err)
	}

	JSON(w, r, logger, status, "application/problem+json", problem)
}

func JSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.ErrorContext(r.Context(), "write response failed", "error", err)
	}
}
