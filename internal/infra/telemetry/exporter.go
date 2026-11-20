package telemetry

import (
	"context"
	"log/slog"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type traceExporter struct {
	sdktrace.SpanExporter
	logger *slog.Logger
}

func (e *traceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.SpanExporter.ExportSpans(ctx, spans); err != nil {
		e.logger.WarnContext(ctx, "trace export unavailable", "error", err)
	}
	return nil
}

type metricExporter struct {
	sdkmetric.Exporter
	logger *slog.Logger
}

func (e *metricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	if err := e.Exporter.Export(ctx, data); err != nil {
		e.logger.WarnContext(ctx, "metric export unavailable", "error", err)
	}
	return nil
}
