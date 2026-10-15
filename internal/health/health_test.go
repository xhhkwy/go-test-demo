package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	for _, tc := range []struct {
		code int
		body string
		ok   bool
	}{
		{200, `{"status":"ok"}`, true}, {503, `{"status":"ok"}`, false}, {200, `{"status":"starting"}`, false}, {200, "bad", false}, {302, "", false},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); _, _ = w.Write([]byte(tc.body)) }))
		err := Probe(context.Background(), s.URL, time.Second)
		s.Close()
		if (err == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}
func TestProbeUnavailable(t *testing.T) {
	if err := Probe(context.Background(), "http://127.0.0.1:1/health", 50*time.Millisecond); err == nil {
		t.Fatal("expected unavailable")
	}
	if err := Probe(context.Background(), "bad", 0); err == nil {
		t.Fatal("expected invalid timeout")
	}
}
