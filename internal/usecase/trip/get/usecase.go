package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Usecase struct{ trips tripRepository }

func New(trips tripRepository) *Usecase { return &Usecase{trips: trips} }
func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	result, err := u.trips.GetByID(ctx, id)
	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return result, nil
}
