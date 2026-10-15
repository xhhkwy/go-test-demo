package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestHealthReadiness(t *testing.T) {
	for _, ready := range []bool{false, true} {
		w := httptest.NewRecorder()
		Health(func() bool { return ready })(w, httptest.NewRequest("GET", "/health", nil))
		expected := 503
		if ready {
			expected = 200
		}
		if w.Code != expected || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("ready %t: %d %s", ready, w.Code, w.Body)
		}
	}
}
