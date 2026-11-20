package positions

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func TestSavePosition(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	handler := New(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := uuid.New()
	at := time.Date(2026, time.August, 24, 12, 0, 5, 0, time.UTC)
	point := model.Position{TripID: id, RecordedAt: at}
	saved := point
	saved.ID = 42
	uc.EXPECT().Save(gomock.Any(), point).Return(saved, nil)
	body := `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-08-24T12:00:05Z"}`
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.Save(recorder, request, id)

	var result api.TripPosition
	err := json.Unmarshal(recorder.Body.Bytes(), &result)
	if recorder.Code != http.StatusCreated || err != nil || result.Id != 42 || result.TripId != id || !result.RecordedAt.Equal(at) {
		t.Fatalf("status = %d, body = %s, error = %v", recorder.Code, recorder.Body, err)
	}
}

func TestSaveInvalidPositionDocument(t *testing.T) {
	for _, body := range []string{
		`{"coordinates":{"longitude":0},"recorded_at":"2026-08-24T12:00:05Z"}`,
		`{"coordinates":{"latitude":null,"longitude":0},"recorded_at":"2026-08-24T12:00:05Z"}`,
		`{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"tomorrow"}`,
		`{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-08-24T12:00:05Z"}{}`,
	} {
		uc := NewMockusecase(gomock.NewController(t))
		handler := New(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
		recorder := httptest.NewRecorder()
		handler.Save(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), uuid.New())
		assertProblem(t, recorder, http.StatusBadRequest, "invalid_request")
	}
}

func TestListPositions(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	handler := New(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := uuid.New()
	uc.EXPECT().List(gomock.Any(), id).Return(nil, nil)
	uc.EXPECT().List(gomock.Any(), id).Return(nil, domain.ErrTripNotFound)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler.List(recorder, request, id)
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"positions":[]}` {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	handler.List(recorder, request, id)
	assertProblem(t, recorder, http.StatusNotFound, "trip_not_found")
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var result api.Problem
	err := json.Unmarshal(response.Body.Bytes(), &result)
	if response.Code != status || response.Header().Get("Content-Type") != "application/problem+json" || err != nil || result.Code != code {
		t.Fatalf("status = %d, body = %s, error = %v", response.Code, response.Body, err)
	}
}
