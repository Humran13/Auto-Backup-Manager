// This file automates the project's GUI acceptance scenario end to end
// against a real `abm gui` subprocess, driving it exclusively through its
// HTTP API -- the same API the browser frontend calls -- never by shelling
// out to abm CLI commands for the actual backup/restore operations. The
// subprocess is necessary (rather than calling Go functions directly, as
// test/integration's other tests do) because internal/paths resolves
// ABM_HOME into package-level variables once, at process start; a real
// child process with its own isolated ABM_HOME is the only way to exercise
// the GUI's actual on-disk behavior without reaching into cmd/abm's
// internals from a different package.
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
	"runtime"
	"sync"
	"testing"
	"time"
)

var (
	guiBinOnce sync.Once
	guiBinPath string
	guiBinErr  error
)

// buildGUIBinary compiles abm once per test run (cached across subtests),
// since `go build` takes a few seconds and every GUI test needs the same
// binary.
func buildGUIBinary(t *testing.T) string {
	t.Helper()
	guiBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "abm-gui-bin-*")
		if err != nil {
			guiBinErr = err
			return
		}
		bin := filepath.Join(dir, "abm")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/abm")
		cmd.Dir = repoRootForGUITest(t)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			guiBinErr = fmt.Errorf("building abm: %w: %s", err, stderr.String())
			return
		}
		guiBinPath = bin
	})
	if guiBinErr != nil {
		t.Fatalf("could not build abm for GUI testing: %v", guiBinErr)
	}
	return guiBinPath
}

func repoRootForGUITest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// test/integration -> repo root
	return filepath.Join(dir, "..", "..")
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// guiClient wraps the HTTP calls a browser would make: fetch a CSRF token
// once, then include it on every mutating request, exactly like
// cmd/abm/web/app.js does.
type guiClient struct {
	t       *testing.T
	baseURL string
	http    *http.Client
	csrf    string
}

func startGUIForTest(t *testing.T, home string) *guiClient {
	t.Helper()
	bin := buildGUIBinary(t)
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	baseURL := "http://" + addr

	cmd := exec.CommandContext(context.Background(), bin, "gui", "--port", fmt.Sprint(port), "--no-open")
	cmd.Env = append(os.Environ(), "ABM_HOME="+home)
	var stderr, stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting abm gui: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	client := &http.Client{Timeout: 30 * time.Second}
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/api/csrf")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil && time.Now().After(deadline) {
		t.Fatalf("abm gui never became ready: %v\nstdout: %s\nstderr: %s", lastErr, stdout.String(), stderr.String())
	}

	gc := &guiClient{t: t, baseURL: baseURL, http: client}
	gc.csrf = gc.getCSRF()
	return gc
}

func (gc *guiClient) getCSRF() string {
	resp, err := gc.http.Get(gc.baseURL + "/api/csrf")
	if err != nil {
		gc.t.Fatalf("GET /api/csrf: %v", err)
	}
	defer resp.Body.Close()
	var v struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		gc.t.Fatalf("decoding csrf response: %v", err)
	}
	return v.Token
}

func (gc *guiClient) do(method, path string, body any) (*http.Response, map[string]any) {
	gc.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			gc.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, gc.baseURL+path, reader)
	if err != nil {
		gc.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", gc.csrf)
	}
	resp, err := gc.http.Do(req)
	if err != nil {
		gc.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp, parsed
}

func (gc *guiClient) doList(method, path string, body any) []map[string]any {
	gc.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, gc.baseURL+path, reader)
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", gc.csrf)
	}
	resp, err := gc.http.Do(req)
	if err != nil {
		gc.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var parsed []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return parsed
}

func (gc *guiClient) mustOK(resp *http.Response, parsed map[string]any, context string) {
	gc.t.Helper()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		gc.t.Fatalf("%s: HTTP %d: %v", context, resp.StatusCode, parsed)
	}
}

