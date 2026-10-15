package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestPanicRecoveryAndRequestID(t *testing.T) {
	var logs bytes.Buffer
	handler := Middleware(slog.New(slog.NewJSONHandler(&logs, nil)), 1024, 1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			_, _ = w.Write([]byte("partial"))
			panic("private payload")
		}
		JSON(w, 200, map[string]string{"ok": "yes"})
	}))
	for _, path := range []string{"/panic", "/normal"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		want := 200
		if path == "/panic" {
			want = 500
		}
		if w.Code != want || w.Header().Get("X-Request-ID") == "" || strings.Contains(w.Body.String(), "partial") || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("response: %d %s", w.Code, w.Body)
		}
	}
	if strings.Count(logs.String(), "request_id") != 2 || strings.Contains(logs.String(), "private payload") {
		t.Fatalf("logs: %s", logs.String())
	}
}

func TestConcurrencyLimitHealthBypass(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := Middleware(quietLogger(), 1024, 1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fingerprint" {
			close(entered)
			<-release
		}
		w.WriteHeader(200)
	}))
	go func() {
		defer close(finished)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/fingerprint", nil))
	}()
	<-entered
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/fingerprint", nil))
	if w.Code != 503 {
		t.Errorf("expected busy, got %d", w.Code)
	}
	h := httptest.NewRecorder()
	handler.ServeHTTP(h, httptest.NewRequest("GET", "/health", nil))
	if h.Code != 200 {
		t.Errorf("health must bypass work slots: %d", h.Code)
	}
	close(release)
	<-finished
}

func TestBodyLimit(t *testing.T) {
	handler := Middleware(quietLogger(), 4, 1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			Failure(w, r, 413, "too_large", "body too large", nil)
			return
		}
		w.WriteHeader(200)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/fingerprint", strings.NewReader("12345")))
	if w.Code != 413 {
		t.Fatalf("got %d", w.Code)
	}
}
