package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/infra/telemetry"
)

func TestLogger(t *testing.T) {
	var out bytes.Buffer
	logger := telemetry.NewLogger(&out, slog.LevelInfo, "trip-service")
	logger.Debug("hidden")
	logger.Info("startup")
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
	logger.With("trip_id", "test").WithGroup("details").InfoContext(ctx, "trip created", "status", "active")
	decoder := json.NewDecoder(&out)
	var startup, trip map[string]any
	if err := decoder.Decode(&startup); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&trip); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("debug message was not filtered")
	}
	if startup["msg"] != "startup" || startup["service"] != "trip-service" || startup["trace_id"] != "" || startup["span_id"] != "" {
		t.Fatalf("bad startup log: %+v", startup)
	}
	details := trip["details"].(map[string]any)
	if trip["trip_id"] != "test" || details["trace_id"] != traceID.String() || details["span_id"] != spanID.String() {
		t.Fatalf("bad trace correlation: %+v", trip)
	}
}
