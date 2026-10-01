package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

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

// resticPasswordKey is the secret-store key holding a job's repository
// password. Each job gets its own repository (and therefore its own
// password) so one compromised job never exposes another's history.
func resticPasswordKey(jobName string) string {
	return "restic-password-" + jobName
}

func repositorySpec(storage config.Storage, job config.Job) string {
	if storage.Type == config.StorageLocal {
		return storage.Options["path"] + "/" + job.RepositoryPath
	}
	return "rclone:" + storage.RcloneRemote + ":" + job.RepositoryPath
}

// Run executes jobName end to end: lock -> dump databases -> restic backup
// -> verify -> record status -> unlock. It never reports success unless
// restic itself confirmed a snapshot was written, and a failed run leaves
// the previous successful snapshot completely untouched, since restic backup
// only ever adds a new snapshot rather than replacing existing ones.
func Run(ctx context.Context, deps *Deps, jobName string) (*Status, error) {
	job, ok := deps.Config.Jobs[jobName]
	if !ok {
		return nil, fmt.Errorf("no such job %q", jobName)
	}
	if !job.Enabled {
		return nil, fmt.Errorf("job %q is disabled", jobName)
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
	status.LastAttempt = deps.now()

	storage := findStorage(deps.Config.Storage, job.Destination)
	if storage == nil {
		return fail(deps, status, fmt.Sprintf("destination %q not found", job.Destination))
	}
	status.Destination = storage.Name

	for _, src := range job.Sources {
		if _, err := os.Stat(src); err != nil {
			return fail(deps, status, fmt.Sprintf("source path %q unavailable: %v", src, err))
		}
	}

	passwordFile, err := deps.Secrets.Path(resticPasswordKey(jobName))
	if err != nil {
		return fail(deps, status, fmt.Sprintf("repository password unavailable: %v", err))
	}

	r := &restic.Runner{
		BinaryPath:   deps.ResticBinary,
		Repository:   repositorySpec(*storage, job),
		PasswordFile: passwordFile,
		RcloneConfig: deps.RcloneConfig,
		Timeout:      2 * time.Hour,
	}

	// Only ever attempt to initialize a repository once per job. After that,
	// a repository that has become unreachable must surface as a failed
	// backup (restic's own "unable to open repository" error from Backup
	// below), never trigger a silent re-init that would orphan prior
	// snapshots under a brand-new empty repository at the same path.
	if !status.Initialized {
		if err := r.Init(ctx); err != nil {
			return fail(deps, status, fmt.Sprintf("initializing repository: %v", err))
		}
		status.Initialized = true
		if err := SaveStatus(deps.StateDir, status); err != nil {
			return status, fmt.Errorf("saving status after repository init: %w", err)
		}
		deps.Logger.Info("repository initialized", "job", jobName, "repository", r.Repository)
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
			return fail(deps, status, fmt.Sprintf("resolving credentials for database %q: %v", db.Name, err))
		}
		dumpPath, cleanup, err := database.Dump(ctx, db, creds, filepath.Join(deps.DumpDir, jobName))
		if cleanup != nil {
			dumpCleanups = append(dumpCleanups, cleanup)
		}
		if err != nil {
			return fail(deps, status, fmt.Sprintf("dumping database %q: %v", db.Name, err))
		}
		backupPaths = append(backupPaths, dumpPath)
	}

	useFSSnapshot := runtime.GOOS == "windows" && canUseFSSnapshot()
	status.LastWarning = ""
	if runtime.GOOS == "windows" && !useFSSnapshot {
		status.LastWarning = "VSS not used: process is not elevated, so open/locked files may be skipped or inconsistent. Scheduled runs (Task Scheduler, running as SYSTEM) are elevated and unaffected."
		deps.Logger.Warn(status.LastWarning, "job", jobName)
	}

	summary, err := r.Backup(ctx, restic.BackupOptions{
		Paths:         backupPaths,
		Excludes:      job.Excludes,
		Tags:          job.Tags,
		MaxFileSizeMB: job.MaxFileSizeMB,
		UseFSSnapshot: useFSSnapshot,
	})
	if err != nil {
		return fail(deps, status, fmt.Sprintf("backup failed: %v", err))
	}

	// Verify restic actually confirmed a snapshot, not merely that the
	// process exited zero.
	if summary.SnapshotID == "" {
		return fail(deps, status, "backup completed without a confirmed snapshot ID")
	}

	status.LastSuccess = deps.now()
	status.LastSnapshotID = summary.SnapshotID
	status.LastDuration = summary.TotalDuration
	status.FilesNew = summary.FilesNew
	status.FilesChanged = summary.FilesChanged
	status.FilesUnmodified = summary.FilesUnmodified
	status.DataAddedBytes = summary.DataAdded
	status.LastError = ""

	if err := SaveStatus(deps.StateDir, status); err != nil {
		return status, fmt.Errorf("backup succeeded but saving status failed: %w", err)
	}
	deps.Logger.Info("backup succeeded", "job", jobName, "snapshot", summary.SnapshotID,
		"new", summary.FilesNew, "changed", summary.FilesChanged, "unmodified", summary.FilesUnmodified)
	return status, nil
}

