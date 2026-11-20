package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/http/domainerror"
)

type buffer struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *buffer) Header() http.Header { return b.header }
func (b *buffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *buffer) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(data)
}

func (b *buffer) Status() int {
	if b.status == 0 {
		return http.StatusOK
	}
	return b.status
}

type requestState struct {
	panicked bool
	request  *http.Request
}
type Stack struct {
	logger     *slog.Logger
	duration   metric.Float64Histogram
	active     metric.Int64UpDownCounter
	tracer     trace.TracerProvider
	propagator propagation.TextMapPropagator
}

func New(logger *slog.Logger, tp trace.TracerProvider, mp metric.MeterProvider, propagator propagation.TextMapPropagator) (*Stack, error) {
	meter := mp.Meter("tripgo/http")
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("create HTTP histogram: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create active requests counter: %w", err)
	}
	return &Stack{logger: logger, duration: duration, active: active, tracer: tp, propagator: propagator}, nil
}

func (s *Stack) Wrap(router http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routePattern(router, r)
		state := &requestState{panicked: true, request: r}
		output := &buffer{header: make(http.Header)}
		inner := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			state.request = request
			attrs := []attribute.KeyValue{attribute.String("http.request.method", r.Method), attribute.String("http.route", route)}
			span := trace.SpanFromContext(request.Context())
			span.SetName(r.Method + " " + route)
			span.SetAttributes(attrs...)
			start := time.Now()
			defer func() {
				status := output.Status()
				if state.panicked {
					status = http.StatusInternalServerError
					span.SetAttributes(attribute.String("error.type", "panic"))
					span.SetStatus(codes.Error, "panic")
				}
				span.SetAttributes(attribute.Int("http.response.status_code", status))
				level := slog.LevelInfo
				if status >= http.StatusInternalServerError {
					level = slog.LevelError
				}
				s.logger.Log(request.Context(), level, "request handled", "method", r.Method, "route", route, "status", status, "duration_ms", time.Since(start).Milliseconds())
			}()
			s.active.Add(request.Context(), 1, metric.WithAttributes(attrs...))
			defer func() {
				s.active.Add(request.Context(), -1, metric.WithAttributes(attrs...))
				status := output.Status()
				if state.panicked {
					status = http.StatusInternalServerError
				}
				metricAttrs := []attribute.KeyValue{attrs[0], attrs[1], attribute.Int("http.response.status_code", status)}
				s.duration.Record(request.Context(), time.Since(start).Seconds(), metric.WithAttributes(metricAttrs...))
			}()
			router.ServeHTTP(writer, request)
			state.panicked = false
		})
		tracing := otelhttp.NewHandler(inner, r.Method+" "+route, otelhttp.WithTracerProvider(s.tracer), otelhttp.WithMeterProvider(noop.NewMeterProvider()), otelhttp.WithPropagators(s.propagator))
		s.recover(w, output, state, func() { tracing.ServeHTTP(output, r) })
	})
}

func (s *Stack) recover(w http.ResponseWriter, output *buffer, state *requestState, next func()) {
	defer func() {
		if value := recover(); value != nil {
			s.logger.ErrorContext(state.request.Context(), "panic recovered", "panic", fmt.Sprint(value), "stack", string(debug.Stack()))
			_, problem := domainerror.Problem(fmt.Errorf("panic: %v", value))
			output.body.Reset()
			output.header = make(http.Header)
			output.status = 0
			output.header.Set("Content-Type", "application/problem+json")
			output.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(output).Encode(problem); err != nil {
				s.logger.ErrorContext(state.request.Context(), "encode panic response", "error", err)
			}
		}
		for key, values := range output.header {
			w.Header()[key] = values
		}
		w.WriteHeader(output.Status())
		if _, err := w.Write(output.body.Bytes()); err != nil {
			s.logger.ErrorContext(state.request.Context(), "write response failed", "error", err)
		}
	}()
	next()
}

func routePattern(router http.Handler, r *http.Request) string {
	if routes, ok := router.(chi.Routes); ok {
		match := chi.NewRouteContext()
		if routes.Match(match, r.Method, r.URL.Path) {
			return match.RoutePattern()
		}
	}
	return "unmatched"
}
