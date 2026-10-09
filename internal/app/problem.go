package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/repository"
)

const (
	contentTypeJSON    = "application/json"
	contentTypeProblem = "application/problem+json"
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

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrTripNotFound):
		writeProblem(w, r, codeTripNotFound, "")
	case errors.Is(err, repository.ErrTripCompleted):
		writeProblem(w, r, codeTripCompleted, "")
	case errors.Is(err, repository.ErrDriverBusy):
		writeProblem(w, r, codeDriverBusy, "")
	default:
		log.Printf("%s %s failed: %v", r.Method, r.URL.Path, err)
		writeProblem(w, r, codeInternalError, "")
	}
}

func invalidParamDetail(err error) string {
	var invalidFormat *api.InvalidParamFormatError
	if errors.As(err, &invalidFormat) {
		return invalidFormat.ParamName + " has an invalid format"
	}

	var tooManyValues *api.TooManyValuesForParamError
	if errors.As(err, &tooManyValues) {
		return tooManyValues.ParamName + " must be passed once"
	}

	return "Request parameter is not valid"
}

func writeProblem(w http.ResponseWriter, r *http.Request, code string, detail string) {
	problem, ok := problems[code]
	if !ok {
		problem = problems[codeInternalError]
	}

	problem.Instance = ptr(r.URL.Path)
	if detail != "" {
		problem.Detail = ptr(detail)
	}

	w.Header().Set("Content-Type", contentTypeProblem)
	w.WriteHeader(int(problem.Status))

	if err := json.NewEncoder(w).Encode(problem); err != nil {
		log.Printf("write problem response failed: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func ptr[T any](v T) *T {
	return &v
}