// Maintain runs `restic forget` (and, if prune is true, prune) for jobName
// using its configured retention policy, falling back to the project
// default of 10 days of hourly history. This is deliberately separate from
// Run so the hourly schedule can call Run every hour while a daily schedule
// calls Maintain once a day, avoiding unnecessary cloud load from pruning on
// every single backup.
func Maintain(ctx context.Context, deps *Deps, jobName string, prune bool) error {
	job, ok := deps.Config.Jobs[jobName]
	if !ok {
		return fmt.Errorf("no such job %q", jobName)
	}
	storage := findStorage(deps.Config.Storage, job.Destination)
	if storage == nil {
		return fmt.Errorf("destination %q not found", job.Destination)
	}

	policy := retention.DefaultRetention()
	if job.Retention != nil {
		policy = *job.Retention
	}
	args, err := retention.BuildForgetArgs(policy)
	if err != nil {
		return fmt.Errorf("building retention policy for %q: %w", jobName, err)
	}

	passwordFile, err := deps.Secrets.Path(resticPasswordKey(jobName))
	if err != nil {
		return fmt.Errorf("repository password unavailable: %w", err)
	}
	r := &restic.Runner{
		BinaryPath:   deps.ResticBinary,
		Repository:   repositorySpec(*storage, job),
		PasswordFile: passwordFile,
		RcloneConfig: deps.RcloneConfig,
		Timeout:      2 * time.Hour,
	}
	return r.Forget(ctx, args, prune)
}

// ResticRunner resolves a job's destination and repository password into a
// ready-to-use *restic.Runner, so CLI commands like `snapshots`, `restore`,
// and `check` share exactly the repository-resolution logic Run/Maintain use
// rather than re-deriving it.
func ResticRunner(deps *Deps, jobName string) (*restic.Runner, config.Job, error) {
	j, ok := deps.Config.Jobs[jobName]
	if !ok {
		return nil, config.Job{}, fmt.Errorf("no such job %q", jobName)
	}
	storage := findStorage(deps.Config.Storage, j.Destination)
	if storage == nil {
		return nil, j, fmt.Errorf("destination %q not found", j.Destination)
	}
	passwordFile, err := deps.Secrets.Path(resticPasswordKey(jobName))
	if err != nil {
		return nil, j, fmt.Errorf("repository password unavailable: %w", err)
	}
	return &restic.Runner{
		BinaryPath:   deps.ResticBinary,
		Repository:   repositorySpec(*storage, j),
		PasswordFile: passwordFile,
		RcloneConfig: deps.RcloneConfig,
		Timeout:      2 * time.Hour,
	}, j, nil
}

// fail records msg as the job's status and returns it as an error together
// with the status, so every failure path saves status exactly once.
func fail(deps *Deps, status *Status, msg string) (*Status, error) {
	status.LastError = msg
	if err := SaveStatus(deps.StateDir, status); err != nil {
		deps.Logger.Error("failed to save job status", "job", status.Job, "error", err)
	}
	return status, errors.New(msg)
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
