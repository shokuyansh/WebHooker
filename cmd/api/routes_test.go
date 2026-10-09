package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesWithoutDatabase(t *testing.T) {
	app := testApplication()
	app.config.env = "test"
	h := app.routes()
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"health", http.MethodGet, "/v1/healthcheckup", "", 200},
		{"missing", http.MethodGet, "/missing", "", 404},
		{"method", http.MethodPut, "/v1/projects", "", 405},
		{"invalid project ID", http.MethodGet, "/v1/projects/0", "", 404},
		{"invalid webhook ID", http.MethodGet, "/v1/projects/1/webhooks/bad", "", 404},
		{"invalid delivery ID", http.MethodGet, "/v1/projects/1/webhooks/1/deliveries/-1", "", 404},
		{"invalid event project", http.MethodPost, "/v1/projects/abc/events", `{}`, 400},
		{"malformed project", http.MethodPost, "/v1/projects", "{", 400},
		{"empty project", http.MethodPost, "/v1/projects", `{"name":""}`, 422},
		{"invalid webhook", http.MethodPost, "/v1/webhook", `{"project_id":1,"callback_url":"http://127.0.0.1","events_registered":["a"]}`, 422},
		{"invalid payload", http.MethodPost, "/v1/projects/1/events", `{"type":"a","payload":null}`, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("status=%d; want=%d body=%s", w.Code, tc.status, w.Body)
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("content type=%s", w.Header().Get("Content-Type"))
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if tc.status >= 400 {
				if _, ok := got["error"]; !ok {
					t.Fatal("missing error envelope")
				}
			}
		})
	}
}
