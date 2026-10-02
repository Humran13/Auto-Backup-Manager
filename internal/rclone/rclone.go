// Package rclone wraps the rclone CLI, used only for providers that have no
// restic-native backend -- the cloud-drive family (Google Drive, OneDrive,
// Dropbox, Box, pCloud, MEGA, Jottacloud, iCloud Drive, Proton Drive) and the
// generic "use an existing rclone remote" escape hatch. restic itself drives
// rclone directly via its "rclone:" repository backend for actual data
// transfer; S3/SFTP/Azure/GCS/Swift all use restic's own native backends
// instead (see internal/backend) and never touch this package. This package
// handles the parts restic doesn't: listing/testing/reconnecting remotes and
// running the OAuth dance for providers that need it.
package rclone

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sync"
)

var authURLPattern = regexp.MustCompile(`https?://127\.0\.0\.1:[0-9]+/auth\?state=[A-Za-z0-9_-]+`)

type authURLWriter struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	onURL    func(string)
	reported string
}

func (w *authURLWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buffer.Write(p)
	if match := authURLPattern.FindString(w.buffer.String()); match != "" && match != w.reported {
		w.reported = match
		if w.onURL != nil {
			w.onURL(match)
		}
	}
	return len(p), nil
}

// Runner executes rclone against a specific config file, keeping every
// device's rclone.conf (which holds OAuth tokens) isolated and under OS
// permissions rather than sharing a user's default rclone config.
type Runner struct {
	BinaryPath string
	ConfigPath string
}

func (r *Runner) binary() string {
	if r.BinaryPath != "" {
		return r.BinaryPath
	}
	return "rclone"
}

func (r *Runner) run(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"--config", r.ConfigPath}, args...)
	cmd := exec.CommandContext(ctx, r.binary(), full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rclone %s: %w: %s", args[0], err, stderr.String())
	}
	return stdout.String(), nil
}

// ListRemotes returns the names of remotes already configured.
func (r *Runner) ListRemotes(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "listremotes")
	if err != nil {
		return nil, err
	}
	var remotes []string
	for _, line := range splitLines(out) {
		if line == "" {
			continue
		}
		remotes = append(remotes, line[:len(line)-1]) // strip trailing ':'
	}
	return remotes, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// Obscure runs rclone's own (reversible, not cryptographically strong)
// password obscuring, required by rclone.conf for any stored password field.
func (r *Runner) Obscure(ctx context.Context, plaintext string) (string, error) {
	cmd := exec.CommandContext(ctx, r.binary(), "obscure", plaintext)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rclone obscure: %w", err)
	}
	return trimNewline(stdout.String()), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// AuthorizeOAuth runs `rclone authorize <providerType> [clientID] [clientSecret]`,
// rclone's headless OAuth flow: it prints a URL for the administrator to open
// on any machine with a browser (not necessarily this one, which is what
// makes it usable on a headless VPS), then returns the resulting token JSON
// once the administrator completes the flow. The caller writes that token
// into the remote's config; it is never persisted by this package directly.
func (r *Runner) AuthorizeOAuth(ctx context.Context, providerType, clientID, clientSecret string) (tokenJSON string, err error) {
	return r.AuthorizeOAuthProgress(ctx, providerType, clientID, clientSecret, nil)
}

// AuthorizeOAuthProgress is AuthorizeOAuth plus a callback for the temporary
// local authorization URL. Desktop installs open it automatically; the GUI
// also displays it so the flow remains visible and testable.
func (r *Runner) AuthorizeOAuthProgress(ctx context.Context, providerType, clientID, clientSecret string, onURL func(string)) (tokenJSON string, err error) {
	args := []string{"authorize", providerType}
	if clientID != "" {
		args = append(args, clientID, clientSecret)
	}
	full := append([]string{"--config", r.ConfigPath}, args...)
	cmd := exec.CommandContext(ctx, r.binary(), full...)
	var stdout, stderr bytes.Buffer
	watcher := &authURLWriter{onURL: onURL}
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(&stderr, watcher)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rclone authorize %s: %w: %s", providerType, err, stderr.String())
	}
	return extractToken(stdout.String()), nil
}

// extractToken pulls the trailing JSON blob `rclone authorize` prints after
// its human-readable instructions.
func extractToken(out string) string {
	idx := bytes.LastIndexByte([]byte(out), '{')
	if idx < 0 {
		return ""
	}
	return trimNewline(out[idx:])
}

// CreateOAuthRemote registers a remote for an OAuth-based provider (Google
// Drive, OneDrive, Dropbox) using a token already obtained via AuthorizeOAuth
// or an interactive `rclone config` run.
func (r *Runner) CreateOAuthRemote(ctx context.Context, name, providerType, clientID, clientSecret, tokenJSON string) error {
	args := []string{"config", "create", name, providerType, "--non-interactive"}
	if clientID != "" {
		args = append(args, "client_id", clientID, "client_secret", clientSecret)
	}
	if tokenJSON != "" {
		args = append(args, "token", tokenJSON)
	}
	_, err := r.run(ctx, args...)
	return err
}

// CreateRemote configures a non-OAuth backend such as MEGA from fields
// collected by ABM's graphical provider form. rclone obscures password
// values before writing its protected config file.
func (r *Runner) CreateRemote(ctx context.Context, name, providerType string, values map[string]string) error {
	args := []string{"config", "create", name, providerType, "--non-interactive", "--obscure"}
	for key, value := range values {
		if value != "" {
			args = append(args, key, value)
		}
	}
	_, err := r.run(ctx, args...)
	return err
}

// Test verifies a remote is reachable by listing its root directory.
func (r *Runner) Test(ctx context.Context, remote string) error {
	_, err := r.run(ctx, "lsd", remote+":")
	return err
}

// Reconnect re-runs a remote's OAuth flow to refresh an expired or revoked
// token, via rclone's own `config reconnect`. This is the normal path for
// providers whose session model expects periodic reauthentication (iCloud
// Drive, Proton Drive, Jottacloud) -- not an error-recovery hack.
func (r *Runner) Reconnect(ctx context.Context, remote string) error {
	_, err := r.run(ctx, "config", "reconnect", remote+":")
	return err
}
