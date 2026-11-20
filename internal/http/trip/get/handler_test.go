package get_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/get"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func TestHandle(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "success", status: 200},
		{name: "not found", err: domain.ErrTripNotFound, status: 404, code: "trip_not_found"},
		{name: "completed", err: domain.ErrTripCompleted, status: 409, code: "trip_completed"},
		{name: "busy", err: domain.ErrDriverBusy, status: 409, code: "driver_busy"},
		{name: "invalid", err: &domain.ValidationError{Field: "price", Reason: "invalid"}, status: 400, code: "invalid_request"},
		{name: "internal", err: errors.New("secret postgres query"), status: 500, code: "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockusecase(ctrl)
			id := uuid.New()
			trip := model.Trip{ID: id, UserID: uuid.New(), DriverID: uuid.New(), Status: domain.StatusActive, StartedAt: time.Now().UTC()}
			uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(trip, tc.err)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
			handler := get.New(uc, logger)
			request := httptest.NewRequest("GET", "/api/v1/trips/"+id.String(), strings.NewReader(""))
			response := httptest.NewRecorder()
			handler.Handle(response, request, id)
			if response.Code != tc.status {
				t.Fatalf("got %d: %s", response.Code, response.Body.String())
			}
			if tc.err != nil {
				var problem api.Problem
				if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != tc.code || response.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatalf("bad problem: %+v", problem)
				}
				if strings.Contains(response.Body.String(), "secret") {
					t.Fatal("internal details leaked")
				}
			} else {
				var result api.Trip
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Id != id || result.UserId != trip.UserID || result.Status != api.TripStatus(domain.StatusActive) {
					t.Fatalf("bad trip: %+v", result)
				}
			}
		})
	}
}
