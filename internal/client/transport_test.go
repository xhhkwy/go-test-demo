package client

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"banner-fingerprint/internal/model"
)

func echo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/fingerprint" || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "bad request", 400)
		return
	}
	var scans []model.Scan
	_ = json.NewDecoder(r.Body).Decode(&scans)
	results := make([]model.Result, len(scans))
	for i, s := range scans {
		results[i] = model.Unknown(s)
	}
	_ = json.NewEncoder(w).Encode(results)
}

func TestOneHundredRoundTrips(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(echo))
	defer s.Close()
	c := New(s.URL, time.Second)
	defer c.Close()
	for i := 0; i < 100; i++ {
		results, err := c.Fingerprint(context.Background(), []model.Scan{{IP: "1.2.3.4", Port: 22}})
		if err != nil || len(results) != 1 || results[0].Protocol != "unknown" {
			t.Fatalf("round trip %d: %+v %v", i, results, err)
		}
	}
}

func TestConnectionFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	c := New("http://"+addr, 100*time.Millisecond)
	defer c.Close()
	if _, err := c.Fingerprint(context.Background(), []model.Scan{}); err == nil {
		t.Fatal("expected connection failure")
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer s.Close()
	c := New(s.URL, 50*time.Millisecond)
	defer c.Close()
	if _, err := c.Fingerprint(context.Background(), []model.Scan{}); err == nil {
		t.Fatal("expected timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Fingerprint(ctx, []model.Scan{}); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestFailedAndInvalidResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"HTTP failure": {503, `{"error":{"code":"busy","message":"try later"}}`},
		"redirect":     {302, ""},
		"bad JSON":     {200, "bad"},
		"null":         {200, "null"},
		"count":        {200, "[]"},
		"order":        {200, `[{"ip":"wrong","port":22,"protocol":"unknown","confidence":0}]`},
		"range":        {200, `[{"ip":"1.2.3.4","port":22,"protocol":"SSH","confidence":2}]`},
		"trailing":     {200, "[] []"},
	} {
		t.Run(name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			c := New(s.URL, time.Second)
			defer c.Close()
			_, err := c.Fingerprint(context.Background(), []model.Scan{{IP: "1.2.3.4", Port: 22}})
			if err == nil || strings.Contains(err.Error(), "<nil>") {
				t.Fatalf("expected useful failure, got %v", err)
			}
		})
	}
}
