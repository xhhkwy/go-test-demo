package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"banner-fingerprint/internal/config"
	"banner-fingerprint/internal/fingerprint"
	"banner-fingerprint/internal/model"
)

func testAPI(t *testing.T, c config.Server) http.Handler {
	t.Helper()
	e, err := fingerprint.Load("../../rules/fingerprints.json")
	if err != nil {
		t.Fatal(err)
	}
	return Middleware(quietLogger(), c.MaxBodyBytes, c.MaxConcurrent, Router(e, c, func() bool { return true }))
}
func defaults() config.Server {
	return config.Server{MaxBodyBytes: model.DefaultMaxBodyBytes, MaxBatch: model.DefaultMaxBatch, MaxBannerBytes: model.DefaultMaxBannerBytes, MaxConcurrent: 4}
}
func request(h http.Handler, method, path, media, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", media)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const validInput = `[{"ip":"::1","port":22,"banner":""}]`

func TestAPIErrorsAndRecovery(t *testing.T) {
	c := defaults()
	c.MaxBodyBytes = 256
	c.MaxBatch = 2
	c.MaxBannerBytes = 32
	h := testAPI(t, c)
	for name, tc := range map[string]struct {
		body, media string
		status      int
		index       bool
	}{
		"media":           {validInput, "text/plain", 415, false},
		"syntax":          {"[", "application/json", 400, false},
		"hex JSON":        {`[{"ip":"::1","port":22,"banner":"\x00"}]`, "application/json", 400, false},
		"null":            {"null", "application/json", 400, false},
		"object":          {"{}", "application/json", 400, false},
		"trailing":        {validInput + " []", "application/json", 400, false},
		"null row":        {"[null]", "application/json", 400, true},
		"missing banner":  {`[{"ip":"::1","port":22}]`, "application/json", 400, true},
		"null banner":     {`[{"ip":"::1","port":22,"banner":null}]`, "application/json", 400, true},
		"IP":              {`[{"ip":"host","port":22,"banner":""}]`, "application/json", 400, true},
		"zone":            {`[{"ip":"fe80::1%eth0","port":22,"banner":""}]`, "application/json", 400, true},
		"port":            {`[{"ip":"::1","port":0,"banner":""}]`, "application/json", 400, true},
		"fractional port": {`[{"ip":"::1","port":22.5,"banner":""}]`, "application/json", 400, true},
		"extra field":     {`[{"ip":"::1","port":22,"banner":"","extra":true}]`, "application/json", 400, true},
		"encoding":        {`[{"ip":"::1","port":22,"banner":"","banner_encoding":"other"}]`, "application/json", 400, true},
		"escaped":         {`[{"ip":"::1","port":22,"banner":"\\xGG","banner_encoding":"escaped"}]`, "application/json", 400, true},
		"base64":          {`[{"ip":"::1","port":22,"banner":"?","banner_encoding":"base64"}]`, "application/json", 400, true},
		"body size":       {strings.Repeat(" ", 257), "application/json", 413, false},
		"batch size":      {"[" + strings.TrimSuffix(strings.Repeat(strings.Trim(validInput, "[]")+",", 3), ",") + "]", "application/json", 413, false},
		"banner size":     {`[{"ip":"::1","port":22,"banner":"` + strings.Repeat("x", 33) + `"}]`, "application/json", 413, true},
	} {
		t.Run(name, func(t *testing.T) {
			w := request(h, "POST", "/fingerprint", tc.media, tc.body)
			var errBody model.APIError
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &errBody) != nil || errBody.Error.Code == "" || (errBody.Error.Index != nil) != tc.index || errBody.Error.RequestID == "" {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
			if w := request(h, "GET", "/health", "", ""); w.Code != 200 {
				t.Fatal("health failed after bad input")
			}
			if w := request(h, "POST", "/fingerprint", "application/json", validInput); w.Code != 200 {
				t.Fatalf("valid request failed: %s", w.Body)
			}
		})
	}
}
func TestAPIGoldenAndContract(t *testing.T) {
	h := testAPI(t, defaults())
	body, err := os.ReadFile("../../testdata/scans.json")
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/fingerprint", "application/json; charset=utf-8", string(body))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	expected, err := os.ReadFile("../../testdata/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var actual, want []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("golden mismatch: %s", w.Body)
	}
	for _, r := range actual {
		if len(r) != 7 {
			t.Fatalf("fields: %+v", r)
		}
	}
	for _, body := range []string{"[]", "[" + strings.Trim(validInput, "[]") + "," + strings.Trim(validInput, "[]") + "]"} {
		w := request(h, "POST", "/fingerprint", "application/json", body)
		if w.Code != 200 || bytes.Equal(w.Body.Bytes(), []byte("null\n")) {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var results []model.Result
		_ = json.Unmarshal(w.Body.Bytes(), &results)
		if body == "[]" && len(results) != 0 {
			t.Fatal("empty batch")
		}
		if body != "[]" && (len(results) != 2 || results[0] != results[1]) {
			t.Fatal("duplicates lost")
		}
	}
}
func TestAPIMethods(t *testing.T) {
	h := testAPI(t, defaults())
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/fingerprint", 405}, {"POST", "/health", 405}, {"GET", "/missing", 404}} {
		w := request(h, tc.method, tc.path, "application/json", "[]")
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}
func TestAPIInvalidUTF8(t *testing.T) {
	h := testAPI(t, defaults())
	raw := append([]byte(`[{"ip":"::1","port":22,"banner":"`), 0xff)
	raw = append(raw, []byte(`"}]`)...)
	r := httptest.NewRequest("POST", "/fingerprint", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
func TestAPIBodyReadFailure(t *testing.T) {
	h := testAPI(t, defaults())
	r := httptest.NewRequest("POST", "/fingerprint", nil)
	r.Header.Set("Content-Type", "application/json")
	r.Body = io.NopCloser(errorReader{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
