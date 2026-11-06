package domainerror

import (
	"errors"
	"net/http"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
)

const (
	codeInvalidRequest = "invalid_request"
	codeTripNotFound   = "trip_not_found"
	codeTripCompleted  = "trip_completed"
	codeDriverBusy     = "driver_busy"
	codeInternalError  = "internal_error"
)

var problems = map[string]api.Problem{
	codeInvalidRequest: {
		Type:   "https://tripgo.example/problems/invalid-request",
		Title:  "Invalid request",
		Status: http.StatusBadRequest,
		Code:   codeInvalidRequest,
		Detail: ptr("Request validation failed"),
	},
	codeTripNotFound: {
		Type:   "https://tripgo.example/problems/trip-not-found",
		Title:  "Trip not found",
		Status: http.StatusNotFound,
		Code:   codeTripNotFound,
		Detail: ptr("Trip was not found"),
	},
	codeTripCompleted: {
		Type:   "https://tripgo.example/problems/trip-completed",
		Title:  "Trip completed",
		Status: http.StatusConflict,
		Code:   codeTripCompleted,
		Detail: ptr("Operation is not allowed for a completed trip"),
	},
	codeDriverBusy: {
		Type:   "https://tripgo.example/problems/driver-busy",
		Title:  "Driver busy",
		Status: http.StatusConflict,
		Code:   codeDriverBusy,
		Detail: ptr("Driver already has an active trip"),
	},
	codeInternalError: {
		Type:   "https://tripgo.example/problems/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Code:   codeInternalError,
		Detail: ptr("Internal server error"),
	},
}

func Problem(err error) (int, api.Problem) {
	code := codeInternalError
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		code = codeInvalidRequest
	case errors.Is(err, domain.ErrTripNotFound):
		code = codeTripNotFound
	case errors.Is(err, domain.ErrTripCompleted):
		code = codeTripCompleted
	case errors.Is(err, domain.ErrDriverBusy):
		code = codeDriverBusy
	}
	problem := problems[code]
	if validation != nil {
		problem.Detail = ptr(validation.Error())
	}
	return int(problem.Status), problem
}
func ptr[T any](v T) *T { return &v }
