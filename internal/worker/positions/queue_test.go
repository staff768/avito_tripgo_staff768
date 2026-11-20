package positions

import (
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func TestWorkersTakeOldestAcrossTicks(t *testing.T) {
	f := startPool(t, 1, 3)
	busy := staleTrip()
	oldest, middle, newest := staleTripAt(time.Unix(1, 0)), staleTripAt(time.Unix(2, 0)), staleTripAt(time.Unix(3, 0))
	started, release := blockWorker(f, busy)
	gomock.InOrder(
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{busy}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{middle, newest}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{oldest}, nil),
	)
	gomock.InOrder(
		f.push.EXPECT().RequestPosition(gomock.Any(), oldest).Return(nil),
		f.push.EXPECT().RequestPosition(gomock.Any(), middle).Return(nil),
		f.push.EXPECT().RequestPosition(gomock.Any(), newest).Return(nil),
	)
	f.tick(t)
	receive(t, started)
	f.tick(t)
	f.tick(t)
	f.afterScan(t)
	close(release)
	if err := f.pool.Shutdown(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestQueueEvictsFreshestAndRejectsNewerTrip(t *testing.T) {
	f := startPool(t, 1, 2)
	busy := staleTrip()
	oldest, middle, fresh, newest := staleTripAt(time.Unix(1, 0)), staleTripAt(time.Unix(2, 0)),
		staleTripAt(time.Unix(3, 0)), staleTripAt(time.Unix(4, 0))
	started, release := blockWorker(f, busy)
	gomock.InOrder(
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{busy}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{middle, fresh}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{oldest, newest}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{newest}, nil),
	)
	gomock.InOrder(
		f.push.EXPECT().RequestPosition(gomock.Any(), oldest).Return(nil),
		f.push.EXPECT().RequestPosition(gomock.Any(), middle).Return(nil),
	)
	f.tick(t)
	receive(t, started)
	f.tick(t)
	f.tick(t)
	f.tick(t)
	f.afterScan(t)
	f.expectGauge(t, "tripgo.position_requests.queue.size", 2)
	close(release)
	if err := f.pool.Shutdown(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectCounts(t, "tripgo.stale_checks", map[string]int64{resultQueued: 4, resultDropped: 2})
	f.expectCounts(t, "tripgo.position_requests", map[string]int64{resultSuccess: 3})
}
