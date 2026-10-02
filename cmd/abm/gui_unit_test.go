package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
)

func TestValidateOriginalSources_AllSupportedLayouts(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		sources []string
		wantErr bool
	}{
		{name: "single Linux source", goos: "linux", sources: []string{"/opt/app"}},
		{name: "multiple Linux sources", goos: "linux", sources: []string{"/opt/app", "/var/lib/app"}},
		{name: "single Windows source", goos: "windows", sources: []string{`C:\apps\one`}},
		{name: "multiple Windows sources same drive", goos: "windows", sources: []string{`C:\apps\one`, `c:\data\two`}},
		{name: "multiple Windows sources different drives", goos: "windows", sources: []string{`C:\apps`, `D:\data`}},
		{name: "empty manifest", goos: "linux", wantErr: true},
		{name: "relative Unix", goos: "linux", sources: []string{"var/data"}, wantErr: true},
		{name: "Unix traversal", goos: "linux", sources: []string{"/opt/../etc"}, wantErr: true},
		{name: "relative Windows", goos: "windows", sources: []string{`data\app`}, wantErr: true},
		{name: "Windows traversal", goos: "windows", sources: []string{`C:\apps\..\Windows`}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOriginalSources(test.goos, test.sources)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateOriginalSources() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateSafeRestoreTarget_RemainsSeparate(t *testing.T) {
	if err := validateSafeRestoreTarget("linux", "/restore/job", []string{"/srv/app", "/var/data"}); err != nil {
		t.Fatalf("separate safe target rejected: %v", err)
	}
	for _, target := range []string{"/srv/app", "/srv/app/recovered", "/srv", "/tmp/../srv/app"} {
		if err := validateSafeRestoreTarget("linux", target, []string{"/srv/app"}); err == nil {
			t.Errorf("unsafe target %q was accepted", target)
		}
	}
	if err := validateSafeRestoreTarget("windows", `D:\Recovered`, []string{`C:\apps`, `D:\data`}); err != nil {
		t.Fatalf("separate Windows target rejected: %v", err)
	}
	if err := validateSafeRestoreTarget("windows", `C:\apps\..\Windows`, []string{`C:\apps`}); err == nil {
		t.Fatal("Windows traversal target was accepted")
	}
}

func TestRestoreOriginalSources_WritesIntendedLocation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "original", "source")
	if err := os.MkdirAll(destination, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "changed.txt"), []byte("newer"), 0o600); err != nil {
		t.Fatal(err)
	}
	staging := t.TempDir()
	var stagedSource string
	if runtime.GOOS == "windows" {
		drive := strings.TrimSuffix(filepath.VolumeName(destination), ":")
		tail := strings.TrimPrefix(destination[len(filepath.VolumeName(destination)):], string(os.PathSeparator))
		stagedSource = filepath.Join(staging, drive, tail)
	} else {
		stagedSource = filepath.Join(staging, strings.TrimPrefix(destination, string(os.PathSeparator)))
	}
	if err := os.MkdirAll(stagedSource, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagedSource, "changed.txt"), []byte("from recovery point"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagedSource, "restored.txt"), []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restoreOriginalSources(runtime.GOOS, staging, []string{destination}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"changed.txt": "from recovery point", "restored.txt": "restored"} {
		got, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(got) != want {
			t.Fatalf("original path %s = %q, %v; want %q", name, got, err, want)
		}
	}
	missingSource := filepath.Join(filepath.Dir(destination), "not-selected")
	if err := restoreOriginalSources(runtime.GOOS, staging, []string{destination, missingSource}); err != nil {
		t.Fatalf("an unselected source absent from staging must be skipped: %v", err)
	}
}

func TestNewCSRFToken_Unique(t *testing.T) {
	a := newCSRFToken()
	b := newCSRFToken()
	if a == b {
		t.Fatal("expected two distinct CSRF tokens")
	}
	if len(a) < 20 {
		t.Fatalf("CSRF token looks too short/weak: %q", a)
	}
}

func TestCSRFMiddleware_RejectsMutationWithoutToken(t *testing.T) {
	g := &guiServer{csrf: "secret-token", runs: map[string]*runState{}}
	handler := g.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a POST with no CSRF token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_RejectsWrongToken(t *testing.T) {
	g := &guiServer{csrf: "secret-token", runs: map[string]*runState{}}
	handler := g.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodDelete, "/api/storage/x", nil)
	req.Header.Set("X-CSRF-Token", "wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a wrong CSRF token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_AcceptsCorrectToken(t *testing.T) {
	g := &guiServer{csrf: "secret-token", runs: map[string]*runState{}}
	handler := g.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", nil)
	req.Header.Set("X-CSRF-Token", "secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a correct CSRF token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_ExemptsGET(t *testing.T) {
	g := &guiServer{csrf: "secret-token", runs: map[string]*runState{}}
	handler := g.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET to be exempt from CSRF checks, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_ExemptsTokenEndpointItself(t *testing.T) {
	// /api/csrf must be reachable without already having a token --
	// otherwise there would be no way to ever obtain the first one.
	g := &guiServer{csrf: "secret-token", runs: map[string]*runState{}}
	handler := g.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/csrf", nil) // hypothetical non-GET; still must not require the token it hands out
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /api/csrf to be exempt from its own check, got %d", rec.Code)
	}
}

func TestLocalOnlyMiddleware_RejectsNonLoopback(t *testing.T) {
	handler := localOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "203.0.113.5:54321" // a real-looking public IP, not loopback
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-loopback remote address, got %d", rec.Code)
	}
}

func TestLocalOnlyMiddleware_AcceptsLoopback(t *testing.T) {
	handler := localOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, addr := range []string{"127.0.0.1:54321", "[::1]:54321"} {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for loopback address %q, got %d", addr, rec.Code)
		}
	}
}

