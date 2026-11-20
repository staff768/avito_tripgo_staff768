package positions

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Usecase struct {
	tx       txManager
	repo     repository
	queue    queue
	observer observer
	clock    clock
}

func New(tx txManager, repo repository, queue queue, observer observer, clock clock) *Usecase {
	return &Usecase{tx: tx, repo: repo, queue: queue, observer: observer, clock: clock}
}

func (u *Usecase) Save(ctx context.Context, position model.Position) (model.Position, error) {
	if err := domain.ValidatePosition(position, u.clock.Now()); err != nil {
		return model.Position{}, err
	}
	var saved model.Position
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		if err := u.repo.AdvanceLastPosition(ctx, position.TripID, position.RecordedAt); err != nil {
			return err
		}
		var err error
		saved, err = u.repo.InsertPosition(ctx, position)
		return err
	})
	if err != nil {
		return model.Position{}, fmt.Errorf("save position: %w", err)
	}

	u.queue.Cancel(ctx, position.TripID)
	u.observer.PositionSaved(ctx)
	return saved, nil
}

func (u *Usecase) List(ctx context.Context, id uuid.UUID) ([]model.Position, error) {
	if err := u.repo.CheckExists(ctx, id); err != nil {
		return nil, fmt.Errorf("find trip for route: %w", err)
	}
	return u.repo.ListPositions(ctx, id)
}
