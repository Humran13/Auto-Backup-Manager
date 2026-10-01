package job

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/backend"
	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/database"
	"github.com/Humran13/Auto-Backup-Manager/internal/lock"
	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
	"github.com/Humran13/Auto-Backup-Manager/internal/retention"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// Deps bundles everything Run needs from the rest of the system, so job
// orchestration stays testable without a real filesystem/restic/OS lock.
type Deps struct {
	Config       *config.Config
	ResticBinary string
	RcloneConfig string
	StateDir     string
	LockDir      string
	DumpDir      string
	Secrets      secrets.Store
	DBCreds      secrets.Store // separate store keyed by Database.CredentialsRef
	Logger       *slog.Logger
	Now          func() time.Time
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// ResticPasswordKey is the secret-store key holding one destination's
// repository password for a job. Each (job, destination) pair gets its own
// repository and its own password, so one compromised destination never
// exposes another's history. Exported so `abm job add` can store a newly
// generated or recovered password under exactly the key Run/Maintain will
// look it up by.
func ResticPasswordKey(jobName, destName string) string {
	return "restic-password-" + jobName + "-" + destName
}

// legacyResticPasswordKey is the pre-multi-destination key shape
// ("restic-password-<job>", with no destination suffix). Still checked as a
// fallback for a job with exactly one destination, so a config that predates
// multi-destination support keeps working without a separate secret-store
// migration step.
func legacyResticPasswordKey(jobName string) string {
	return "restic-password-" + jobName
}

func resolvePasswordFile(store secrets.Store, jobName, destName string, numDestinations int) (string, error) {
	path, err := store.Path(ResticPasswordKey(jobName, destName))
	if err == nil {
		return path, nil
	}
	if numDestinations == 1 {
		if legacyPath, legacyErr := store.Path(legacyResticPasswordKey(jobName)); legacyErr == nil {
			return legacyPath, nil
		}
	}
	return "", err
}

// runnerFor builds a *restic.Runner for one (job, destination) pair.
func runnerFor(deps *Deps, jobName, destName string, job config.Job) (*restic.Runner, error) {
	storage := findStorage(deps.Config.Storage, destName)
	if storage == nil {
		return nil, fmt.Errorf("destination %q not found", destName)
	}
	passwordFile, err := resolvePasswordFile(deps.Secrets, jobName, destName, len(job.Destinations))
	if err != nil {
		return nil, fmt.Errorf("repository password unavailable: %w", err)
	}
	target, err := backend.Build(*storage, job.RepositoryPath, deps.Secrets, deps.RcloneConfig)
	if err != nil {
		return nil, err
	}
	return &restic.Runner{
		BinaryPath:   deps.ResticBinary,
		Repository:   target.Spec,
		PasswordFile: passwordFile,
		RcloneConfig: target.RcloneConfig,
		Env:          target.Env,
		ExtraArgs:    target.ExtraArgs,
		Timeout:      2 * time.Hour,
	}, nil
}

// Run executes jobName end to end: lock -> dump databases -> restic backup
// against each configured destination -> verify -> record status -> unlock.
// It never reports a destination successful unless restic itself confirmed a
// snapshot was written, and a failed destination leaves its previous
// successful snapshot completely untouched, since restic backup only ever
// adds a new snapshot rather than replacing existing ones.
//
// Destinations are backed up sequentially, not in parallel: they are
// independent repositories (safe to parallelize later) but sequential
// keeps logging and failure attribution simple for now. The job's
// EffectivePolicy decides whether a secondary destination's failure fails
// the whole run (PolicyAllRequired) or only marks it degraded while the
// primary's success still counts as the run succeeding (PolicyPrimaryRequired,
// the default).
func Run(ctx context.Context, deps *Deps, jobName string) (*Status, error) {
	job, ok := deps.Config.Jobs[jobName]
	if !ok {
		return nil, fmt.Errorf("no such job %q", jobName)
	}
	if !job.Enabled {
		return nil, fmt.Errorf("job %q is disabled", jobName)
	}
	if len(job.Destinations) == 0 {
		return nil, fmt.Errorf("job %q has no destinations configured", jobName)
	}

	l := lock.New(filepath.Join(deps.LockDir, jobName+".lock"))
	if err := l.Acquire(); err != nil {
		if lockedErr, ok := err.(*lock.ErrLocked); ok {
			deps.Logger.Warn("skipping run: job already in progress", "job", jobName, "pid", lockedErr.PID)
			return nil, lockedErr
		}
		return nil, fmt.Errorf("acquiring lock for %q: %w", jobName, err)
	}
	defer l.Release()

	status, err := LoadStatus(deps.StateDir, jobName)
	if err != nil {
		return nil, err
	}
	now := deps.now()
	status.LastAttempt = now
	status.LastWarning = ""

	for _, src := range job.Sources {
		if _, err := os.Stat(src); err != nil {
			return failRun(deps, status, fmt.Sprintf("source path %q unavailable: %v", src, err))
		}
	}

	backupPaths := append([]string{}, job.Sources...)
	var dumpCleanups []func()
	defer func() {
		for _, c := range dumpCleanups {
			c()
		}
	}()
	for _, db := range job.Databases {
		creds, err := resolveDBCredentials(deps.DBCreds, db)
		if err != nil {
			return failRun(deps, status, fmt.Sprintf("resolving credentials for database %q: %v", db.Name, err))
		}
		dumpPath, cleanup, err := database.Dump(ctx, db, creds, filepath.Join(deps.DumpDir, jobName))
		if cleanup != nil {
			dumpCleanups = append(dumpCleanups, cleanup)
		}
		if err != nil {
			return failRun(deps, status, fmt.Sprintf("dumping database %q: %v", db.Name, err))
		}
		backupPaths = append(backupPaths, dumpPath)
	}

	useFSSnapshot := runtime.GOOS == "windows" && canUseFSSnapshot()
	if runtime.GOOS == "windows" && !useFSSnapshot {
		status.LastWarning = "VSS not used: process is not elevated, so open/locked files may be skipped or inconsistent. Scheduled runs (Task Scheduler, running as SYSTEM) are elevated and unaffected."
		deps.Logger.Warn(status.LastWarning, "job", jobName)
	}

	primaryOK := false
	anyFailed := false
	for i, destName := range job.Destinations {
		dr := status.Destination(destName)
		dr.LastAttempt = now
		dr.LastError = ""

		if err := runOneDestination(ctx, deps, jobName, destName, job, dr, backupPaths, useFSSnapshot); err != nil {
			dr.LastError = err.Error()
			anyFailed = true
			deps.Logger.Warn("backup to destination failed", "job", jobName, "destination", destName, "error", err)
			continue
		}
		if i == 0 {
			primaryOK = true
		}
	}

	status.Degraded = false
	status.LastError = ""
	switch job.EffectivePolicy() {
	case config.PolicyAllRequired:
		if anyFailed {
			failedNames := failedDestinations(status, job.Destinations)
			return failRun(deps, status, fmt.Sprintf("destination(s) failed under all-required policy: %v", failedNames))
		}
	default: // PolicyPrimaryRequired
		if !primaryOK {
			return failRun(deps, status, fmt.Sprintf("primary destination %q failed: %s", job.Primary(), status.Destination(job.Primary()).LastError))
		}
		if anyFailed {
			status.Degraded = true
			deps.Logger.Warn("backup degraded: primary succeeded but a secondary destination failed", "job", jobName)
		}
	}

	status.LastSuccess = now
	if err := SaveStatus(deps.StateDir, status); err != nil {
		return status, fmt.Errorf("backup succeeded but saving status failed: %w", err)
	}
	if primary := status.Primary(); primary != nil {
		deps.Logger.Info("backup succeeded", "job", jobName, "snapshot", primary.LastSnapshotID,
			"new", primary.FilesNew, "changed", primary.FilesChanged, "unmodified", primary.FilesUnmodified, "degraded", status.Degraded)
	}
	return status, nil
}

// runOneDestination backs jobName up to destName, updating dr in place.
func runOneDestination(ctx context.Context, deps *Deps, jobName, destName string, job config.Job, dr *DestinationResult, backupPaths []string, useFSSnapshot bool) error {
	r, err := runnerFor(deps, jobName, destName, job)
	if err != nil {
		return err
	}

	// Only ever attempt to initialize a repository once per (job,
	// destination). After that, a repository that has become unreachable
	// must surface as a failed backup (restic's own "unable to open
	// repository" error from Backup below), never trigger a silent re-init
	// that would orphan prior snapshots under a brand-new empty repository
	// at the same path.
	if !dr.Initialized {
		if err := r.Init(ctx); err != nil {
			return fmt.Errorf("initializing repository: %w", err)
		}
		dr.Initialized = true
		deps.Logger.Info("repository initialized", "job", jobName, "destination", destName, "repository", r.Repository)
	}

	summary, err := r.Backup(ctx, restic.BackupOptions{
		Paths:         backupPaths,
		Excludes:      job.Excludes,
		Tags:          job.Tags,
		MaxFileSizeMB: job.MaxFileSizeMB,
		UseFSSnapshot: useFSSnapshot,
	})
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	if summary.SnapshotID == "" {
		return fmt.Errorf("backup completed without a confirmed snapshot ID")
	}

	dr.LastSuccess = deps.now()
	dr.LastSnapshotID = summary.SnapshotID
	dr.LastDuration = summary.TotalDuration
	dr.FilesNew = summary.FilesNew
	dr.FilesChanged = summary.FilesChanged
	dr.FilesUnmodified = summary.FilesUnmodified
	dr.DataAddedBytes = summary.DataAdded
	return nil
}

func failedDestinations(status *Status, destinations []string) []string {
	var out []string
	for _, name := range destinations {
		if d := status.Destination(name); d.LastError != "" {
			out = append(out, name)
		}
	}
	return out
}

// failRun records msg as the overall run's status and returns it as an
// error together with the status, so every run-level failure path saves
// status exactly once.
func failRun(deps *Deps, status *Status, msg string) (*Status, error) {
	status.LastError = msg
	if err := SaveStatus(deps.StateDir, status); err != nil {
		deps.Logger.Error("failed to save job status", "job", status.Job, "error", err)
	}
	return status, fmt.Errorf("%s", msg)
}

// Maintain runs `restic forget` (and, if prune is true, prune) against every
// destination of jobName using its configured retention policy, falling back
// to the project default of 10 days of hourly history. This is deliberately
// separate from Run so the hourly schedule can call Run every hour while a
// daily schedule calls Maintain once a day, avoiding unnecessary cloud load
// from pruning on every single backup. A failure on one destination does not
// stop maintenance of the others; all errors are joined and returned.
func Maintain(ctx context.Context, deps *Deps, jobName string, prune bool) error {
	job, ok := deps.Config.Jobs[jobName]
	if !ok {
		return fmt.Errorf("no such job %q", jobName)
	}
	if len(job.Destinations) == 0 {
		return fmt.Errorf("job %q has no destinations configured", jobName)
	}

	policy := retention.DefaultRetention()
	if job.Retention != nil {
		policy = *job.Retention
	}
	args, err := retention.BuildForgetArgs(policy)
	if err != nil {
		return fmt.Errorf("building retention policy for %q: %w", jobName, err)
	}

	var firstErr error
	for _, destName := range job.Destinations {
		r, err := runnerFor(deps, jobName, destName, job)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("destination %q: %w", destName, err)
			}
			continue
		}
		if err := r.Forget(ctx, args, prune); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("destination %q: %w", destName, err)
		}
	}
	return firstErr
}

