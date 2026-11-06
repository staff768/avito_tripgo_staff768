package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/config"
)

type Telemetry struct {
	logger     *slog.Logger
	Tracer     *sdktrace.TracerProvider
	Meter      *sdkmetric.MeterProvider
	Propagator propagation.TextMapPropagator
	created    metric.Int64Counter
	completed  metric.Int64Counter
}

func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Telemetry, error) {
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", cfg.OTelServiceName), attribute.String("service.version", cfg.ServiceVersion)))
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}
	var te sdktrace.SpanExporter
	var me sdkmetric.Exporter
	switch cfg.OTelProtocol {
	case "http/protobuf":
		te, err = otlptracehttp.New(ctx)
		if err == nil {
			me, err = otlpmetrichttp.New(ctx)
		}
	case "grpc":
		te, err = otlptracegrpc.New(ctx)
		if err == nil {
			me, err = otlpmetricgrpc.New(ctx)
		}
	}
	if err != nil {
		if te != nil {
			err = errors.Join(err, te.Shutdown(ctx))
		}
		return nil, fmt.Errorf("create OTLP exporters: %w", err)
	}
	sampler := Sampler(cfg.OTelSampler, cfg.OTelSamplerArg)
	tracer := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sampler), sdktrace.WithBatcher(&traceExporter{SpanExporter: te, logger: logger}))
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(&metricExporter{Exporter: me, logger: logger}, sdkmetric.WithInterval(cfg.OTelMetricInterval))))
	t := &Telemetry{logger: logger, Tracer: tracer, Meter: meter, Propagator: propagation.TraceContext{}}
	t.created, err = meter.Meter("tripgo").Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"))
	if err == nil {
		t.completed, err = meter.Meter("tripgo").Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"))
	}
	if err != nil {
		return nil, errors.Join(err, t.Shutdown(ctx))
	}
	return t, nil
}

func Sampler(name string, ratio float64) sdktrace.Sampler {
	switch name {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratio)
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}
func (t *Telemetry) Created(ctx context.Context)   { t.created.Add(ctx, 1) }
func (t *Telemetry) Completed(ctx context.Context) { t.completed.Add(ctx, 1) }
func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(t.Meter.Shutdown(ctx), t.Tracer.Shutdown(ctx))
}

func (t *Telemetry) RegisterActiveTrips(repository activeTrips) error {
	gauge, err := t.Meter.Meter("tripgo").Int64ObservableGauge("tripgo.trips.active", metric.WithUnit("{trip}"))
	if err != nil {
		return fmt.Errorf("create active trips gauge: %w", err)
	}
	_, err = t.Meter.Meter("tripgo").RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		count, err := repository.CountActive(ctx)
		if err != nil {
			t.logger.WarnContext(ctx, "active trips collection unavailable", "error", err)
			return nil
		}
		observer.ObserveInt64(gauge, count)
		return nil
	}, gauge)
	return err
}
