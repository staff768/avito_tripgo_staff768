package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/staff768/avito_tripgo_staff768/internal/infra/postgres"
)

func TestDatabaseSignals(t *testing.T) {
	const tripsTable = "trips"
	for _, tc := range []struct {
		name       string
		sql        string
		operation  string
		collection string
		err        error
		errorType  string
	}{
		{name: "select", sql: "SELECT id FROM trips WHERE id=$1", operation: "SELECT", collection: tripsTable},
		{name: "insert", sql: "INSERT INTO trip_status_history (trip_id) VALUES ($1)", operation: "INSERT", collection: "trip_status_history"},
		{name: "unique", sql: "INSERT INTO trips (id) VALUES ($1)", operation: "INSERT", collection: tripsTable, err: &pgconn.PgError{Code: "23505"}, errorType: "23505"},
		{name: "timeout", sql: "UPDATE trips SET status=$1", operation: "UPDATE", collection: tripsTable, err: context.DeadlineExceeded, errorType: "timeout"},
		{name: "canceled", sql: "BEGIN", operation: "BEGIN", err: context.Canceled, errorType: "canceled"},
		{name: "unknown", sql: "COMMIT", operation: "COMMIT", err: errors.New("failure"), errorType: "database_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := metric.NewManualReader()
			mp := metric.NewMeterProvider(metric.WithReader(reader))
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			t.Cleanup(func() {
				if err := tp.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
				if err := mp.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			tracer, err := postgres.NewTracer(tp, mp)
			if err != nil {
				t.Fatal(err)
			}
			ctx, parent := tp.Tracer("test").Start(context.Background(), "request")
			queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: tc.sql, Args: []any{"sensitive-parameter"}})
			tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: tc.err})
			parent.End()
			spans := exporter.GetSpans()
			if len(spans) != 2 || spans[0].Parent.SpanID() != spans[1].SpanContext.SpanID() {
				t.Fatalf("invalid span tree: %+v", spans)
			}
			attributes := attribute.NewSet(spans[0].Attributes...)
			assertAttr(t, attributes, "db.operation.name", tc.operation)
			assertAttr(t, attributes, "db.collection.name", tc.collection)
			if tc.err != nil {
				assertAttr(t, attributes, "error.type", tc.errorType)
				if spans[0].Status.Code != codes.Error {
					t.Fatal("error span is not marked")
				}
			}
			if attributes.HasValue("pgx.query.parameters") || attributes.HasValue("db.query.text") {
				t.Fatal("query payload in trace")
			}
			var data metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &data); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, scope := range data.ScopeMetrics {
				for _, m := range scope.Metrics {
					if m.Name != "db.client.operation.duration" {
						continue
					}
					found = true
					points := m.Data.(metricdata.Histogram[float64]).DataPoints
					if len(points) != 1 || points[0].Count != 1 {
						t.Fatal("bad database histogram")
					}
					assertAttr(t, points[0].Attributes, "db.operation.name", tc.operation)
					assertAttr(t, points[0].Attributes, "db.collection.name", tc.collection)
					assertAttr(t, points[0].Attributes, "error.type", tc.errorType)
				}
			}
			if !found {
				t.Fatal("database metric missing")
			}
		})
	}
}

func assertAttr(t *testing.T, attributes attribute.Set, key, expected string) {
	t.Helper()
	value, exists := attributes.Value(attribute.Key(key))
	if !exists || value.AsString() != expected {
		t.Fatalf("%s=%s expected %s", key, value.AsString(), expected)
	}
}
