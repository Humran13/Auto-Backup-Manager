// Package restic wraps the restic CLI: it is the only place in the codebase
// that shells out to restic, so locking, timeouts, secret redaction and JSON
// parsing are handled consistently everywhere restic is used.
package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner executes restic against one repository. Repository is a restic
// repository spec; for cloud destinations this is "rclone:<remote>:<path>",
// which makes restic itself invoke rclone as its data-transport backend.
// PasswordFile must point at a file containing only the repository password,
// created with OS-restricted permissions by internal/secrets -- the password
// is never passed as a command-line argument or logged.
type Runner struct {
	BinaryPath   string
	Repository   string
	PasswordFile string
	RcloneConfig string // path to rclone.conf; propagated via RCLONE_CONFIG
	Env          []string
	// ExtraArgs are additional global restic flags (e.g. "-o",
	// "sftp.command=...") inserted right after "-r <repo>", before the
	// subcommand. See internal/backend.Target.ExtraArgs for why SFTP needs
	// this for a non-default port or key file.
	ExtraArgs []string
	Timeout   time.Duration
}

// Snapshot mirrors the subset of `restic snapshots --json` fields the
// manager needs.
type Snapshot struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"short_id"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Paths    []string  `json:"paths"`
	Tags     []string  `json:"tags"`
}

// BackupSummary mirrors restic's final JSON message from `backup --json`,
// which reports what actually happened rather than assuming success.
type BackupSummary struct {
	MessageType     string  `json:"message_type"` // "summary" on success
	FilesNew        int     `json:"files_new"`
	FilesChanged    int     `json:"files_changed"`
	FilesUnmodified int     `json:"files_unmodified"`
	DataAdded       uint64  `json:"data_added"`
	TotalDuration   float64 `json:"total_duration"`
	SnapshotID      string  `json:"snapshot_id"`
}

func (r *Runner) binary() string {
	if r.BinaryPath != "" {
		return r.BinaryPath
	}
	return "restic"
}