func (gc *guiClient) pollRun(runID string) map[string]any {
	gc.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, parsed := gc.do(http.MethodGet, "/api/runs/"+runID, nil)
		if resp.StatusCode == http.StatusOK {
			if done, _ := parsed["done"].(bool); done {
				return parsed
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	gc.t.Fatalf("run %s did not complete within the deadline", runID)
	return nil
}

// TestGUI_FullAcceptanceScenario automates exactly the project's required
// acceptance scenario: fresh config -> setup -> local storage -> source
// folder -> backup -> modify/add/delete -> second backup -> list snapshots
// -> delete original data -> restore first snapshot (verify byte-for-byte)
// -> restore latest (verify byte-for-byte) -> doctor. Every step goes
// through the real GUI HTTP API against a real running `abm gui` process.
func TestGUI_FullAcceptanceScenario(t *testing.T) {
	requireRestic(t)

	root := testRoot(t)
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "source")
	dest := filepath.Join(root, "dest")
	for _, d := range []string{home, source, dest} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	gc := startGUIForTest(t, home)

	// 1-3: fresh config via setup wizard's own endpoint.
	resp, parsed := gc.do(http.MethodPost, "/api/setup", map[string]string{"deviceName": "gui-test", "organization": "acme"})
	gc.mustOK(resp, parsed, "setup")

	// 4-5: add local storage + source folder via a job.
	resp, parsed = gc.do(http.MethodPost, "/api/storage", map[string]any{
		"name": "local", "provider": "local", "options": map[string]string{"path": dest},
	})
	gc.mustOK(resp, parsed, "add storage")

	resp, parsed = gc.do(http.MethodPost, "/api/jobs", map[string]any{
		"name": "j1", "sources": []string{source}, "destinations": []string{"local"},
	})
	gc.mustOK(resp, parsed, "add job")

	// 6: create files.
	writeFile(t, filepath.Join(source, "file1.txt"), "file1 original")
	writeFile(t, filepath.Join(source, "file2.txt"), "file2 original")

	// 7: run backup (snapshot A).
	resp, parsed = gc.do(http.MethodPost, "/api/jobs/j1/run", map[string]any{})
	gc.mustOK(resp, parsed, "run backup A")
	runA := gc.pollRun(parsed["runId"].(string))
	if errMsg, _ := runA["error"].(string); errMsg != "" {
		t.Fatalf("backup A failed: %s", errMsg)
	}
	resultA, _ := runA["result"].(map[string]any)
	destsA, _ := resultA["destinations"].([]any)
	if len(destsA) == 0 {
		t.Fatal("backup A produced no destination result")
	}
	snapshotA, _ := destsA[0].(map[string]any)["last_snapshot_id"].(string)
	if snapshotA == "" {
		t.Fatal("backup A produced no snapshot id")
	}

	time.Sleep(1100 * time.Millisecond)

	// 8: modify/add/delete.
	writeFile(t, filepath.Join(source, "file1.txt"), "file1 MODIFIED")
	writeFile(t, filepath.Join(source, "file3.txt"), "file3 new")
	os.Remove(filepath.Join(source, "file2.txt"))

	// 9: run second backup (snapshot B / latest).
	resp, parsed = gc.do(http.MethodPost, "/api/jobs/j1/run", map[string]any{})
	gc.mustOK(resp, parsed, "run backup B")
	runB := gc.pollRun(parsed["runId"].(string))
	if errMsg, _ := runB["error"].(string); errMsg != "" {
		t.Fatalf("backup B failed: %s", errMsg)
	}

	// 10: list snapshots.
	snaps := gc.doList(http.MethodGet, "/api/snapshots?job=j1", nil)
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d: %v", len(snaps), snaps)
	}

	// 11: delete original test data (simulating total loss).
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}

	// 12-13: restore snapshot A, verify byte-for-byte.
	resp, parsed = gc.do(http.MethodPost, "/api/restore", map[string]any{"job": "j1", "snapshotId": snapshotA})
	gc.mustOK(resp, parsed, "restore A")
	runRestoreA := gc.pollRun(parsed["runId"].(string))
	if errMsg, _ := runRestoreA["error"].(string); errMsg != "" {
		t.Fatalf("restore A failed: %s", errMsg)
	}
	targetA, _ := parsed["target"].(string)
	restoredSourceA := restoredPathFor(targetA, source)
	assertFileContent(t, restoredSourceA, "file1.txt", "file1 original")
	assertFileContent(t, restoredSourceA, "file2.txt", "file2 original")
	if _, err := os.Stat(filepath.Join(restoredSourceA, "file3.txt")); !os.IsNotExist(err) {
		t.Fatal("file3.txt should not exist in snapshot A's restore")
	}

	// 14-15: restore latest, verify byte-for-byte.
	resp, parsed = gc.do(http.MethodPost, "/api/restore", map[string]any{"job": "j1", "snapshotId": "latest"})
	gc.mustOK(resp, parsed, "restore latest")
	runRestoreLatest := gc.pollRun(parsed["runId"].(string))
	if errMsg, _ := runRestoreLatest["error"].(string); errMsg != "" {
		t.Fatalf("restore latest failed: %s", errMsg)
	}
	targetLatest, _ := parsed["target"].(string)
	restoredSourceLatest := restoredPathFor(targetLatest, source)
	assertFileContent(t, restoredSourceLatest, "file1.txt", "file1 MODIFIED")
	assertFileContent(t, restoredSourceLatest, "file3.txt", "file3 new")
	if _, err := os.Stat(filepath.Join(restoredSourceLatest, "file2.txt")); !os.IsNotExist(err) {
		t.Fatal("file2.txt should not exist in the latest restore")
	}

	// 16: doctor.
	checks := gc.doList(http.MethodGet, "/api/doctor", nil)
	if len(checks) == 0 {
		t.Fatal("expected at least one doctor check")
	}
	for _, c := range checks {
		name, _ := c["name"].(string)
		if name == "restic" || name == "config" {
			if status, _ := c["status"].(string); status != "healthy" {
				t.Errorf("expected doctor check %q to be healthy, got %v", name, c)
			}
		}
	}
}

