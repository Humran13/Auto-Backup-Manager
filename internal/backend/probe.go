package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// probeTempDir creates a temp directory for Probe's own use. On Windows it
// avoids the default os.TempDir() (nested under C:\Users\<user>\AppData,
// typically several directories deep): restic restore reconstructs a
// snapshot's full absolute source path under --target and tries to fix up
// every parent directory's timestamp along the way, including ones the
// current process doesn't own when running unelevated (e.g. C:\Users
// itself), which restic then reports as a fatal error even though the
// actual restored file content is correct. A shallow root directly under
// the temp directory's own drive sidesteps that ACL quirk; it falls back to
// the ordinary os.TempDir() if that drive-root isn't writable.
func probeTempDir(pattern string) (string, error) {
	if runtime.GOOS == "windows" {
		if vol := filepath.VolumeName(os.TempDir()); vol != "" {
			if dir, err := os.MkdirTemp(vol+`\`, pattern); err == nil {
				return dir, nil
			}
		}
	}
	return os.MkdirTemp("", pattern)
}

// probeSubPath is where Probe creates its disposable test repository, nested
// under the destination's own path rather than a randomly-named one that
// would then need backend-specific delete support (every backend family
// here -- local, sftp, s3, azure, gs, swift, rclone -- would need its own
// object-deletion code otherwise). Re-running Probe reuses and overwrites
// this same tiny repository idempotently instead.
const probeSubPath = ".abm-capability-test"

// Probe is the capability/round-trip test every storage destination must
// pass before ABM accepts it: initialize a repository, back up a tiny test
// file, list the snapshot, restore it, and verify the content matches. A
// destination that cannot complete this round-trip is not a safe restic
// repository target, whatever rclone backend or credentials it has.
//
// It intentionally does not attempt to delete the disposable repository
// afterward (see probeSubPath) -- that would need per-backend object
// deletion this project doesn't otherwise implement. The leftover is a few
// KB and clearly named.
func Probe(ctx context.Context, resticBinary string, storage config.Storage, secretStore secrets.Store, rcloneConfigPath string) error {
	target, err := Build(storage, probeSubPath, secretStore, rcloneConfigPath)
	if err != nil {
		return fmt.Errorf("resolving repository target: %w", err)
	}

	passwordFile, cleanup, err := ephemeralPasswordFile()
	if err != nil {
		return err
	}
	defer cleanup()

	r := &restic.Runner{
		BinaryPath:   resticBinary,
		Repository:   target.Spec,
		PasswordFile: passwordFile,
		RcloneConfig: target.RcloneConfig,
		Env:          target.Env,
		ExtraArgs:    target.ExtraArgs,
		// A capability test must never hang `abm storage add` indefinitely
		// (e.g. a slow/unresponsive destination); 2 minutes is generous for
		// a few KB of test data over any real network destination.
		Timeout: 2 * time.Minute,
	}

	if err := r.Init(ctx); err != nil {
		return fmt.Errorf("initializing capability-test repository: %w", err)
	}

	testDir, err := probeTempDir("abm-probe-src-*")
	if err != nil {
		return fmt.Errorf("creating probe source dir: %w", err)
	}
	defer os.RemoveAll(testDir)
	testContent := "Auto-Backup-Manager capability test\n"
	testFile := filepath.Join(testDir, "probe.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
		return fmt.Errorf("writing probe file: %w", err)
	}

	summary, err := r.Backup(ctx, restic.BackupOptions{Paths: []string{testDir}})
	if err != nil {
		return fmt.Errorf("probe backup failed: %w", err)
	}
	if summary.SnapshotID == "" {
		return fmt.Errorf("probe backup produced no confirmed snapshot")
	}

	snaps, err := r.Snapshots(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing probe snapshots: %w", err)
	}
	if len(snaps) == 0 {
		return fmt.Errorf("probe snapshot not visible after backup")
	}

	restoreDir, err := probeTempDir("abm-probe-restore-*")
	if err != nil {
		return fmt.Errorf("creating probe restore dir: %w", err)
	}
	defer os.RemoveAll(restoreDir)
	if err := r.Restore(ctx, restic.RestoreOptions{SnapshotID: "latest", Target: restoreDir}); err != nil {
		return fmt.Errorf("probe restore failed: %w", err)
	}

	restoredPath := restoredFilePath(restoreDir, testFile)
	got, err := os.ReadFile(restoredPath)
	if err != nil {
		return fmt.Errorf("probe restored file not found at %s: %w", restoredPath, err)
	}
	if string(got) != testContent {
		return fmt.Errorf("probe restored content mismatch: got %q, want %q", got, testContent)
	}
	return nil
}

// restoredFilePath mirrors where restic places a restored absolute path
// under --target (see docs/providers restore-test sections): the full
// source path is recreated underneath restoreDir.
func restoredFilePath(restoreDir, absSourceFile string) string {
	vol := filepath.VolumeName(absSourceFile)
	if vol != "" {
		return filepath.Join(restoreDir, vol[:len(vol)-1], absSourceFile[len(vol):])
	}
	return filepath.Join(restoreDir, absSourceFile)
}

// ephemeralPasswordFile creates a one-off restic repository password for a
// disposable probe repository -- this password is never meant to be reused
// or recovered, so it lives only in a temp file for the probe's duration.
func ephemeralPasswordFile() (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "abm-probe-pw-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating probe password file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString("abm-capability-test-password-not-a-real-secret"); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}
