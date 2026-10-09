//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shokuyansh/Webhooker/internal/data"
	"github.com/shokuyansh/Webhooker/internal/urlguard"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func useDeliveryTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	saved := client
	replacement := *saved
	replacement.Transport = transport
	client = &replacement
	t.Cleanup(func() { replacement.CloseIdleConnections(); client = saved })
}
func callbackServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	// Only this test transport maps virtual public callbacks to a local server.
	// The production dialer is tested independently and remains unchanged.
	useDeliveryTransport(t, transport)
	return server
}
func storedDelivery(t *testing.T, app *application, d *data.Delivery) *data.Delivery {
	t.Helper()
	got, err := app.models.Deliveries.Get(d.WebHookID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func dueNow(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	if _, err := db.Exec("UPDATE deliveries SET next_attempt_at=NOW()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationDeliverySigning(t *testing.T) {
	type request struct {
		method, path, query string
		headers             http.Header
		body                []byte
		err                 error
	}
	captured := make(chan request, 1)
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		captured <- request{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body, err}
		w.WriteHeader(204)
	}))
	app, _, d, event, hook := deliveryFixture(t, "http://callback.example.test/hook?token=abc")
	before := time.Now()
	if err := app.performDelivery(d, context.Background()); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	var got request
	select {
	case got = <-captured:
	case <-time.After(time.Second):
		t.Fatal("callback request missing")
	}
	if got.err != nil || got.method != "POST" || got.path != "/hook" || got.query != "token=abc" || !bytes.Equal(got.body, event.Payload) {
		t.Fatalf("callback=%+v payload=%s", got, event.Payload)
	}
	if got.headers.Get("Content-Type") != "application/json" || got.headers.Get("Accept") != "application/json" || got.headers.Get("webhook-id") != strconv.FormatInt(d.ID, 10) {
		t.Fatalf("headers=%v", got.headers)
	}
	timestamp := got.headers.Get("webhook-timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if unix < before.Unix() || unix > after.Unix() {
		t.Fatalf("timestamp=%d outside request interval", unix)
	}
	mac := hmac.New(sha256.New, hook.SigningSecret)
	mac.Write([]byte(strconv.FormatInt(d.ID, 10) + "." + timestamp + "."))
	mac.Write(got.body)
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got.headers.Get("webhook-signature") != want {
		t.Fatalf("signature=%q want=%q", got.headers.Get("webhook-signature"), want)
	}
	stored := storedDelivery(t, app, d)
	if stored.Status != "SUCCESS" || stored.AttemptCount != 1 || stored.ResponseStatus == nil || *stored.ResponseStatus != 204 || stored.LastAttemptAt == nil {
		t.Fatalf("delivery=%+v", stored)
	}
}

func TestIntegrationDeliveryHTTPStatuses(t *testing.T) {
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(r.URL.Path[1:])
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(code)
	}))
	for _, tc := range []struct {
		code     int
		attempts int64
		want     string
	}{
		{200, 0, "SUCCESS"}, {201, 0, "SUCCESS"}, {204, 0, "SUCCESS"}, {299, 0, "SUCCESS"},
		{300, 0, "FAILED"}, {301, 0, "FAILED"}, {302, 0, "FAILED"}, {400, 0, "FAILED"}, {401, 0, "FAILED"}, {403, 0, "FAILED"}, {404, 0, "FAILED"},
		{408, 0, "RETRYING"}, {429, 0, "RETRYING"}, {500, 0, "RETRYING"}, {503, 0, "RETRYING"}, {599, 0, "RETRYING"},
		{408, 4, "FAILED"}, {429, 4, "FAILED"}, {503, 4, "FAILED"}, {204, 4, "SUCCESS"},
	} {
		t.Run(fmt.Sprintf("status_%d_attempt_%d", tc.code, tc.attempts+1), func(t *testing.T) {
			app, db, d, _, _ := deliveryFixture(t, fmt.Sprintf("http://callback.example.test/%d", tc.code))
			if _, err := db.Exec("UPDATE deliveries SET attempt_count=$1 WHERE id=$2", tc.attempts, d.ID); err != nil {
				t.Fatal(err)
			}
			d.AttemptCount = tc.attempts
			before := time.Now()
			if err := app.performDelivery(d, context.Background()); err != nil {
				t.Fatal(err)
			}
			got := storedDelivery(t, app, d)
			if got.Status != tc.want || got.AttemptCount != tc.attempts+1 || got.ResponseStatus == nil || *got.ResponseStatus != tc.code || got.LastAttemptAt == nil {
				t.Fatalf("persisted=%+v; want status=%s attempts=%d code=%d", got, tc.want, tc.attempts+1, tc.code)
			}
			if tc.want == "RETRYING" {
				upper := 5 * time.Minute * time.Duration(1<<uint(tc.attempts))
				if got.NextAttemptAt.Before(before.Add(-time.Second)) || got.NextAttemptAt.After(time.Now().Add(upper+time.Second)) {
					t.Fatalf("retry outside full-jitter bounds: %s", got.NextAttemptAt)
				}
			}
		})
	}
}

func TestIntegrationDeliveryNetworkErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		attempts int64
		want     string
	}{
		{"connection failure", net.ErrClosed, 0, "RETRYING"},
		{"request deadline", context.DeadlineExceeded, 0, "RETRYING"},
		{"blocked destination", urlguard.ErrBlockedDestination, 0, "FAILED"},
		{"wrapped blocked destination", fmt.Errorf("dial: %w", urlguard.ErrBlockedDestination), 0, "FAILED"},
		{"fifth failure", net.ErrClosed, 4, "FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useDeliveryTransport(t, roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, tc.err }))
			app, db, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
			oldCode := 503
			if _, err := db.Exec("UPDATE deliveries SET attempt_count=$1,response_status=$2 WHERE id=$3", tc.attempts, oldCode, d.ID); err != nil {
				t.Fatal(err)
			}
			d.AttemptCount = tc.attempts
			d.ResponseStatus = &oldCode
			if err := app.performDelivery(d, context.Background()); err != nil {
				t.Fatal(err)
			}
			got := storedDelivery(t, app, d)
			if got.Status != tc.want || got.AttemptCount != tc.attempts+1 || got.ResponseStatus != nil || got.LastAttemptAt == nil {
				t.Fatalf("network outcome=%+v", got)
			}
		})
	}
	// Missing records are infrastructure errors, not counted callback attempts.
	app, _, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	d.EventID = 999999
	if err := app.performDelivery(d, context.Background()); err == nil || !strings.Contains(err.Error(), "load event") {
		t.Fatalf("event load error=%v", err)
	}
	if got := storedDelivery(t, app, d); got.AttemptCount != 0 || got.Status != "PENDING" {
		t.Fatalf("load error counted an attempt: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := app.performDelivery(d, ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled error=%v", err)
	}
}

func TestIntegrationDeliveryBlocksLoopback(t *testing.T) {
	app, _, d, _, _ := deliveryFixture(t, "http://127.0.0.1:1/hook")
	// Use the untouched production client, including its SSRF-safe dialer.
	if err := app.performDelivery(d, context.Background()); err != nil {
		t.Fatal(err)
	}
	got := storedDelivery(t, app, d)
	if got.Status != "FAILED" || got.AttemptCount != 1 || got.ResponseStatus != nil {
		t.Fatalf("blocked destination=%+v", got)
	}
}

func TestIntegrationDeliveryDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int64
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "http://callback.example.test/target")
			w.WriteHeader(302)
			return
		}
		redirected.Add(1)
		w.WriteHeader(204)
	}))
	app, _, d, _, _ := deliveryFixture(t, "http://callback.example.test/redirect")
	if err := app.performDelivery(d, context.Background()); err != nil {
		t.Fatal(err)
	}
	got := storedDelivery(t, app, d)
	if redirected.Load() != 0 || got.Status != "FAILED" || got.ResponseStatus == nil || *got.ResponseStatus != 302 {
		t.Fatalf("redirects=%d outcome=%+v", redirected.Load(), got)
	}
}

func TestIntegrationRetryThenSuccess(t *testing.T) {
	var calls atomic.Int64
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(503)
		case 2:
			w.WriteHeader(429)
		default:
			w.WriteHeader(204)
		}
	}))
	app, db, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	for attempt := int64(1); attempt <= 3; attempt++ {
		dueNow(t, db, d.ID)
		if err := app.processPendingDeliveries(context.Background()); err != nil {
			t.Fatal(err)
		}
		got := storedDelivery(t, app, d)
		want := "RETRYING"
		code := 503
		if attempt == 2 {
			code = 429
		}
		if attempt == 3 {
			want = "SUCCESS"
			code = 204
		}
		if got.AttemptCount != attempt || got.Status != want || got.ResponseStatus == nil || *got.ResponseStatus != code || calls.Load() != attempt {
			t.Fatalf("attempt=%d calls=%d stored=%+v", attempt, calls.Load(), got)
		}
		if attempt < 3 {
			if _, err := db.Exec("UPDATE deliveries SET next_attempt_at=NOW()+interval '1 hour' WHERE id=$1", d.ID); err != nil {
				t.Fatal(err)
			}
			if err := app.processPendingDeliveries(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != attempt {
				t.Fatal("worker retried before scheduled time")
			}
		}
	}
	dueNow(t, db, d.ID)
	if err := app.processPendingDeliveries(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("successful delivery was selected again")
	}
}

