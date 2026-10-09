package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
)

func testApplication() *application {
	return &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestReadJSON(t *testing.T) {
	app := testApplication()
	for _, tc := range []struct{ name, body, want string }{
		{"valid", `{"name":"order"}`, ""}, {"trailing whitespace", "{\"name\":\"order\"}\n\t ", ""},
		{"empty", "", "empty"}, {"malformed", "{", "badly-formed"},
		{"syntax", `{"name":!}`, "badly-formed"}, {"wrong field type", `{"name":42}`, "incorrect JSON type"},
		{"wrong top level", `[]`, "incorrect JSON type"}, {"unknown", `{"extra":true}`, "unknown key"},
		{"multiple", `{"name":"a"} {"name":"b"}`, "single JSON value"},
		{"trailing junk", `{"name":"a"} junk`, "single JSON value"},
		{"oversized", `{"name":"` + strings.Repeat("x", 1_048_576) + `"}`, "larger than 1048576"},
		{"oversized whitespace", `{"name":"a"}` + strings.Repeat(" ", 1_048_576), "single JSON value"},
		{"at limit", `{"name":"` + strings.Repeat("x", 1_048_576-len(`{"name":""}`)) + `"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var dst struct {
				Name string `json:"name"`
			}
			err := app.readJSON(httptest.NewRecorder(), r, &dst)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if dst.Name == "" {
					t.Fatal("valid input not decoded")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v; want containing %q", err, tc.want)
			}
		})
	}
	raw := json.RawMessage(`{"number":9007199254740993}`)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"payload":`+string(raw)+`}`))
	var dst struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := app.readJSON(httptest.NewRecorder(), r, &dst); err != nil {
		t.Fatal(err)
	}
	if string(dst.Payload) != string(raw) {
		t.Fatalf("payload precision lost: %s", dst.Payload)
	}
}

func TestReadJSONInvalidDestinationPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("invalid destination did not panic")
		}
	}()
	testApplication().readJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)), nil)
}

func TestReadIDParams(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"positive", "42", true}, {"zero", "0", false}, {"negative", "-1", false},
		{"non integer", "abc", false}, {"missing", "", false}, {"overflow", strings.Repeat("9", 30), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			params := httprouter.Params{{Key: "id", Value: tc.value}, {Key: "webhook_id", Value: "7"}}
			r = r.WithContext(context.WithValue(r.Context(), httprouter.ParamsKey, params))
			ids, err := testApplication().readIDParams(r, "id", "webhook_id")
			if !tc.valid {
				if err == nil {
					t.Fatal("invalid ID accepted")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(ids, []int{42, 7}) {
				t.Fatalf("ids=%v error=%v", ids, err)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	app := testApplication()
	w := httptest.NewRecorder()
	if err := app.writeJSON(w, envelope{"ok": true}, http.StatusCreated, http.Header{"X-Test": []string{"one", "two"}}); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusCreated || w.Header().Get("Content-Type") != "application/json" || len(w.Header().Values("X-Test")) != 2 {
		t.Fatalf("response=%v", w.Result())
	}
	var got map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !got["ok"] {
		t.Fatalf("body=%s error=%v", w.Body, err)
	}
	if !strings.HasSuffix(w.Body.String(), "\n") {
		t.Fatal("JSON response missing newline")
	}
	w = httptest.NewRecorder()
	if err := app.writeJSON(w, make(chan int), http.StatusCreated, nil); err == nil {
		t.Fatal("unsupported JSON value accepted")
	}
	if w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
		t.Fatal("encoding failure partially wrote response")
	}
}

func TestBackgroundRecoversAndCompletes(t *testing.T) {
	app := testApplication()
	completed := make(chan struct{})
	app.background(func() { close(completed); panic("test panic") })
	done := make(chan struct{})
	go func() { app.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background panic left wait group blocked")
	}
	select {
	case <-completed:
	default:
		t.Fatal("background task did not run")
	}
}
