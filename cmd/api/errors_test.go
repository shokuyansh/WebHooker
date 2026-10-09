package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorResponses(t *testing.T) {
	app := testApplication()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, tc := range []struct {
		name    string
		status  int
		respond func(http.ResponseWriter, *http.Request)
	}{
		{"bad request", 400, func(w http.ResponseWriter, r *http.Request) {
			app.badRequestErrorResponse(w, r, errors.New("bad input"))
		}},
		{"missing", 404, app.notFoundErrorResponse}, {"method", 405, app.methodNotAllowedErrorResponse},
		{"conflict", 409, app.editConflictResponse}, {"limited", 429, app.rateLimitErrorResponse},
		{"validation", 422, func(w http.ResponseWriter, r *http.Request) {
			app.failedValidationResponse(w, r, map[string]string{"name": "required"})
		}},
		{"server", 500, func(w http.ResponseWriter, r *http.Request) {
			app.serverErrorResponse(w, r, errors.New("internal secret"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.respond(w, r)
			if w.Code != tc.status || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d headers=%v", w.Code, w.Header())
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got["error"]) == 0 {
				t.Fatal("error envelope missing")
			}
			if tc.name == "validation" {
				var fields map[string]string
				if err := json.Unmarshal(got["error"], &fields); err != nil || fields["name"] != "required" {
					t.Fatalf("validation=%v error=%v", fields, err)
				}
			}
			if tc.name == "server" && strings.Contains(w.Body.String(), "internal secret") {
				t.Fatal("internal error leaked")
			}
		})
	}
	w := httptest.NewRecorder()
	app.errorResponse(w, r, make(chan int), 400)
	if w.Code != 500 {
		t.Fatalf("error encoding failure status=%d", w.Code)
	}
}