// TestGUI_SecurityAndErrorHandling covers the specific hardening
// requirements: a secret value is never echoed back in any API response,
// invalid input is rejected with a clear error rather than a crash, and
// operations against a nonexistent job/storage/run return a normal 404
// instead of a panic or a raw Go error.
func TestGUI_SecurityAndErrorHandling(t *testing.T) {
	requireRestic(t)

	root := testRoot(t)
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	gc := startGUIForTest(t, home)

	resp, parsed := gc.do(http.MethodPost, "/api/setup", map[string]string{"deviceName": "sec-test"})
	gc.mustOK(resp, parsed, "setup")

	t.Run("secret values never appear in storage list response", func(t *testing.T) {
		secretValue := "super-secret-access-key-should-never-leak-AKIAEXAMPLE"
		resp, parsed := gc.do(http.MethodPost, "/api/storage", map[string]any{
			"name": "s3fake", "provider": "generic-s3",
			"options":   map[string]string{"bucket": "mybucket", "endpoint": "example.com"},
			"secrets":   map[string]string{"access_key": "fake", "secret_key": secretValue},
			"skipProbe": true,
		})
		gc.mustOK(resp, parsed, "add storage with skipProbe")

		httpResp, err := gc.http.Get(gc.baseURL + "/api/storage")
		if err != nil {
			t.Fatal(err)
		}
		body := new(bytes.Buffer)
		body.ReadFrom(httpResp.Body)
		httpResp.Body.Close()
		if bytesContains(body.Bytes(), secretValue) {
			t.Fatalf("secret value leaked in /api/storage response: %s", body.String())
		}
	})

	t.Run("creating a job with no sources is rejected with a clear error, not a crash", func(t *testing.T) {
		resp, parsed := gc.do(http.MethodPost, "/api/jobs", map[string]any{"name": "bad", "sources": []string{}, "destinations": []string{"s3fake"}})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for a job with no sources, got %d: %v", resp.StatusCode, parsed)
		}
		if parsed["error"] == nil {
			t.Fatal("expected an error message in the response body")
		}
	})

	t.Run("creating a storage with unknown provider is rejected", func(t *testing.T) {
		resp, parsed := gc.do(http.MethodPost, "/api/storage", map[string]any{"name": "x", "provider": "not-a-real-provider"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for an unknown provider, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("running a nonexistent job returns 404, not a crash", func(t *testing.T) {
		resp, parsed := gc.do(http.MethodPost, "/api/jobs/does-not-exist/run", map[string]any{})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for a nonexistent job, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("restoring a nonexistent job returns 404, not a crash", func(t *testing.T) {
		resp, parsed := gc.do(http.MethodPost, "/api/restore", map[string]any{"job": "does-not-exist", "snapshotId": "latest"})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for restoring a nonexistent job, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("polling a nonexistent run ID returns 404, not a crash", func(t *testing.T) {
		resp, parsed := gc.do(http.MethodGet, "/api/runs/totally-made-up-id", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for an unknown run ID, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("malformed JSON body is rejected with 400, not a crash", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, gc.baseURL+"/api/jobs", bytes.NewReader([]byte("{not valid json")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", gc.csrf)
		resp, err := gc.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for malformed JSON, got %d", resp.StatusCode)
		}
	})

	t.Run("mutating request without CSRF token is rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, gc.baseURL+"/api/storage/s3fake", nil)
		resp, err := gc.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for a DELETE with no CSRF token, got %d", resp.StatusCode)
		}
	})

	t.Run("a path-traversal-shaped storage name is rejected, not a filesystem operation", func(t *testing.T) {
		resp, _ := gc.do(http.MethodDelete, "/api/storage/..%2f..%2f..%2fetc%2fpasswd", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for a path-traversal-shaped storage name (it should just not match any configured storage), got %d", resp.StatusCode)
		}
	})
}

func bytesContains(haystack []byte, needle string) bool {
	return bytes.Contains(haystack, []byte(needle))
}