func TestIntegrationRetryStopsAfterFiveAttempts(t *testing.T) {
	var calls atomic.Int64
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	app, db, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	for attempt := int64(1); attempt <= 5; attempt++ {
		dueNow(t, db, d.ID)
		if err := app.processPendingDeliveries(context.Background()); err != nil {
			t.Fatal(err)
		}
		got := storedDelivery(t, app, d)
		want := "RETRYING"
		if attempt == 5 {
			want = "FAILED"
		}
		if got.AttemptCount != attempt || got.Status != want || calls.Load() != attempt {
			t.Fatalf("attempt=%d calls=%d delivery=%+v", attempt, calls.Load(), got)
		}
	}
	dueNow(t, db, d.ID)
	if err := app.processPendingDeliveries(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 5 {
		t.Fatal("terminal delivery retried a sixth time")
	}
}

func TestIntegrationDeliveryCancellation(t *testing.T) {
	app, _, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	started := make(chan struct{})
	useDeliveryTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.performDelivery(d, ctx) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delivery did not stop on cancellation")
	}
	got := storedDelivery(t, app, d)
	if got.AttemptCount != 0 || got.Status != "PENDING" || got.LastAttemptAt != nil {
		t.Fatalf("shutdown cancellation consumed attempt: %+v", got)
	}
}

func TestIntegrationDeliveryRequestTimeout(t *testing.T) {
	useDeliveryTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("request has no deadline")
			return nil, errors.New("request has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > 5*time.Second {
			t.Errorf("unexpected request timeout: %s", remaining)
			return nil, fmt.Errorf("unexpected timeout: %s", remaining)
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	app, _, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	if err := app.performDelivery(d, context.Background()); err != nil {
		t.Fatal(err)
	}
	got := storedDelivery(t, app, d)
	if got.Status != "RETRYING" || got.AttemptCount != 1 || got.ResponseStatus != nil {
		t.Fatalf("timeout outcome=%+v", got)
	}
}

func TestIntegrationWorkerCancellation(t *testing.T) {
	app, db, d, _, _ := deliveryFixture(t, "http://callback.example.test/hook")
	dueNow(t, db, d.ID)
	started := make(chan struct{})
	useDeliveryTransport(t, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	app.background(func() { app.runDeliveryWorker(ctx) })
	go func() { app.wg.Wait(); close(done) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start delivery")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker prevented graceful shutdown")
	}
	if got := storedDelivery(t, app, d); got.AttemptCount != 0 {
		t.Fatalf("canceled worker consumed attempt: %+v", got)
	}
}

func TestIntegrationWorkerContinuesAfterDeliveryError(t *testing.T) {
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	app, db, bad, e, hook := deliveryFixture(t, "://invalid-request-url")
	valid := data.WebHook{ProjectID: e.ProjectID, CallbackURL: "http://callback.example.test/hook", Events: hook.Events, SigningSecret: hook.SigningSecret}
	if err := app.models.Webhooks.Insert(&valid); err != nil {
		t.Fatal(err)
	}
	good := data.Delivery{EventID: e.ID, WebHookID: valid.ID}
	if err := app.models.Deliveries.Create(&good); err != nil {
		t.Fatal(err)
	}
	dueNow(t, db, bad.ID)
	dueNow(t, db, good.ID)
	if err := app.processPendingDeliveries(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := storedDelivery(t, app, bad); got.AttemptCount != 0 {
		t.Fatalf("malformed request counted attempt: %+v", got)
	}
	if got := storedDelivery(t, app, &good); got.Status != "SUCCESS" || got.AttemptCount != 1 {
		t.Fatalf("worker stopped after earlier error: %+v", got)
	}
}

func TestIntegrationEndToEndDelivery(t *testing.T) {
	received := make(chan []byte, 1)
	callbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		received <- body
		w.WriteHeader(204)
	}))
	app, db := integrationApplication(t)
	h := app.routes()
	p := apiProject(t, h, "Orders")
	hook, _ := apiWebhook(t, h, p.ProjectID, "http://callback.example.test/hook", "order.paid")
	event := resource[data.Event](t, apiRequest(t, h, "POST", fmt.Sprintf("/v1/projects/%d/events", p.ProjectID), map[string]any{"type": "order.paid", "payload": json.RawMessage(`{"id":9007199254740993}`)}, 201), "event")
	deliveries, err := app.models.Deliveries.DeliveriesForWebhook(hook.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("fan-out=%v error=%v", deliveries, err)
	}
	dueNow(t, db, deliveries[0].ID)
	if err := app.processPendingDeliveries(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		var got map[string]json.RawMessage
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if string(got["id"]) != "9007199254740993" {
			t.Fatalf("callback lost precision: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("no callback received")
	}
	path := fmt.Sprintf("/v1/projects/%d/webhooks/%d/deliveries/%d", p.ProjectID, hook.ID, deliveries[0].ID)
	got := resource[data.Delivery](t, apiRequest(t, h, "GET", path, nil, 200), "delivery")
	if got.EventID != event.ID || got.Status != "SUCCESS" || got.AttemptCount != 1 || got.ResponseStatus == nil || *got.ResponseStatus != 204 {
		t.Fatalf("history=%+v", got)
	}
}
