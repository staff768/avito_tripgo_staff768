package positions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

const (
	staleAfter    = time.Minute
	resultSuccess = "success"
	batchSize     = 100
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type poolFixture struct {
	pool    *Pool
	repo    *MockstaleTrips
	push    *MockpushGateway
	reader  *sdkmetric.ManualReader
	spans   *tracetest.SpanRecorder
	ctx     context.Context
	cancel  context.CancelFunc
	ticks   chan time.Time
	stopped chan struct{}
}

func newPoolFixture(t *testing.T, workers, capacity int) poolFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	f := poolFixture{
		repo: NewMockstaleTrips(ctrl), push: NewMockpushGateway(ctrl), reader: sdkmetric.NewManualReader(),
		spans: tracetest.NewSpanRecorder(), ctx: ctx, cancel: cancel, ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(f.reader))
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(f.spans))
	var err error
	f.pool, err = New(Config{
		StaleAfter: staleAfter, PushTimeout: 3 * time.Second, Workers: workers, QueueSize: capacity, BatchSize: batchSize,
	},
		f.repo, f.push, slog.New(slog.NewTextHandler(io.Discard, nil)), tp, mp)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx)); err != nil {
			t.Error(err)
		}
	})
	return f
}

func startPool(t *testing.T, workers, capacity int) poolFixture {
	t.Helper()
	f := newPoolFixture(t, workers, capacity)
	ticker := NewMockTicker(gomock.NewController(t))
	ticker.EXPECT().Ticks().Return(f.ticks)
	ticker.EXPECT().Stop().Do(func() { close(f.stopped) })
	f.pool.Start(f.ctx, ticker)
	t.Cleanup(func() {
		f.cancel()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := f.pool.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f poolFixture) tick(t *testing.T) {
	t.Helper()
	select {
	case f.ticks <- time.Time{}:
	case <-f.ctx.Done():
		t.Fatal("scanner did not take the tick")
	}
}

func (f poolFixture) afterScan(t *testing.T) {
	t.Helper()
	reached := make(chan struct{})
	f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).
		DoAndReturn(func(context.Context, time.Duration, int, *model.StaleTripCursor) ([]model.StaleTrip, error) {
			close(reached)
			return nil, nil
		})
	f.tick(t)
	receive(t, reached)
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for pool")
		var zero T
		return zero
	}
}

func staleTrip() model.StaleTrip { return staleTripAt(time.Unix(0, 0)) }

func staleTripAt(at time.Time) model.StaleTrip {
	return model.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), LastActivityAt: at}
}

func blockWorker(f poolFixture, trip model.StaleTrip) (<-chan struct{}, chan<- struct{}) {
	started, release := make(chan struct{}), make(chan struct{})
	f.push.EXPECT().RequestPosition(gomock.Any(), trip).DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	return started, release
}

func TestScannerDeduplicatesDropsAndCancels(t *testing.T) {
	f := startPool(t, 1, 1)
	first, waiting, overflow := staleTrip(), staleTrip(), staleTripAt(time.Unix(1, 0))
	started, release := blockWorker(f, first)
	gomock.InOrder(
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{first}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{first, waiting, overflow}, nil),
		f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{waiting, overflow}, nil),
	)
	f.tick(t)
	receive(t, started)
	f.tick(t)
	f.tick(t)
	f.afterScan(t)
	f.expectGauge(t, "tripgo.position_requests.queue.size", 1)
	f.expectGauge(t, "tripgo.position_requests.workers.active", 1)
	f.pool.Cancel(f.ctx, waiting.ID)
	f.expectGauge(t, "tripgo.position_requests.queue.size", 0)
	close(release)
	if err := f.pool.Shutdown(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectCounts(t, "tripgo.stale_checks", map[string]int64{resultQueued: 2, resultDuplicate: 2, resultDropped: 2})
	f.expectCounts(t, "tripgo.position_requests", map[string]int64{resultSuccess: 1, "cancelled": 1})
	f.expectGauge(t, "tripgo.position_requests.queue.size", 0)
	f.expectGauge(t, "tripgo.position_requests.workers.active", 0)
}

