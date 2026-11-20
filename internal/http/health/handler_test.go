package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/health"
)

func TestHealth(t *testing.T) {
	db := NewMockpinger(gomock.NewController(t))
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := health.New(db, time.Second, logger)
	response := httptest.NewRecorder()

	handler.Health(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	checkResponse(t, response, http.StatusOK, api.Ok)
}

func TestReady(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		body   api.HealthResponseStatus
	}{
		{name: "available", status: http.StatusOK, body: api.Ok},
		{name: "database failure", err: errors.New("private database details"), status: http.StatusServiceUnavailable, body: api.Unavailable},
		{name: "deadline exceeded", err: context.DeadlineExceeded, status: http.StatusServiceUnavailable, body: api.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := NewMockpinger(gomock.NewController(t))
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
			timeout := time.Second
			handler := health.New(db, timeout, logger)
			request := httptest.NewRequest(http.MethodGet, "/ready", nil)
			type contextKey struct{}
			request = request.WithContext(context.WithValue(request.Context(), contextKey{}, "request value"))
			var pingContext context.Context
			db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				pingContext = ctx
				deadline, ok := ctx.Deadline()
				if !ok || deadline.After(time.Now().Add(timeout)) || ctx.Value(contextKey{}) != "request value" {
					t.Fatal("database ping does not preserve the request context and timeout")
				}
				return tc.err
			})
			response := httptest.NewRecorder()

			handler.Ready(response, request)

			checkResponse(t, response, tc.status, tc.body)
			if !errors.Is(pingContext.Err(), context.Canceled) {
				t.Fatal("ping context was not released after the response")
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("database details leaked into readiness response")
			}
		})
	}
}

func checkResponse(t *testing.T, response *httptest.ResponseRecorder, status int, expected api.HealthResponseStatus) {
	t.Helper()
	if response.Code != status || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	var body api.HealthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != expected {
		t.Fatalf("got status %s, expected %s", body.Status, expected)
	}
}
