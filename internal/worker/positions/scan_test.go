package positions

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

func TestScannerReadsAllBatches(t *testing.T) {
	f := newPoolFixture(t, 1, 250)
	trips := scanTrips()
	first, second := trips[99], trips[199]
	gomock.InOrder(
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return(trips[:100], nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, &model.StaleTripCursor{
			LastActivityAt: first.LastActivityAt, ID: first.ID,
		}).Return(trips[100:200], nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, &model.StaleTripCursor{
			LastActivityAt: second.LastActivityAt, ID: second.ID,
		}).Return(trips[200:], nil),
	)
	f.pool.check(f.ctx)
	f.expectGauge(t, "tripgo.position_requests.queue.size", 250)
	f.expectCounts(t, "tripgo.stale_checks", map[string]int64{resultQueued: 250})
	f.expectRowsRead(t, 1, 250)
}

func TestScannerStopsAtFirstDropAndRestartsFromBeginning(t *testing.T) {
	f := newPoolFixture(t, 1, 50)
	trips := scanTrips()
	f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return(trips[:100], nil).Times(2)
	f.pool.check(f.ctx)
	f.pool.check(f.ctx)
	f.expectGauge(t, "tripgo.position_requests.queue.size", 50)
	f.expectCounts(t, "tripgo.stale_checks", map[string]int64{resultQueued: 50, resultDuplicate: 50, resultDropped: 2})
	f.expectRowsRead(t, 2, 200)
}

func scanTrips() []model.StaleTrip {
	trips := make([]model.StaleTrip, 250)
	for i := range trips {
		trips[i] = staleTripAt(time.Unix(int64(i), 0))
	}
	return trips
}

func (f poolFixture) expectRowsRead(t *testing.T, count uint64, sum int64) {
	t.Helper()
	histogram, ok := f.collect(t, "tripgo.stale_checks.rows_read").(metricdata.Histogram[int64])
	if !ok || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != count || histogram.DataPoints[0].Sum != sum {
		t.Fatalf("rows read = %+v, want count %d and sum %d", histogram, count, sum)
	}
}
