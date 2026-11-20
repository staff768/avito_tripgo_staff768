package positions

import (
	"container/heap"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Config struct {
	StaleAfter  time.Duration
	PushTimeout time.Duration
	Workers     int
	QueueSize   int
	BatchSize   int
}

type Pool struct {
	cfg    Config
	repo   staleTrips
	push   pushGateway
	logger *slog.Logger
	tracer trace.Tracer
	stats  instruments

	mu       sync.Mutex
	ready    *sync.Cond
	queue    taskHeap
	pending  map[uuid.UUID]*task
	active   int64
	draining bool

	stopScan context.CancelFunc
	stopWork context.CancelFunc
	done     chan struct{}
}

func New(cfg Config, repo staleTrips, push pushGateway, logger *slog.Logger, tp trace.TracerProvider, mp metric.MeterProvider) (*Pool, error) {
	p := &Pool{
		cfg: cfg, repo: repo, push: push, logger: logger, tracer: tp.Tracer("tripgo/positions"),
		queue: make(taskHeap, 0, cfg.QueueSize), pending: make(map[uuid.UUID]*task), done: make(chan struct{}),
	}
	p.ready = sync.NewCond(&p.mu)
	stats, err := p.newInstruments(mp.Meter("tripgo/positions"))
	if err != nil {
		return nil, fmt.Errorf("create position pool instruments: %w", err)
	}
	p.stats = stats
	return p, nil
}

func (p *Pool) Start(ctx context.Context, ticker Ticker) {
	scanCtx, stopScan := context.WithCancel(ctx)
	workCtx, stopWork := context.WithCancel(context.WithoutCancel(ctx))
	p.stopScan, p.stopWork = stopScan, stopWork

	var workers sync.WaitGroup
	for range p.cfg.Workers {
		workers.Go(func() { p.work(workCtx) })
	}
	p.logger.InfoContext(ctx, "position workers started", "workers", p.cfg.Workers)

	go func() {
		defer close(p.done)
		defer stopWork()
		defer stopScan()
		p.scan(scanCtx, ticker)
		p.mu.Lock()
		p.draining = true
		p.ready.Broadcast()
		p.mu.Unlock()
		workers.Wait()
		p.logger.InfoContext(workCtx, "position workers stopped")
	}()
}

func (p *Pool) Shutdown(ctx context.Context) error {
	p.stopScan()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		p.stopWork()
		p.mu.Lock()
		p.ready.Broadcast()
		p.mu.Unlock()
		return fmt.Errorf("drain position pool: %w", ctx.Err())
	}
}

func (p *Pool) Cancel(ctx context.Context, id uuid.UUID) {
	p.mu.Lock()
	job := p.pending[id]
	if job == nil || job.index < 0 {
		p.mu.Unlock()
		return
	}
	heap.Remove(&p.queue, job.index)
	delete(p.pending, id)
	p.mu.Unlock()
	p.stats.requests.Add(ctx, 1, resultLabel("cancelled"))
}

func (p *Pool) scan(ctx context.Context, ticker Ticker) {
	p.logger.InfoContext(ctx, "position scanner started")
	defer p.logger.InfoContext(ctx, "position scanner stopped")
	defer ticker.Stop()
	ticks := ticker.Ticks()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if ctx.Err() != nil {
				return
			}
			p.check(ctx)
		}
	}
}

func (p *Pool) check(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan")
	defer span.End()
	var rowsRead int64
	defer func() { p.stats.rowsRead.Record(ctx, rowsRead) }()

	var after *model.StaleTripCursor
	for ctx.Err() == nil {
		trips, err := p.repo.ListStaleTrips(ctx, p.cfg.StaleAfter, p.cfg.BatchSize, after)
		if err != nil {
			if ctx.Err() == nil {
				p.logger.ErrorContext(ctx, "scan stale trips failed", "error", err)
				span.RecordError(err)
				span.SetStatus(codes.Error, "scan stale trips failed")
			}
			return
		}
		rowsRead += int64(len(trips))
		if p.enqueueBatch(ctx, trips, span.SpanContext()) || len(trips) < p.cfg.BatchSize {
			return
		}
		last := trips[len(trips)-1]
		after = &model.StaleTripCursor{LastActivityAt: last.LastActivityAt, ID: last.ID}
	}
}

func (p *Pool) enqueueBatch(ctx context.Context, trips []model.StaleTrip, scan trace.SpanContext) bool {
	for _, trip := range trips {
		if ctx.Err() != nil {
			return true
		}
		result, dropped := p.enqueue(trip, scan)
		p.stats.checks.Add(ctx, 1, resultLabel(result))
		if dropped != nil {
			if result != resultDropped {
				p.stats.checks.Add(ctx, 1, resultLabel(resultDropped))
			}
			p.logger.WarnContext(ctx, "position queue full", "trip_id", dropped.trip.ID)
			return true
		}
	}
	return false
}

const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
)

func (p *Pool) enqueue(trip model.StaleTrip, scan trace.SpanContext) (string, *task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.pending[trip.ID]; exists {
		return resultDuplicate, nil
	}
	job := &task{trip: trip, scan: scan}
	var dropped *task
	if p.queue.Len() == p.cfg.QueueSize {
		freshest := p.queue.freshest()
		if !trip.LastActivityAt.Before(freshest.trip.LastActivityAt) {
			return resultDropped, job
		}
		heap.Remove(&p.queue, freshest.index)
		delete(p.pending, freshest.trip.ID)
		dropped = freshest
	}
	heap.Push(&p.queue, job)
	p.pending[trip.ID] = job
	p.ready.Signal()
	return resultQueued, dropped
}

func (p *Pool) work(ctx context.Context) {
	for {
		job := p.next(ctx)
		if job == nil {
			return
		}
		p.process(ctx, job)
	}
}

func (p *Pool) next(ctx context.Context) *task {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.queue.Len() == 0 && !p.draining && ctx.Err() == nil {
		p.ready.Wait()
	}
	if ctx.Err() != nil || p.queue.Len() == 0 {
		return nil
	}
	p.active++
	return heap.Pop(&p.queue).(*task)
}

func (p *Pool) process(ctx context.Context, job *task) {
	ctx, span := p.tracer.Start(ctx, "positions.process", trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: job.scan}))
	defer span.End()
	defer func() {
		p.mu.Lock()
		p.active--
		delete(p.pending, job.trip.ID)
		p.mu.Unlock()
	}()

	requestCtx, cancel := context.WithTimeout(ctx, p.cfg.PushTimeout)
	defer cancel()
	start := time.Now()
	err := p.push.RequestPosition(requestCtx, job.trip)
	result := "success"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, "request position failed")
		p.logger.WarnContext(ctx, "request position failed", "trip_id", job.trip.ID, "error", err)
	}
	label := resultLabel(result)
	p.stats.duration.Record(ctx, time.Since(start).Seconds(), label)
	p.stats.requests.Add(ctx, 1, label)
}
