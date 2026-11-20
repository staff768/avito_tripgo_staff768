//go:generate go tool mockgen -destination mocks_test.go -package push net/http RoundTripper
package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/mock/gomock"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type responseBody struct {
	io.Reader
	closed bool
}

func (b *responseBody) Close() error {
	b.closed = true
	return nil
}

func TestRequestPosition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "accepted", status: http.StatusAccepted},
		{name: "client error", status: http.StatusBadRequest},
		{name: "server error", status: http.StatusServiceUnavailable},
		{name: "redirect", status: http.StatusTemporaryRedirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway, transport := newTestGateway(t)
			trip := model.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
			body := &responseBody{Reader: strings.NewReader("{}")}
			transport.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assertPushRequest(t, req, trip)
				response := &http.Response{StatusCode: tc.status, Body: body, Header: http.Header{}}
				response.Header.Set("Location", "/redirect")
				return response, nil
			})
			err := gateway.RequestPosition(context.Background(), trip)
			var status *statusError
			if tc.status == http.StatusAccepted {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.As(err, &status) || status.code != tc.status {
				t.Fatalf("push status error = %v, want %d", err, tc.status)
			}
			if !body.closed {
				t.Fatal("response body remained open")
			}
		})
	}
}

func TestPushTransportFailure(t *testing.T) {
	gateway, transport := newTestGateway(t)
	cause := errors.New("network unavailable")
	transport.EXPECT().RoundTrip(gomock.Any()).Return(nil, cause)
	err := gateway.RequestPosition(context.Background(), model.StaleTrip{})
	var status *statusError
	if !errors.Is(err, cause) || errors.As(err, &status) {
		t.Fatalf("push transport error = %v", err)
	}
}

func newTestGateway(t *testing.T) (*Gateway, *MockRoundTripper) {
	t.Helper()
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	gateway, err := New("https://push.example", time.Second, tp, propagation.TraceContext{})
	if err != nil {
		t.Fatal(err)
	}
	transport := NewMockRoundTripper(gomock.NewController(t))
	gateway.client.Transport = transport
	return gateway, transport
}

func assertPushRequest(t *testing.T, req *http.Request, trip model.StaleTrip) {
	t.Helper()
	defer func() { _ = req.Body.Close() }()
	var payload struct {
		RequestID string            `json:"request_id"`
		Recipient uuid.UUID         `json:"recipient_id"`
		Kind      string            `json:"kind"`
		Data      map[string]string `json:"data"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/api/v1/push" || req.Method != http.MethodPost || req.Header.Get("traceparent") == "" ||
		req.Header.Get("Content-Type") != "application/json" || payload.RequestID == "" ||
		payload.Recipient != trip.DriverID || payload.Kind != "REQUEST_POSITION" || payload.Data["trip_id"] != trip.ID.String() {
		t.Fatalf("push request = %v, payload = %+v", req, payload)
	}
}