func TestIsAddrInUse(t *testing.T) {
	cases := map[string]bool{
		"listen tcp 127.0.0.1:8765: bind: address already in use":                      true,
		"listen tcp 127.0.0.1:8765: bind: Only one usage of each socket address (...)": true,
		"listen tcp 127.0.0.1:8765: bind: permission denied":                           false,
	}
	for msg, want := range cases {
		if got := isAddrInUse(errString(msg)); got != want {
			t.Errorf("isAddrInUse(%q) = %v, want %v", msg, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestToProviderView_RoundTripsRegistryFields(t *testing.T) {
	p, ok := provider.Get("local")
	if !ok {
		t.Fatal("expected 'local' provider to exist")
	}
	v := toProviderView(p)
	if v.ID != "local" || v.DisplayName != p.DisplayName || v.Backend != string(p.Backend) {
		t.Fatalf("provider view lost fields: %+v", v)
	}
	if len(v.RequiredFields) != len(p.RequiredFields) {
		t.Fatalf("expected %d required fields, got %d", len(p.RequiredFields), len(v.RequiredFields))
	}
}

func TestToProviderView_NeverLeaksNothingSecretFlagsOnPublicFields(t *testing.T) {
	// Every S3-style provider has a secret_key field; confirm the view
	// preserves Secret=true so the frontend knows to mask it -- losing this
	// flag during JSON conversion would be a real secret-exposure risk.
	p, ok := provider.Get("generic-s3")
	if !ok {
		t.Fatal("expected 'generic-s3' provider to exist")
	}
	v := toProviderView(p)
	found := false
	for _, f := range v.RequiredFields {
		if f.Key == "secret_key" {
			found = true
			if !f.Secret {
				t.Fatal("secret_key field lost its Secret=true flag in the GUI view")
			}
		}
	}
	if !found {
		t.Fatal("expected generic-s3 to have a secret_key required field")
	}
}

func TestToJobView_ConvertsDefaults(t *testing.T) {
	j := config.Job{
		Sources: []string{"/data"}, Destinations: []string{"d1"},
		RepositoryPath: "org/dev/job1", Enabled: true,
		Retention: &config.Retention{KeepWithinHourly: "240h"},
	}
	v := toJobView("job1", j)
	if v.Name != "job1" || v.KeepWithin != "240h" || v.Policy != string(config.PolicyPrimaryRequired) {
		t.Fatalf("unexpected job view: %+v", v)
	}
}

func TestStageHandler_MapsLogMessagesToStages(t *testing.T) {
	rs := &runState{}
	logger := newStageLogger(rs)
	logger.Info("repository initialized", "job", "j1")
	if rs.Stage != "Initializing repository" {
		t.Fatalf("expected stage to update from log message, got %q", rs.Stage)
	}
	logger.Info("backup succeeded", "job", "j1")
	if rs.Stage != "Completed" {
		t.Fatalf("expected stage 'Completed', got %q", rs.Stage)
	}
}

func TestStageHandler_IgnoresUnrelatedMessages(t *testing.T) {
	rs := &runState{Stage: "untouched"}
	logger := newStageLogger(rs)
	logger.Info("some unrelated message")
	if rs.Stage != "untouched" {
		t.Fatalf("expected stage to remain unchanged for an unrelated message, got %q", rs.Stage)
	}
}

func TestGUIServer_RunLifecycle(t *testing.T) {
	g := &guiServer{csrf: "x", runs: map[string]*runState{}}
	id, rs := g.newRun("backup", "job1")
	if id == "" {
		t.Fatal("expected a non-empty run ID")
	}
	got, ok := g.getRun(id)
	if !ok || got != rs {
		t.Fatal("expected getRun to return the same runState newRun created")
	}
	if _, ok := g.getRun("does-not-exist"); ok {
		t.Fatal("expected getRun to report false for an unknown ID")
	}
}

// TestRunGUIServer_PortAlreadyInUseIsReportedClearly holds a port open with
// a plain listener, then confirms runGUIServer reports the specific,
// actionable "port already in use" error rather than a generic bind
// failure -- this doesn't touch internal/paths (no config/status
// involved), so it's safe to run as a normal unit test.
func TestRunGUIServer_PortAlreadyInUseIsReportedClearly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err = runGUIServer(ctx, &app{log: logger}, addr, false, logger)
	if err == nil {
		t.Fatal("expected an error when the port is already in use")
	}
	wantSubstr := "port already in use"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("expected error to mention %q, got: %v", wantSubstr, err)
	}
}

// TestRunGUIServer_GracefulShutdown confirms canceling the context stops
// the server cleanly (no error) rather than hanging or panicking.
func TestRunGUIServer_GracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // free it up for runGUIServer itself to bind

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan error, 1)
	go func() { done <- runGUIServer(ctx, &app{log: logger}, addr, false, logger) }()

	// Give it a moment to actually start listening before shutting down.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.Dial("tcp", addr); err == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s of context cancellation")
	}
}
