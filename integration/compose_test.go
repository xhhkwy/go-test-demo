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
	"strconv"
	"strings"
	"testing"
	"time"
)

type container struct {
	ID     string
	Image  string
	Config struct {
		User        string
		Healthcheck struct{ Test []string }
	}
	State struct {
		Status    string
		ExitCode  int
		StartedAt time.Time
		Health    struct {
			Status string
			Log    []struct {
				End      time.Time
				ExitCode int
			}
		}
	}
	HostConfig struct {
		ReadonlyRootfs bool
		CapDrop        []string
		SecurityOpt    []string
		Memory         int64
		NanoCpus       int64
		PidsLimit      int64
		PortBindings   map[string]any
	}
	Mounts          []struct{ RW bool }
	NetworkSettings struct{ Networks map[string]any }
}

func TestCompose(t *testing.T) {
	if os.Getenv("FP_DOCKER_TEST") != "1" {
		t.Skip("set FP_DOCKER_TEST=1 after building Compose images")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := os.ReadFile(filepath.Join(root, "rules", "fingerprints.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	rulesPath := filepath.Join(dir, "fingerprints.json")
	if err := os.WriteFile(rulesPath, rules, 0644); err != nil {
		t.Fatal(err)
	}
	project := "fp-accept-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	env := append(os.Environ(), "RULES_DIR="+dir, "CLIENT_FORMAT=json", "INPUT_FILE="+filepath.Join(root, "testdata", "scans.json"))
	run := func(timeout time.Duration, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = root
		cmd.Env = env
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			return out.Bytes(), fmt.Errorf("docker %v: %w: %s", args, err, stderr.String())
		}
		if len(args) > 0 && args[0] == "logs" {
			return append(out.Bytes(), stderr.Bytes()...), nil
		}
		return out.Bytes(), nil
	}
	composeFiles := []string{"compose.yaml"}
	compose := func(timeout time.Duration, args ...string) ([]byte, error) {
		base := []string{"compose", "-p", project}
		for _, file := range composeFiles {
			base = append(base, "-f", file)
		}
		return run(timeout, append(base, args...)...)
	}
	must := func(out []byte, err error) []byte {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	defer func() {
		if out, err := compose(20*time.Second, "down", "--remove-orphans"); err != nil {
			t.Errorf("cleanup: %v %s", err, out)
		}
	}()
	id := func(service string) string {
		return strings.TrimSpace(string(must(compose(5*time.Second, "ps", "-aq", service))))
	}
	inspect := func(cid string) container {
		var list []container
		if err := json.Unmarshal(must(run(5*time.Second, "inspect", cid)), &list); err != nil || len(list) != 1 {
			t.Fatalf("inspect: %v", err)
		}
		return list[0]
	}
	waitHealthy := func(cid string) {
		t.Helper()
		for until := time.Now().Add(20 * time.Second); time.Now().Before(until); {
			if inspect(cid).State.Health.Status == "healthy" {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("server did not become healthy")
	}
	must(compose(25*time.Second, "up", "-d", "--no-build", "--pull", "never"))
	serverID, clientID := id("server"), id("client")
	if serverID == "" || clientID == "" {
		t.Fatal("missing containers")
	}
	waitHealthy(serverID)
	code := strings.TrimSpace(string(must(run(10*time.Second, "wait", clientID))))
	if code != "0" {
		t.Fatalf("client exit %s: %s", code, must(run(5*time.Second, "logs", clientID)))
	}
	var actual, want any
	if err := json.Unmarshal(must(run(5*time.Second, "logs", clientID)), &actual); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join(root, "testdata", "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(actual)
	w, _ := json.Marshal(want)
	if !bytes.Equal(a, w) {
		t.Fatalf("container result mismatch: %s", a)
	}
	serverState, clientState := inspect(serverID), inspect(clientID)
	healthBeforeClient := false
	for _, probe := range serverState.State.Health.Log {
		if probe.ExitCode == 0 && !probe.End.After(clientState.State.StartedAt) {
			healthBeforeClient = true
		}
	}
	if !healthBeforeClient {
		t.Fatal("no successful health probe before client start")
	}
	for _, tc := range []struct {
		name   string
		state  container
		memory int64
	}{
		{"server", serverState, 512 << 20}, {"client", clientState, 128 << 20},
	} {
		s := tc.state
		if s.Config.User != "65532:65532" || !s.HostConfig.ReadonlyRootfs || len(s.HostConfig.CapDrop) != 1 || s.HostConfig.CapDrop[0] != "ALL" || !strings.Contains(strings.Join(s.HostConfig.SecurityOpt, ","), "no-new-privileges") {
			t.Fatalf("%s privilege settings: %+v", tc.name, s.HostConfig)
		}
		if s.HostConfig.Memory != tc.memory || s.HostConfig.NanoCpus != 1000000000 || s.HostConfig.PidsLimit != 64 || len(s.HostConfig.PortBindings) != 0 || len(s.NetworkSettings.Networks) != 1 {
			t.Fatalf("%s limits/network: %+v", tc.name, s.HostConfig)
		}
		for _, mount := range s.Mounts {
			if mount.RW {
				t.Fatal("writable mount")
			}
		}
	}
	for network := range serverState.NetworkSettings.Networks {
		var networks []struct{ Internal bool }
		if err := json.Unmarshal(must(run(5*time.Second, "network", "inspect", network)), &networks); err != nil || len(networks) != 1 || !networks[0].Internal {
			t.Fatalf("network not internal: %v", err)
		}
	}
	t.Log("20/20 results; service DNS; health before client; no published ports; non-root/readonly/capabilities/resources: PASS")
	// The optional debug override must actually work through loopback only.
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	debugPort := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()
	env = append(env, "DEBUG_PORT="+strconv.Itoa(debugPort))
	composeFiles = append(composeFiles, "compose.debug.yaml")
	must(compose(25*time.Second, "up", "-d", "--no-build", "--pull", "never", "server"))
	serverID = id("server")
	waitHealthy(serverID)
	debugClient := &http.Client{Timeout: 2 * time.Second}
	resp, err := debugClient.Get("http://127.0.0.1:" + strconv.Itoa(debugPort) + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("debug health status %d", resp.StatusCode)
	}
	bindings := inspect(serverID).HostConfig.PortBindings["8080/tcp"].([]any)
	if len(bindings) != 1 || bindings[0].(map[string]any)["HostIp"] != "127.0.0.1" {
		t.Fatalf("debug bindings: %+v", bindings)
	}
	t.Log("debug override: real loopback HTTP access, binds 127.0.0.1 only: PASS")
	updated := bytes.Replace(rules, []byte(`"product":"nginx"`), []byte(`"product":"nginx-custom"`), 1)
	if bytes.Equal(updated, rules) {
		t.Fatal("rule edit did not apply")
	}
	if err := os.WriteFile(rulesPath, updated, 0644); err != nil {
		t.Fatal(err)
	}
	must(compose(15*time.Second, "restart", "server"))
	waitHealthy(serverID)
	output := must(compose(15*time.Second, "run", "--rm", "--no-deps", "client"))
	var rows []map[string]any
	if err := json.Unmarshal(output, &rows); err != nil || len(rows) != 20 || rows[1]["product"] != "nginx-custom" {
		t.Fatalf("rule update: %v %s", err, output)
	}
	if inspect(serverID).Image != serverState.Image {
		t.Fatal("image changed")
	}
	t.Log("external rule change after restart with same image: PASS")
	must(compose(15*time.Second, "stop", "server"))
	if inspect(serverID).State.ExitCode != 0 {
		t.Fatal("server failed graceful SIGTERM")
	}
	t.Log("SIGTERM graceful shutdown: PASS")
	must(compose(15*time.Second, "down"))
	composeFiles = []string{"compose.yaml"}
	if err := os.WriteFile(rulesPath, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, startErr := compose(15*time.Second, "up", "-d", "--no-build", "--pull", "never")
	if startErr == nil {
		t.Fatal("invalid rules started the stack")
	}
	if cid := id("client"); cid != "" && !inspect(cid).State.StartedAt.IsZero() {
		t.Fatal("client started without healthy server")
	}
	sid := id("server")
	if sid == "" {
		t.Fatal("missing failed server")
	}
	if !bytes.Contains(must(run(5*time.Second, "logs", sid)), []byte("decode rules")) {
		t.Fatal("missing rule failure")
	}
	t.Log("invalid rules reject startup and prevent client execution: PASS")
}
