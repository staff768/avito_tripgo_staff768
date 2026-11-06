package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type queryInfo struct {
	started    time.Time
	operation  string
	collection string
}
type (
	queryKey struct{}
	Tracer   struct {
		*otelpgx.Tracer
		duration metric.Float64Histogram
	}
)

func NewTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*Tracer, error) {
	duration, err := mp.Meter("tripgo/postgres").Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("create database histogram: %w", err)
	}
	return &Tracer{Tracer: otelpgx.NewTracer(otelpgx.WithTracerProvider(tp), otelpgx.WithMeterProvider(noop.NewMeterProvider()), otelpgx.WithDisableSQLStatementInAttributes(), otelpgx.WithDisableConnectionDetailsInAttributes(), otelpgx.WithDisableAcquireTracer()), duration: duration}, nil
}

func (t *Tracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	fields := strings.Fields(strings.ToUpper(data.SQL))
	info := queryInfo{started: time.Now()}
	if len(fields) > 0 {
		info.operation = fields[0]
	}
	for i, field := range fields {
		if (field == "FROM" || field == "INTO" || field == "UPDATE") && i+1 < len(fields) {
			table := strings.ToLower(fields[i+1])
			if table == "trips" || table == "trip_status_history" {
				info.collection = table
			}
			break
		}
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", info.operation), attribute.String("db.collection.name", info.collection))
	return context.WithValue(ctx, queryKey{}, info)
}

func (t *Tracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	info, ok := ctx.Value(queryKey{}).(queryInfo)
	if ok {
		errorType := ""
		if data.Err != nil {
			errorType = "database_error"
			var pgErr *pgconn.PgError
			if errors.As(data.Err, &pgErr) {
				errorType = pgErr.Code
			}
			if errors.Is(data.Err, context.Canceled) {
				errorType = "canceled"
			}
			if errors.Is(data.Err, context.DeadlineExceeded) {
				errorType = "timeout"
			}
			trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", errorType))
		}
		t.duration.Record(ctx, time.Since(info.started).Seconds(), metric.WithAttributes(attribute.String("db.operation.name", info.operation), attribute.String("db.collection.name", info.collection), attribute.String("error.type", errorType)))
	}
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}
