package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRecoverPanic(t *testing.T) {
	app := testApplication()
	w := httptest.NewRecorder()
	handler := app.recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("sensitive internal error") }))
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 500 || w.Header().Get("Connection") != "close" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "Server could not process your request." {
		t.Fatalf("unexpected public error: %v", body)
	}
}

func TestRateLimit(t *testing.T) {
	app := testApplication()
	app.config.limiter.enabled = true
	app.config.limiter.rps = 0.000001
	app.config.limiter.burst = 2
	h := app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := func(remote, path string, headers http.Header) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = remote
		r.Header = headers
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 2; i++ {
		if w := request("192.0.2.1:1000", "/v1/projects", nil); w.Code != 204 {
			t.Fatalf("burst request %d status=%d", i, w.Code)
		}
	}
	for i := 0; i < 5; i++ {
		w := request(fmt.Sprintf("192.0.2.1:%d", 2000+i), "/v1/projects", http.Header{"X-Forwarded-For": []string{fmt.Sprintf("8.8.8.%d", i)}, "X-Real-Ip": []string{"1.1.1.1"}})
		if w.Code != 429 {
			t.Fatalf("same-IP request escaped limit: %d", w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] != "rate limit exceeded" {
			t.Fatalf("429 body=%s error=%v", w.Body, err)
		}
	}
	for i := 0; i < 5; i++ {
		if w := request("192.0.2.1:1000", "/v1/healthcheckup", nil); w.Code != 204 {
			t.Fatalf("healthcheck limited: %d", w.Code)
		}
	}
	if w := request("192.0.2.2:1000", "/v1/projects", nil); w.Code != 204 {
		t.Fatalf("different IP shared bucket: %d", w.Code)
	}
	if w := request("[2001:db8::1]:1000", "/v1/projects", nil); w.Code != 204 {
		t.Fatalf("IPv6 rejected: %d", w.Code)
	}
	if w := request("invalid-address", "/v1/projects", nil); w.Code != 500 {
		t.Fatalf("invalid remote address status=%d", w.Code)
	}
}

func TestRateLimitDisabled(t *testing.T) {
	app := testApplication()
	h := app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := 0; i < 20; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
		r.RemoteAddr = "invalid"
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatalf("disabled limiter status=%d", w.Code)
		}
	}
}

func TestRateLimitConcurrentRequests(t *testing.T) {
	app := testApplication()
	app.config.limiter.enabled = true
	app.config.limiter.rps = 0.000001
	app.config.limiter.burst = 5
	h := app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	var accepted, rejected, other atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
			r.RemoteAddr = "192.0.2.3:1000"
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			switch w.Code {
			case 204:
				accepted.Add(1)
			case 429:
				rejected.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 5 || rejected.Load() != 45 || other.Load() != 0 {
		t.Fatalf("accepted=%d rejected=%d other=%d", accepted.Load(), rejected.Load(), other.Load())
	}
}