func TestWorkersDrainWithFixedConcurrency(t *testing.T) {
	const workers = 3
	f := startPool(t, workers, 8)
	trips := make([]model.StaleTrip, 8)
	started, release := make(chan struct{}, len(trips)), make(chan struct{})
	for i := range trips {
		trips[i] = staleTrip()
		f.push.EXPECT().RequestPosition(gomock.Any(), trips[i]).DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
			started <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return(trips, nil)
	f.tick(t)
	f.afterScan(t)
	for range workers {
		receive(t, started)
	}
	f.expectGauge(t, "tripgo.position_requests.queue.size", int64(len(trips)-workers))
	f.expectGauge(t, "tripgo.position_requests.workers.active", workers)

	f.cancel()
	receive(t, f.stopped)
	close(release)
	receive(t, f.pool.done)
	f.expectCounts(t, "tripgo.position_requests", map[string]int64{resultSuccess: int64(len(trips))})
}

func TestPushFailureDoesNotStopWorker(t *testing.T) {
	f := startPool(t, 1, 2)
	first, second := staleTrip(), staleTripAt(time.Unix(1, 0))
	f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{first, second}, nil)
	gomock.InOrder(
		f.push.EXPECT().RequestPosition(gomock.Any(), first).Return(errors.New("push unavailable")),
		f.push.EXPECT().RequestPosition(gomock.Any(), second).Return(nil),
	)
	f.tick(t)
	f.afterScan(t)
	if err := f.pool.Shutdown(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectCounts(t, "tripgo.position_requests", map[string]int64{resultSuccess: 1, "error": 1})
	histogram, ok := f.collect(t, "tripgo.position_requests.duration").(metricdata.Histogram[float64])
	if !ok || len(histogram.DataPoints) != 2 {
		t.Fatalf("unexpected push duration histogram: %+v", histogram)
	}
	workerSpans := 0
	for _, span := range f.spans.Ended() {
		if span.Name() != "positions.process" {
			continue
		}
		workerSpans++
		links := span.Links()
		if span.Parent().IsValid() || len(links) != 1 || !links[0].SpanContext.IsValid() {
			t.Fatalf("worker span parent = %v, links = %v", span.Parent(), links)
		}
	}
	if workerSpans != 2 {
		t.Fatalf("worker spans = %d, want 2", workerSpans)
	}
}

func TestShutdownDeadlineCancelsPush(t *testing.T) {
	f := startPool(t, 1, 1)
	trip := staleTrip()
	started := make(chan struct{})
	f.repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter, batchSize, nil).Return([]model.StaleTrip{trip}, nil)
	f.push.EXPECT().RequestPosition(gomock.Any(), trip).DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	f.tick(t)
	receive(t, started)
	drainCtx, cancel := context.WithCancel(f.ctx)
	cancel()
	if err := f.pool.Shutdown(drainCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown = %v", err)
	}
	receive(t, f.pool.done)
}

func (f poolFixture) collect(t *testing.T, name string) metricdata.Aggregation {
	t.Helper()
	var result metricdata.ResourceMetrics
	if err := f.reader.Collect(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	for _, scope := range result.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return metric.Data
			}
		}
	}
	t.Fatalf("metric %s is missing", name)
	return nil
}

func (f poolFixture) expectGauge(t *testing.T, name string, value int64) {
	t.Helper()
	gauge, ok := f.collect(t, name).(metricdata.Gauge[int64])
	if !ok || len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != value {
		t.Fatalf("%s = %+v, want %d", name, gauge, value)
	}
}

func (f poolFixture) expectCounts(t *testing.T, name string, want map[string]int64) {
	t.Helper()
	sum, ok := f.collect(t, name).(metricdata.Sum[int64])
	if !ok || len(sum.DataPoints) != len(want) {
		t.Fatalf("%s = %+v, want %v", name, sum, want)
	}
	for _, point := range sum.DataPoints {
		key, _ := point.Attributes.Value("result")
		if point.Value != want[key.AsString()] {
			t.Fatalf("%s result=%s count=%d, want %v", name, key.AsString(), point.Value, want)
		}
	}
}
