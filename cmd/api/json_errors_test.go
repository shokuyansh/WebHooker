package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadJSONPropagatesReadError(t *testing.T) {
	want := errors.New("request connection failed")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = io.NopCloser(errorReader{err: want})
	var input struct{ Name string }
	if err := testApplication().readJSON(httptest.NewRecorder(), r, &input); !errors.Is(err, want) {
		t.Fatalf("read error = %v; want %v", err, want)
	}
}

type errorResponseWriter struct {
	header http.Header
	err    error
}

func (w *errorResponseWriter) Header() http.Header { return w.header }
func (w *errorResponseWriter) WriteHeader(int)     {}
func (w *errorResponseWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestWriteJSONPropagatesWriteError(t *testing.T) {
	want := errors.New("response connection failed")
	w := &errorResponseWriter{header: make(http.Header), err: want}
	if err := testApplication().writeJSON(w, envelope{"ok": true}, http.StatusOK, nil); !errors.Is(err, want) {
		t.Fatalf("write error = %v; want %v", err, want)
	}
}
