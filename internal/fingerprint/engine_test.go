package fingerprint

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"banner-fingerprint/internal/model"
)

func testEngine(t testing.TB) *Engine {
	t.Helper()
	e, err := Load("../../rules/fingerprints.json")
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func readJSON(t testing.TB, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}
func TestSampleGolden(t *testing.T) {
	e := testEngine(t)
	var scans []model.Scan
	var expected []model.Result
	readJSON(t, "../../testdata/scans.json", &scans)
	readJSON(t, "../../testdata/expected.json", &expected)
	if len(scans) != 20 || len(expected) != 20 {
		t.Fatal("expected twenty samples")
	}
	for i, s := range scans {
		t.Run(s.IP, func(t *testing.T) {
			data, err := Decode(s, model.DefaultMaxBannerBytes)
			if err != nil {
				t.Fatal(err)
			}
			actual := e.Recognize(s, data)
			want := expected[i]
			if math.Abs(actual.Confidence-want.Confidence) > 1e-9 {
				t.Fatalf("confidence: %+v want %+v", actual, want)
			}
			actual.Confidence = want.Confidence
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("actual %+v; expected %+v", actual, want)
			}
		})
	}
}
func TestRecognitionEdges(t *testing.T) {
	e := testEngine(t)
	for name, tc := range map[string]struct{ banner, protocol, product, version string }{
		"empty":                    {"", "unknown", "", ""},
		"port is not evidence":     {"garbage", "unknown", "", ""},
		"SMTP":                     {"220 mail.example ESMTP", "unknown", "", ""},
		"case-insensitive header":  {"HTTP/1.1 200 OK\nsErVeR: NGINX/1.24.0\n", "HTTP", "nginx", "1.24.0"},
		"body ignored":             {"HTTP/1.1 200 OK\r\n\r\nServer: nginx/1.24.0\nSSH-2.0-OpenSSH_9.3", "HTTP", "", ""},
		"no version":               {"HTTP/1.1 200 OK\nServer: nginx", "HTTP", "nginx", ""},
		"jetty parentheses":        {"HTTP/1.1 200 OK\nServer: Jetty(9.4.51.v20230217)", "HTTP", "Jetty", "9.4.51.v20230217"},
		"ssh preamble":             {"notice\nSSH-2.0-OpenSSH_9.3 Debian-1\n", "SSH", "OpenSSH", "9.3"},
		"SSH suffix":               {"SSH-2.0-OpenSSH_8.9p1 Ubuntu-3", "SSH", "OpenSSH", "8.9p1"},
		"missing mysql terminator": {"J\x00\x00\x00\n8.0.32", "unknown", "", ""},
		"bad mysql length":         {"\x02\x00\x00\x00\n8.0.32\x00", "unknown", "", ""},
		"wrong sequence":           {"J\x00\x00\x01\n8.0.32\x00", "unknown", "", ""},
		"truncated mysql":          {"J\x00\x00\x00\n8.0.32\x00", "MySQL", "MySQL", "8.0.32"},
		"binary garbage":           {"\xff\x00\x10\x80", "unknown", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := e.Recognize(model.Scan{IP: "::1", Port: 45678}, []byte(tc.banner))
			if r.Protocol != tc.protocol || r.Product != tc.product || r.Version != tc.version {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestDecodeEncodings(t *testing.T) {
	want := []byte("J\x00\x00\x00\n8.0.32\x00")
	for _, s := range []model.Scan{
		{Banner: string(want)},
		{BannerEncoding: "text", Banner: string(want)},
		{BannerEncoding: "escaped", Banner: `J\x00\x00\x00\n8.0.32\x00`},
		{BannerEncoding: "base64", Banner: base64.StdEncoding.EncodeToString(want)},
	} {
		got, err := Decode(s, len(want))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%+v: %x %v", s, got, err)
		}
	}
	// Text mode preserves a literal escape: no implicit second unquoting.
	got, err := Decode(model.Scan{Banner: `\x00`}, 4)
	if err != nil || string(got) != `\x00` {
		t.Fatal("text was decoded twice")
	}
	for _, s := range []model.Scan{
		{BannerEncoding: "escaped", Banner: `\x0`}, {BannerEncoding: "escaped", Banner: `\xGG`},
		{BannerEncoding: "escaped", Banner: `\`}, {BannerEncoding: "escaped", Banner: `\q`},
		{BannerEncoding: "base64", Banner: "@@"}, {BannerEncoding: "other"},
	} {
		if _, err := Decode(s, 100); err == nil {
			t.Fatalf("accepted %+v", s)
		}
	}
	for _, encoding := range []string{"text", "escaped", "base64"} {
		banner := "12345"
		if encoding == "base64" {
			banner = base64.StdEncoding.EncodeToString([]byte(banner))
		}
		if _, err := Decode(model.Scan{Banner: banner, BannerEncoding: encoding}, 4); !errors.Is(err, ErrBannerTooLarge) {
			t.Fatalf("%s limit: %v", encoding, err)
		}
	}
	if _, err := Decode(model.Scan{Banner: strings.Repeat("x", 65537)}, 65536); !errors.Is(err, ErrBannerTooLarge) {
		t.Fatal(err)
	}
}
func TestExternalRuleChange(t *testing.T) {
	file := minimalRules()
	file.Rules[0].Pattern = "^CUSTOM/([0-9.]+)$"
	file.Rules[0].Product = "CustomSSH"
	data, _ := json.Marshal(file)
	e, err := Compile(data)
	if err != nil {
		t.Fatal(err)
	}
	r := e.Recognize(model.Scan{}, []byte("CUSTOM/1.2"))
	if r.Product != "CustomSSH" || r.Version != "1.2" {
		t.Fatalf("external rule ignored: %+v", r)
	}
}
func FuzzRecognize(f *testing.F) {
	e := testEngine(f)
	for _, s := range []string{"", "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3", "J\x00\x00\x00\n8.0.32\x00", "HTTP/1.1 200 OK\r\nServer: nginx/1.24.0", "\xff\x00"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		r := e.Recognize(model.Scan{IP: "127.0.0.1", Port: 1}, data)
		if r.Protocol == "" || r.Confidence < 0 || r.Confidence > 1 {
			t.Fatalf("%+v", r)
		}
	})
}
func FuzzDecode(f *testing.F) {
	f.Add(`J\x00\x00\x00\n8.0.32\x00`, uint8(1))
	f.Add("AA==", uint8(2))
	f.Add("", uint8(0))
	f.Fuzz(func(t *testing.T, banner string, mode uint8) {
		if len(banner) > 65536 {
			t.Skip()
		}
		data, err := Decode(model.Scan{Banner: banner, BannerEncoding: []string{"text", "escaped", "base64"}[mode%3]}, 1024)
		if err == nil && len(data) > 1024 {
			t.Fatal("limit bypass")
		}
	})
}
