package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/http/middleware"
)

func TestSignalsAndRecovery(t *testing.T) {
	for _, panicRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "panic"}[panicRequest], func(t *testing.T) {
			var logs bytes.Buffer
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
			stack, err := middleware.New(testLogger(&logs), tp, mp, propagation.TraceContext{})
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Get("/trips/{tripId}", func(w http.ResponseWriter, r *http.Request) {
				if panicRequest {
					panic("test panic")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/trips/unique-id", nil)
			request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
			stack.Wrap(router).ServeHTTP(response, request)
			want := http.StatusNoContent
			if panicRequest {
				want = http.StatusInternalServerError
			}
			if response.Code != want {
				t.Fatalf("got %d %s", response.Code, response.Body.String())
			}
			if panicRequest && !bytes.Contains(response.Body.Bytes(), []byte(`"code":"internal_error"`)) {
				t.Fatal("invalid panic response")
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 || spans[0].Name != "GET /trips/{tripId}" || spans[0].SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
				t.Fatalf("bad spans: %+v", spans)
			}
			checkLogs(t, &logs, spans, want, panicRequest)
			var data metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &data); err != nil {
				t.Fatal(err)
			}
			checkMetrics(t, data)
		})
	}
}

func checkLogs(t *testing.T, logs *bytes.Buffer, spans tracetest.SpanStubs, want int, panicRequest bool) {
	t.Helper()
	requests, panics := 0, 0
	decoder := json.NewDecoder(logs)
	for decoder.More() {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry["trace_id"] != spans[0].SpanContext.TraceID().String() || entry["span_id"] != spans[0].SpanContext.SpanID().String() {
			t.Fatalf("missing trace correlation: %+v", entry)
		}
		if entry["msg"] == "request handled" {
			requests++
			if entry["route"] != "/trips/{tripId}" || entry["status"] != float64(want) {
				t.Fatalf("bad request log: %+v", entry)
			}
		}
		if entry["msg"] == "panic recovered" {
			panics++
			if entry["stack"] == "" {
				t.Fatal("stack missing")
			}
		}
	}
	if requests != 1 || (panicRequest && panics != 1) {
		t.Fatalf("request logs=%d panics=%d", requests, panics)
	}
}

func checkMetrics(t *testing.T, data metricdata.ResourceMetrics) {
	t.Helper()
	activeFound, durationFound := false, false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "http.server.active_requests":
				activeFound = true
				for _, point := range m.Data.(metricdata.Sum[int64]).DataPoints {
					if point.Value != 0 {
						t.Fatal("active requests leaked")
					}
				}
			case "http.server.request.duration":
				durationFound = true
				points := m.Data.(metricdata.Histogram[float64]).DataPoints
				if len(points) != 1 || points[0].Count != 1 {
					t.Fatal("bad histogram")
				}
				for _, attr := range points[0].Attributes.ToSlice() {
					if attr.Key == "http.route" && attr.Value.AsString() != "/trips/{tripId}" {
						t.Fatal("raw path in metric")
					}
				}
			}
		}
	}
	if !activeFound || !durationFound {
		t.Fatal("HTTP metrics missing")
	}
}

type testHandler struct{ slog.Handler }

func testLogger(out io.Writer) *slog.Logger {
	return slog.New(&testHandler{Handler: slog.NewJSONHandler(out, nil)})
}

func (h *testHandler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanContextFromContext(ctx)
	record.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	return h.Handler.Handle(ctx, record)
}
