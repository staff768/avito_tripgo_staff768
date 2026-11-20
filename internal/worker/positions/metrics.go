package positions

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type instruments struct {
	checks   metric.Int64Counter
	rowsRead metric.Int64Histogram
	requests metric.Int64Counter
	duration metric.Float64Histogram
}

func (p *Pool) newInstruments(meter metric.Meter) (instruments, error) {
	checks, err := meter.Int64Counter("tripgo.stale_checks", metric.WithUnit("{check}"))
	if err != nil {
		return instruments{}, err
	}
	rowsRead, err := meter.Int64Histogram("tripgo.stale_checks.rows_read", metric.WithUnit("{row}"),
		metric.WithExplicitBucketBoundaries(0, 10, 50, 100, 200, 500, 1000, 5000))
	if err != nil {
		return instruments{}, err
	}
	requests, err := meter.Int64Counter("tripgo.position_requests", metric.WithUnit("{request}"))
	if err != nil {
		return instruments{}, err
	}
	duration, err := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5))
	if err != nil {
		return instruments{}, err
	}
	queue, err := meter.Int64ObservableGauge("tripgo.position_requests.queue.size", metric.WithUnit("{task}"))
	if err != nil {
		return instruments{}, err
	}
	active, err := meter.Int64ObservableGauge("tripgo.position_requests.workers.active", metric.WithUnit("{worker}"))
	if err != nil {
		return instruments{}, err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		p.mu.Lock()
		queued, busy := p.queue.Len(), p.active
		p.mu.Unlock()
		observer.ObserveInt64(queue, int64(queued))
		observer.ObserveInt64(active, busy)
		return nil
	}, queue, active)
	return instruments{checks: checks, rowsRead: rowsRead, requests: requests, duration: duration}, err
}

func resultLabel(result string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("result", result))
}
