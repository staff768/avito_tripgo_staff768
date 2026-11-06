package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
	"github.com/staff768/avito_tripgo_staff768/internal/usecase/trip/get"
)

func TestExecute(t *testing.T) {
	for _, failure := range []error{nil, domain.ErrTripNotFound, errors.New("database failure")} {
		ctrl := gomock.NewController(t)
		trips := NewMocktripRepository(ctrl)
		id := uuid.New()
		expected := model.Trip{ID: id, Price: 100}
		trips.EXPECT().GetByID(gomock.Any(), id).Return(expected, failure)
		result, err := get.New(trips).Execute(context.Background(), id)
		if !errors.Is(err, failure) {
			t.Fatalf("got %v", err)
		}
		if failure == nil && result != expected {
			t.Fatalf("got %+v", result)
		}
		if failure != nil && result.ID != uuid.Nil {
			t.Fatal("error returned data")
		}
	}
}
