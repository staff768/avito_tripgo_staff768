package create_test

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/create"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func TestHandle(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "success", status: 201},
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
			handler := create.New(uc, logger)
			request := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(`{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`))
			response := httptest.NewRecorder()
			handler.Handle(response, request, api.CreateTripParams{})
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
				if response.Header().Get("Location") != "/api/v1/trips/"+id.String() {
					t.Fatal("missing location")
				}
			}
		})
	}
}

func TestInvalidBody(t *testing.T) {
	valid := `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`
	cases := []string{"", "null", "{}", "{", valid + " {}", strings.Replace(valid, `"price":0`, `"price":null`, 1), strings.Replace(valid, `"latitude":0`, `"latitude":null`, 1), strings.Replace(valid, `"price":0`, `"unknown":0,"price":0`, 1), strings.Replace(valid, `"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c"`, `"user_id":"bad"`, 1), strings.Repeat("x", (1<<20)+1)}
	for i, body := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			uc := NewMockusecase(gomock.NewController(t))
			handler := create.New(uc, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			response := httptest.NewRecorder()
			handler.Handle(response, httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(body)), api.CreateTripParams{})
			var problem api.Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if response.Code != 400 || problem.Code != "invalid_request" {
				t.Fatalf("got %d %s", response.Code, response.Body.String())
			}
		})
	}
}
