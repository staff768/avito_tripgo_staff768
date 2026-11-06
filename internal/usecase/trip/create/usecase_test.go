package create_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
	"github.com/staff768/avito_tripgo_staff768/internal/usecase/trip/create"
)

func TestExecute(t *testing.T) {
	repositoryFailure := errors.New("database failed")
	historyFailure := errors.New("history failed")
	cases := []struct {
		name       string
		repoErr    error
		historyErr error
	}{
		{name: "success"},
		{name: "repository", repoErr: repositoryFailure},
		{name: "history", historyErr: historyFailure},
		{name: "conflict", repoErr: domain.ErrDriverBusy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			tx, trips, history, clock, observer := NewMocktxManager(ctrl), NewMocktripRepository(ctrl), NewMockhistoryRepository(ctrl), NewMockclock(ctrl), NewMockobserver(ctrl)
			at := time.Now().UTC()
			params := model.CreateTripParams{UserID: uuid.New(), DriverID: uuid.New(), Price: 1450}
			clock.EXPECT().Now().Return(at)
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
			repoErr := tc.repoErr
			trips.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, trip model.Trip) (model.Trip, error) {
				if trip.UserID != params.UserID || trip.DriverID != params.DriverID || trip.Price != params.Price || trip.ID == uuid.Nil || trip.Status != domain.StatusActive || !trip.StartedAt.Equal(at) {
					t.Fatalf("bad saved trip: %+v", trip)
				}
				return trip, repoErr
			})
			if tc.repoErr == nil {
				history.EXPECT().Add(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, change model.StatusChange) error {
					if change.TripID == uuid.Nil || change.Reason == nil || change.ToStatus != domain.StatusActive || !change.ChangedAt.Equal(at) {
						t.Fatalf("bad history: %+v", change)
					}
					return tc.historyErr
				})
			}
			if tc.repoErr == nil && tc.historyErr == nil {
				observer.EXPECT().Created(gomock.Any())
			}
			var logs bytes.Buffer
			uc := create.New(tx, trips, history, clock, observer, slog.New(slog.NewJSONHandler(&logs, nil)))
			result, err := uc.Execute(context.Background(), params)
			wantErr := tc.repoErr
			if wantErr == nil {
				wantErr = tc.historyErr
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("got %v want %v", err, wantErr)
			}
			if wantErr == nil && (result.ID == uuid.Nil || result.Status != domain.StatusActive) {
				t.Fatalf("bad result: %+v", result)
			}
			if wantErr != nil && result.ID != uuid.Nil {
				t.Fatal("failure returned a trip")
			}
			assertBusinessLog(t, &logs, result.ID, wantErr)
		})
	}
}

func TestTransactionFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	tx, trips, history, clock, observer := NewMocktxManager(ctrl), NewMocktripRepository(ctrl), NewMockhistoryRepository(ctrl), NewMockclock(ctrl), NewMockobserver(ctrl)
	params := model.CreateTripParams{UserID: uuid.New(), DriverID: uuid.New(), Price: 1450}
	clock.EXPECT().Now().Return(time.Now())
	failure := errors.New("begin failed")
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(failure)
	uc := create.New(tx, trips, history, clock, observer, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	_, err := uc.Execute(context.Background(), params)
	if !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidParams(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := NewMockclock(ctrl)
	clock.EXPECT().Now().Return(time.Now())
	uc := create.New(NewMocktxManager(ctrl), NewMocktripRepository(ctrl), NewMockhistoryRepository(ctrl), clock, NewMockobserver(ctrl), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	_, err := uc.Execute(context.Background(), model.CreateTripParams{})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("got %v", err)
	}
}

func assertBusinessLog(t *testing.T, logs *bytes.Buffer, id uuid.UUID, failure error) {
	t.Helper()
	if failure != nil {
		if logs.Len() != 0 {
			t.Fatal("failed transaction emitted a success log")
		}
		return
	}
	var entry struct {
		Level   string `json:"level"`
		Message string `json:"msg"`
		TripID  string `json:"trip_id"`
	}
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("expected one business log: %v", err)
	}
	if entry.Level != "INFO" || entry.Message != "trip created" || entry.TripID != id.String() {
		t.Fatalf("unexpected business log: %+v", entry)
	}
}
