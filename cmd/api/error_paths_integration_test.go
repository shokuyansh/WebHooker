//go:build integration

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIntegrationAPIDatabaseErrors(t *testing.T) {
	app, db := integrationApplication(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	h := app.routes()
	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"project read", "GET", "/v1/projects/1", nil},
		{"project create", "POST", "/v1/projects", map[string]any{"name": "Orders"}},
		{"webhook read", "GET", "/v1/projects/1/webhooks/1", nil},
		{"webhook patch", "PATCH", "/v1/projects/1/webhooks/1", map[string]any{"activated": false}},
		{"webhook delete", "DELETE", "/v1/projects/1/webhooks/1", nil},
		{"webhook list", "GET", "/v1/projects/1/webhooks", nil},
		{"webhook create", "POST", "/v1/webhook", map[string]any{"project_id": 1, "callback_url": "https://example.com", "events_registered": []string{"a"}}},
		{"event create", "POST", "/v1/projects/1/events", map[string]any{"type": "a", "payload": map[string]any{"ok": true}}},
		{"delivery list", "GET", "/v1/deliveries", nil},
		{"delivery history", "GET", "/v1/projects/1/webhooks/1/deliveries", nil},
		{"delivery read", "GET", "/v1/projects/1/webhooks/1/deliveries/1", nil},
	} {
		t.Run(tc.name, func(t *testing.T) { apiRequest(t, h, tc.method, tc.path, tc.body, http.StatusInternalServerError) })
	}
}

type observedBody struct {
	io.Reader
	closed *bool
}

func (b *observedBody) Close() error { *b.closed = true; return nil }

func TestIntegrationDeliveryUpdateFailure(t *testing.T) {
	app, db, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	closed := false
	useDeliveryTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if err := db.Close(); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: &observedBody{Reader: strings.NewReader(""), closed: &closed}, Request: r}, nil
	}))
	err := app.performDelivery(d, context.Background())
	if err == nil || !strings.Contains(err.Error(), "update delivery") {
		t.Fatalf("update failure=%v", err)
	}
	if !closed {
		t.Fatal("callback response body leaked when database update failed")
	}
	var _ io.ReadCloser = (*observedBody)(nil)
}
