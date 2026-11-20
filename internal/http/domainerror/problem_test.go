package domainerror_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrTripNotFound, 404, "trip_not_found"},
		{domain.ErrTripCompleted, 409, "trip_completed"},
		{domain.ErrDriverBusy, 409, "driver_busy"},
		{&domain.ValidationError{Field: "price", Reason: "must not be negative"}, 400, "invalid_request"},
		{errors.New("postgres password=secret SELECT * FROM trips"), 500, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			status, body := domainerror.Problem(fmt.Errorf("wrapped: %w", tc.err))
			if status != tc.status || int(body.Status) != status || body.Code != tc.code || body.Type == "" || body.Title == "" {
				t.Fatalf("got %d %+v", status, body)
			}
			if tc.status == 500 && (strings.Contains(*body.Detail, "secret") || strings.Contains(*body.Detail, "postgres")) {
				t.Fatal("internal details leaked")
			}
			if tc.status == 400 && !strings.Contains(*body.Detail, "price") {
				t.Fatal("validation field missing")
			}
		})
	}
}
