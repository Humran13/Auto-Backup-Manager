// The GUI is a thin HTTP layer over the exact same internal/job,
// internal/config, internal/provider, internal/backend, internal/doctor,
// and internal/scheduler calls every CLI command already uses (see app in
// main.go) -- there is no separate "GUI business logic" to keep in sync
// with the CLI, only handlers that parse a request and call the same
// functions. Binds to 127.0.0.1 only; never exposed on a public/LAN
// interface by this project.
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

// guiServer holds everything the HTTP handlers need: the shared app (same
// as the CLI's), an in-memory CSRF token (regenerated per server process,
// never persisted), and in-memory run-state for async backup/restore/probe
// operations so the UI can show real progress without inventing fake
// percentages restic doesn't report.
type guiServer struct {
	app  *app
	csrf string

	mu   sync.Mutex
	runs map[string]*runState // keyed by an opaque run ID
}

// runState tracks one in-flight (or finished) async operation (backup now,
// restore, storage probe). Stage reflects real events as they actually
// happen (via a slog.Handler attached just for that run), not a simulated
// percentage.
type runState struct {
	Kind      string    `json:"kind"` // "backup" | "restore" | "probe"
	Target    string    `json:"target"`
	Stage     string    `json:"stage"`
	StartedAt time.Time `json:"startedAt"`
	Done      bool      `json:"done"`
	Error     string    `json:"error,omitempty"`
	Result    any       `json:"result,omitempty"`
}

func newGUIServer(a *app) *guiServer {
	return &guiServer{app: a, csrf: newCSRFToken(), runs: map[string]*runState{}}
}

func newCSRFToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failing is effectively unrecoverable; a predictable
		// fallback would be a real CSRF weakness, so fail loudly instead.
		panic("gui: failed to generate CSRF token: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func (g *guiServer) newRun(kind, target string) (string, *runState) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := newCSRFToken()
	rs := &runState{Kind: kind, Target: target, Stage: "starting", StartedAt: time.Now()}
	g.runs[id] = rs
	return id, rs
}

func (g *guiServer) getRun(id string) (*runState, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	rs, ok := g.runs[id]
	return rs, ok
}

// stageHandler is a slog.Handler that turns real log events emitted by
// internal/job.Run (repository initialized, backup succeeded, backup to
// destination failed, VSS warning, ...) into a human-readable current stage
// for the GUI to poll -- a direct reflection of what's actually happening,
// not a fabricated progress bar.
type stageHandler struct{ rs *runState }

func (h *stageHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *stageHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *stageHandler) WithGroup(string) slog.Handler            { return h }
func (h *stageHandler) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	switch {
	case strings.Contains(msg, "repository initialized"):
		h.rs.Stage = "Initializing repository"
	case strings.Contains(msg, "VSS not used"):
		h.rs.Stage = "Scanning files (no VSS)"
	case strings.Contains(msg, "backup to destination failed"):
		h.rs.Stage = "Destination failed"
	case strings.Contains(msg, "backup degraded"):
		h.rs.Stage = "Secondary destination degraded"
	case strings.Contains(msg, "backup succeeded"):
		h.rs.Stage = "Completed"
	}
	return nil
}

func newStageLogger(rs *runState) *slog.Logger { return slog.New(&stageHandler{rs: rs}) }

// writeJSON and friends keep every handler's response shape consistent and
// make sure a Go error never gets leaked verbatim where it might contain a
// path or other detail worth normalizing -- callers still pass their own
// message, this just centralizes the status code + content type.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// csrfMiddleware requires a matching X-CSRF-Token header on every mutating
// request. The token is generated fresh per server process and handed to
// the page via GET /api/csrf; it is never written into HTML/JS source or
// logged. GET/HEAD requests (read-only) are exempt.
func (g *guiServer) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/csrf" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-CSRF-Token") != g.csrf {
			writeErr(w, http.StatusForbidden, "missing or invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// localOnlyMiddleware is a defense-in-depth second check behind binding to
// 127.0.0.1: rejects any request whose remote address isn't loopback, in
// case the server is ever started with a non-default bind address by a
// future change.
func localOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeErr(w, http.StatusForbidden, "the Auto-Backup-Manager GUI only accepts connections from this machine")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *guiServer) routes() http.Handler {
	mux := http.NewServeMux()

	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err) // embedded at build time; a failure here is a packaging bug, not a runtime condition
	}
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	mux.HandleFunc("GET /api/csrf", g.handleCSRF)
	mux.HandleFunc("GET /api/status", g.handleStatus)
	mux.HandleFunc("GET /api/providers", g.handleProviders)

	mux.HandleFunc("GET /api/storage", g.handleStorageList)
	mux.HandleFunc("POST /api/storage", g.handleStorageAdd)
	mux.HandleFunc("POST /api/storage/test", g.handleStorageTest)
	mux.HandleFunc("POST /api/storage/reconnect", g.handleStorageReconnect)
	mux.HandleFunc("DELETE /api/storage/{name}", g.handleStorageRemove)

	mux.HandleFunc("GET /api/jobs", g.handleJobsList)
	mux.HandleFunc("POST /api/jobs", g.handleJobUpsert)
	mux.HandleFunc("DELETE /api/jobs/{name}", g.handleJobRemove)
	mux.HandleFunc("POST /api/jobs/{name}/enable", g.handleJobEnable)
	mux.HandleFunc("POST /api/jobs/{name}/run", g.handleJobRun)

	mux.HandleFunc("GET /api/snapshots", g.handleSnapshots)
	mux.HandleFunc("POST /api/restore", g.handleRestore)

	mux.HandleFunc("GET /api/schedule", g.handleScheduleShow)
	mux.HandleFunc("POST /api/schedule", g.handleScheduleSet)

	mux.HandleFunc("GET /api/doctor", g.handleDoctor)

	mux.HandleFunc("GET /api/activity", g.handleActivity)

	mux.HandleFunc("GET /api/settings", g.handleSettingsGet)
	mux.HandleFunc("POST /api/settings", g.handleSettingsSet)

	mux.HandleFunc("POST /api/setup", g.handleSetup)

	mux.HandleFunc("GET /api/runs/{id}", g.handleRunStatus)

	return localOnlyMiddleware(g.csrfMiddleware(mux))
}

func (g *guiServer) handleCSRF(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"token": g.csrf})
}

func (g *guiServer) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	rs, ok := g.getRun(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such run")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

// runGUIServer binds to addr (127.0.0.1:<port> by default), serves until ctx
// is canceled (Ctrl+C), and reports a clear, specific error if the port is
// already in use rather than a generic bind failure.
func runGUIServer(ctx context.Context, a *app, addr string, open bool, log *slog.Logger) error {
	g := newGUIServer(a)
	srv := &http.Server{Addr: addr, Handler: g.routes()}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("port already in use: %s -- stop whatever is using it, or run 'abm gui --port <other-port>'", addr)
		}
		return fmt.Errorf("could not start the GUI server on %s: %w", addr, err)
	}

	url := "http://" + addr
	fmt.Println("Auto-Backup-Manager GUI running at", url)
	fmt.Println("(bound to localhost only; press Ctrl+C to stop)")
	if open {
		openBrowser(url)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func isAddrInUse(err error) bool {
	return strings.Contains(err.Error(), "address already in use") ||
		strings.Contains(err.Error(), "Only one usage of each socket address") // Windows wording
}