// run executes restic with args, returning stdout. Stderr is captured and
// included in the error on failure, with the repository password's own file
// path (never its contents) as the only repository secret restic ever sees.
func (r *Runner) run(ctx context.Context, args ...string) ([]byte, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}

	full := append([]string{"-r", r.Repository}, r.ExtraArgs...)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, r.binary(), full...)
	// Start from the parent environment (PATH, LOCALAPPDATA/HOME for
	// restic's own cache dir, etc.) rather than replacing it -- restic and
	// any rclone subprocess it spawns both need it.
	cmd.Env = append(os.Environ(), r.baseEnv()...)
	cmd.Env = append(cmd.Env, r.Env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("restic %s: %w: %s", args[0], err, redact(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (r *Runner) baseEnv() []string {
	env := []string{
		"RESTIC_PASSWORD_FILE=" + r.PasswordFile,
	}
	if r.RcloneConfig != "" {
		env = append(env, "RCLONE_CONFIG="+r.RcloneConfig)
	}
	return env
}

// redact strips anything that looks like a secret from restic's stderr
// before it is wrapped into an error that may end up in logs.
func redact(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// Init creates a new repository. If a repository already exists at this
// location (e.g. it was created by a prior run, or restored from elsewhere),
// Init treats that as success rather than an error.
func (r *Runner) Init(ctx context.Context) error {
	_, err := r.run(ctx, "init")
	if err != nil && isAlreadyInitialized(err) {
		return nil
	}
	return err
}

// isAlreadyInitialized recognizes restic's "config already exists" family of
// errors, the one case where a failed `init` call means the repository is
// fine, not broken. Every other init failure (network, auth, permissions)
// must propagate as a real error: callers rely on this to decide whether to
// ever retry initialization, and initializing over an unreachable-but-not-
// actually-missing repository must never look like "success".
func isAlreadyInitialized(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "already initialized") ||
		strings.Contains(msg, "already exists")
}

// Exists reports whether the repository is already initialized.
func (r *Runner) Exists(ctx context.Context) (bool, error) {
	_, err := r.run(ctx, "cat", "config")
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "unable to open") || strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "Is there a repository") {
		return false, nil
	}
	return false, err
}

// BackupOptions configures a single `restic backup` invocation.
type BackupOptions struct {
	Paths         []string
	Excludes      []string
	Tags          []string
	MaxFileSizeMB int64
	UseFSSnapshot bool // Windows VSS via restic --use-fs-snapshot
}

// Backup runs `restic backup` and returns the parsed summary. A non-nil
// error, or a summary whose MessageType is not "summary", both mean the
// backup did not succeed and must not be reported to the caller as success.
func (r *Runner) Backup(ctx context.Context, opts BackupOptions) (*BackupSummary, error) {
	args := []string{"backup", "--json"}
	for _, e := range opts.Excludes {
		args = append(args, "--exclude", e)
	}
	for _, t := range opts.Tags {
		args = append(args, "--tag", t)
	}
	if opts.MaxFileSizeMB > 0 {
		args = append(args, "--exclude-larger-than", fmt.Sprintf("%dM", opts.MaxFileSizeMB))
	}
	if opts.UseFSSnapshot {
		args = append(args, "--use-fs-snapshot")
	}
	args = append(args, opts.Paths...)

	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseBackupSummary(out)
}

// parseBackupSummary scans restic's newline-delimited JSON messages for the
// terminal "summary" message, which is the only message that confirms the
// snapshot was actually written.
func parseBackupSummary(out []byte) (*BackupSummary, error) {
	lines := bytes.Split(out, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var probe struct {
			MessageType string `json:"message_type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}
		if probe.MessageType == "summary" {
			var s BackupSummary
			if err := json.Unmarshal(line, &s); err != nil {
				return nil, fmt.Errorf("parsing backup summary: %w", err)
			}
			return &s, nil
		}
	}
	return nil, fmt.Errorf("restic backup produced no summary message; treat as failed")
}

// Snapshots lists snapshots, optionally filtered by tag.
func (r *Runner) Snapshots(ctx context.Context, tags []string) ([]Snapshot, error) {
	args := []string{"snapshots", "--json"}
	for _, t := range tags {
		args = append(args, "--tag", t)
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	if err := json.Unmarshal(out, &snaps); err != nil {
		return nil, fmt.Errorf("parsing snapshots: %w", err)
	}
	return snaps, nil
}

// Forget runs `restic forget` with the given policy args (see
// internal/retention.BuildForgetArgs), optionally pruning immediately.
func (r *Runner) Forget(ctx context.Context, policyArgs []string, prune bool) error {
	args := append([]string{"forget"}, policyArgs...)
	if prune {
		args = append(args, "--prune")
	}
	_, err := r.run(ctx, args...)
	return err
}

// Check runs `restic check`, optionally verifying a subset of pack data
// rather than downloading the entire repository every time.
func (r *Runner) Check(ctx context.Context, readDataSubset string) error {
	args := []string{"check"}
	if readDataSubset != "" {
		args = append(args, "--read-data-subset", readDataSubset)
	}
	_, err := r.run(ctx, args...)
	return err
}

// Unlock removes stale repository locks left by a killed or crashed process.
func (r *Runner) Unlock(ctx context.Context) error {
	_, err := r.run(ctx, "unlock")
	return err
}

// RestoreOptions configures a single `restic restore` invocation. Target
// must be a directory distinct from the original source by default; in-place
// restores are an explicit, separate opt-in enforced by callers, not here.
type RestoreOptions struct {
	SnapshotID string // "latest" is a valid restic snapshot ID
	Target     string
	Include    []string
}

// Restore runs `restic restore`.
func (r *Runner) Restore(ctx context.Context, opts RestoreOptions) error {
	if opts.Target == "" {
		return fmt.Errorf("restore target directory is required")
	}
	args := []string{"restore", opts.SnapshotID, "--target", opts.Target}
	for _, inc := range opts.Include {
		args = append(args, "--include", inc)
	}
	_, err := r.run(ctx, args...)
	return err
}

// Latest returns the newest snapshot by timestamp, optionally restricted to
// tags. This is the single implementation of "latest" semantics: the newest
// *successful* snapshot, never a destructive overwrite of prior history.
func Latest(snapshots []Snapshot) (*Snapshot, error) {
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots available")
	}
	best := snapshots[0]
	for _, s := range snapshots[1:] {
		if s.Time.After(best.Time) {
			best = s
		}
	}
	return &best, nil
}