// ResticRunner resolves one (job, destination) pair's repository and
// password into a ready-to-use *restic.Runner, so CLI commands like
// `snapshots`, `restore`, and `check` share exactly the repository-
// resolution logic Run/Maintain use rather than re-deriving it. An empty
// destName resolves to the job's primary destination.
func ResticRunner(deps *Deps, jobName, destName string) (*restic.Runner, config.Job, error) {
	j, ok := deps.Config.Jobs[jobName]
	if !ok {
		return nil, config.Job{}, fmt.Errorf("no such job %q", jobName)
	}
	if destName == "" {
		destName = j.Primary()
	}
	if destName == "" {
		return nil, j, fmt.Errorf("job %q has no destinations configured", jobName)
	}
	r, err := runnerFor(deps, jobName, destName, j)
	return r, j, err
}

func findStorage(all []config.Storage, name string) *config.Storage {
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

func resolveDBCredentials(store secrets.Store, db config.Database) (database.Credentials, error) {
	if db.Kind == config.DatabaseSQLite {
		return database.Credentials{}, nil
	}
	raw, err := store.Get(db.CredentialsRef)
	if err != nil {
		return database.Credentials{}, err
	}
	// Stored as "username:password"; see internal/secrets for how
	// credentials_ref entries are written during `abm job add`.
	for i := 0; i < len(raw); i++ {
		if raw[i] == ':' {
			return database.Credentials{Username: raw[:i], Password: raw[i+1:]}, nil
		}
	}
	return database.Credentials{}, fmt.Errorf("credentials_ref %q is not in username:password form", db.CredentialsRef)
}
