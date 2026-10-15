package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestRealServerClient(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	paths := map[string]string{}
	for _, name := range []string{"server", "client"} {
		path := filepath.Join(bin, name+suffix)
		cmd := exec.Command("go", "build", "-o", path, "./cmd/"+name)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v %s", name, err, out)
		}
		paths[name] = path
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	server := exec.CommandContext(ctx, paths["server"], "--addr", addr)
	server.Dir = root
	var serverLog bytes.Buffer
	server.Stderr = &serverLog
	if err := server.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); _ = server.Wait() }()
	url := "http://" + addr
	healthy := false
	for until := time.Now().Add(10 * time.Second); time.Now().Before(until); {
		probe := exec.Command(paths["server"], "healthcheck", "--url", url+"/health", "--timeout", "200ms")
		if err := probe.Run(); err == nil {
			healthy = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !healthy {
		t.Fatal("server did not become healthy")
	}
	cmd := exec.Command(paths["client"], "--server", url, "--format", "json")
	cmd.Dir = root
	var output, stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("client: %v %s", err, &stderr)
	}
	var got, want any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "testdata", "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output mismatch: %s", output.String())
	}
	t.Log("real server + independent client: 20/20 results match expected")
	cmd = exec.Command(paths["client"], "--server", url, "--format", "table")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(out, []byte("OpenSSH")) || !bytes.Contains(out, []byte("CONFIDENCE")) {
		t.Fatalf("table: %v %s", err, out)
	}
	// Each rejected request is followed by a healthy and a successful request.
	httpClient := &http.Client{Timeout: time.Second}
	for _, body := range []string{"[", "null", `[{"ip":"bad","port":22,"banner":""}]`, `[{"ip":"::1","port":0,"banner":""}]`, `[{"ip":"::1","port":22,"banner":"?","banner_encoding":"base64"}]`} {
		resp, err := httpClient.Post(url+"/fingerprint", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("invalid input status %d", resp.StatusCode)
		}
		resp, err = httpClient.Get(url + "/health")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal("unhealthy after failure")
		}
		resp, err = httpClient.Post(url+"/fingerprint", "application/json", bytes.NewBufferString("[]"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal("normal request failed after error")
		}
	}
	for name, args := range map[string][]string{
		"missing input": {"--input", filepath.Join(bin, "missing.json")},
		"bad format":    {"--format", "csv"},
		"unavailable":   {"--server", "http://127.0.0.1:1", "--timeout", "100ms"},
	} {
		cmd := exec.Command(paths["client"], args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err == nil || len(out) == 0 {
			t.Fatalf("%s must fail usefully: %v %s", name, err, out)
		}
	}
	badFile := filepath.Join(bin, "invalid.json")
	if err := os.WriteFile(badFile, []byte("["), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(paths["client"], "--input", badFile, "--server", url)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err == nil || len(out) == 0 {
		t.Fatalf("bad JSON file: %v %s", err, out)
	}
	badRules := filepath.Join(bin, "rules.json")
	for _, body := range []string{`{"version":1,"rules":[{"id":"bad","source":"banner","pattern":"[","protocol":"SSH","confidence":0.9}]}`, `{"version":1,"rules":[{"id":"bad","source":"banner","pattern":".","protocol":"SSH","confidence":2}]}`} {
		if err := os.WriteFile(badRules, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(paths["server"], "--rules", badRules)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err == nil || len(out) == 0 {
			t.Fatalf("invalid rules start: %v %s", err, out)
		}
	}
	t.Log(fmt.Sprintf("CLI errors, startup errors and HTTP failure recovery: passed on %s/%s", runtime.GOOS, runtime.GOARCH))
}
