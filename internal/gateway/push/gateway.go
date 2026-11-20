package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type Gateway struct {
	endpoint   string
	client     *http.Client
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

type statusError struct{ code int }

func (e *statusError) Error() string {
	return fmt.Sprintf("push service returned HTTP %d", e.code)
}

func New(baseURL string, timeout time.Duration, tp trace.TracerProvider, propagator propagation.TextMapPropagator) (*Gateway, error) {
	endpoint, err := url.JoinPath(baseURL, "api/v1/push")
	if err != nil {
		return nil, fmt.Errorf("build Push Service endpoint: %w", err)
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			ForceAttemptHTTP2:     true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Gateway{endpoint: endpoint, client: client, tracer: tp.Tracer("tripgo/push"), propagator: propagator}, nil
}

func (g *Gateway) RequestPosition(ctx context.Context, trip model.StaleTrip) (resultErr error) {
	ctx, span := g.tracer.Start(ctx, "push.request_position", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("http.request.method", http.MethodPost)))
	defer span.End()
	defer func() {
		if resultErr != nil {
			span.RecordError(resultErr)
			span.SetStatus(codes.Error, "push request failed")
		}
	}()

	payload := struct {
		RequestID   string            `json:"request_id"`
		RecipientID uuid.UUID         `json:"recipient_id"`
		Kind        string            `json:"kind"`
		Data        map[string]string `json:"data"`
	}{
		RequestID: uuid.NewString(), RecipientID: trip.DriverID, Kind: "REQUEST_POSITION",
		Data: map[string]string{"trip_id": trip.ID.String()},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	g.propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode != http.StatusAccepted {
		return &statusError{code: resp.StatusCode}
	}
	if readErr != nil {
		return fmt.Errorf("read push response: %w", readErr)
	}
	return nil
}

func (g *Gateway) Close() { g.client.CloseIdleConnections() }
