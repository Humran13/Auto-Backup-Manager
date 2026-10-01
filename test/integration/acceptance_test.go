// Package integration exercises Auto-Backup-Manager against a real restic
// binary and a local-filesystem destination. It automates the project's
// acceptance scenario end to end: backup, modify, backup, delete, backup,
// list snapshots, restore an old snapshot, restore latest, verify content,
// simulate a destination failure, and confirm the prior snapshot survives.
//
// It is skipped automatically if `restic` isn't on PATH, so it never blocks
// unit-test-only environments; CI installs restic explicitly to run it.
package integration

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// restoredPathFor mirrors where restic places a restored absolute path under
// --target: on Unix, target+absPath directly; on Windows, restic turns the
// drive letter into a folder (C:\foo -> target\C\foo).
func restoredPathFor(target, absSource string) string {
	if vol := filepath.VolumeName(absSource); vol != "" {
		return filepath.Join(target, strings.TrimSuffix(vol, ":"), absSource[len(vol):])
	}
	return filepath.Join(target, absSource)
}

func requireRestic(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic not found on PATH; skipping integration test")
	}
}

// memStore is an in-memory secrets.Store good enough for tests: Path writes
// the value to a temp file, matching what RESTIC_PASSWORD_FILE needs from a
// real store without touching OS-specific protection mechanisms.
type memStore struct {
	dir    string
	values map[string]string
}

func newMemStore(t *testing.T) *memStore {
	return &memStore{dir: t.TempDir(), values: map[string]string{}}
}
func (m *memStore) Get(key string) (string, error) {
	v, ok := m.values[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}
func (m *memStore) Set(key, value string) error { m.values[key] = value; return nil }
func (m *memStore) Path(key string) (string, error) {
	v, err := m.Get(key)
	if err != nil {
		return "", err
	}
	p := filepath.Join(m.dir, key)
	if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
		return "", err
	}
	return p, nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// testRoot returns a temp directory for the test. On Windows it deliberately
// avoids nesting under the user profile (where t.TempDir() lives): restic
// restore reconstructs a snapshot's full absolute path under --target and
// tries to fix up every parent directory's timestamp along the way,
// including ones the current process doesn't own when running unelevated
// (e.g. C:\Users itself), which restic then reports as a fatal error even
// though the actual restored files are correct. A shallow root under C:\
// sidesteps that ACL quirk; CI's windows-latest runners run elevated by
// default and would not need this, but local unelevated runs do.
func testRoot(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp(`C:\`, "abm-it-*")
	if err != nil {
		t.Skipf("cannot create a shallow test root under C:\\ (%v); skipping to avoid an unelevated Windows ACL false failure", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestAcceptance_BackupModifyDeleteRestore(t *testing.T) {
	requireRestic(t)

	root := testRoot(t)
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "destination")
	restoreADir := filepath.Join(root, "restoreA")
	restoreLatestDir := filepath.Join(root, "restoreLatest")

	writeFile(t, filepath.Join(sourceDir, "file1.txt"), "file1 original")
	writeFile(t, filepath.Join(sourceDir, "file2.txt"), "file2 original")
	writeFile(t, filepath.Join(sourceDir, "subdir", "file3.txt"), "file3 original")

	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Global:  config.Global{DeviceID: "dev-test", Organization: "testorg"},
		Storage: []config.Storage{{Name: "local", Type: config.StorageLocal, Options: map[string]string{"path": destDir}}},
		Jobs: map[string]config.Job{
			"testjob": {
				Sources:        []string{sourceDir},
				Destination:    "local",
				RepositoryPath: "testorg/dev-test/testjob",
				Retention:      &config.Retention{KeepWithinHourly: "240h"},
				Enabled:        true,
			},
		},
	}

	store := newMemStore(t)
	store.Set("restic-password-testjob", "test-password-not-a-real-secret")

	deps := &job.Deps{
		Config:   cfg,
		StateDir: filepath.Join(root, "state"),
		LockDir:  filepath.Join(root, "locks"),
		DumpDir:  filepath.Join(root, "dumps"),
		Secrets:  store,
		DBCreds:  store,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	ctx := context.Background()

	// A: first backup.
	stA, err := job.Run(ctx, deps, "testjob")
	if err != nil {
		t.Fatalf("backup A failed: %v", err)
	}
	snapshotA := stA.LastSnapshotID
	if snapshotA == "" {
		t.Fatal("backup A produced no snapshot id")
	}

	time.Sleep(1100 * time.Millisecond) // ensure a distinct restic timestamp

	// Modify + add new file, then backup B.
	writeFile(t, filepath.Join(sourceDir, "file1.txt"), "file1 MODIFIED")
	writeFile(t, filepath.Join(sourceDir, "file4.txt"), "file4 new")
	stB, err := job.Run(ctx, deps, "testjob")
	if err != nil {
		t.Fatalf("backup B failed: %v", err)
	}
	if stB.FilesNew != 1 || stB.FilesChanged != 1 {
		t.Fatalf("backup B: expected 1 new + 1 changed file, got new=%d changed=%d", stB.FilesNew, stB.FilesChanged)
	}

	// Delete files, then backup C.
	os.Remove(filepath.Join(sourceDir, "file2.txt"))
	os.RemoveAll(filepath.Join(sourceDir, "subdir"))
	stC, err := job.Run(ctx, deps, "testjob")
	if err != nil {
		t.Fatalf("backup C failed: %v", err)
	}
	latestSnapshot := stC.LastSnapshotID

	// H: list all snapshots.
	r, _, err := job.ResticRunner(deps, "testjob")
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := r.Snapshots(ctx, nil)
	if err != nil {
		t.Fatalf("listing snapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}

	// I/J: restore snapshot A and verify it exactly matches the original.
	if err := r.Restore(ctx, restoreOpts(snapshotA, restoreADir)); err != nil {
		t.Fatalf("restoring snapshot A: %v", err)
	}
	restoredSourceA := restoredPathFor(restoreADir, sourceDir)
	assertFileContent(t, restoredSourceA, "file1.txt", "file1 original")
	assertFileContent(t, restoredSourceA, "file2.txt", "file2 original")
	assertFileContent(t, filepath.Join(restoredSourceA, "subdir"), "file3.txt", "file3 original")
	if _, err := os.Stat(filepath.Join(restoredSourceA, "file4.txt")); !os.IsNotExist(err) {
		t.Fatal("file4.txt should not exist in snapshot A's restore")
	}

	// K/L: restore latest and verify it matches the expected post-deletion state.
	if err := r.Restore(ctx, restoreOpts("latest", restoreLatestDir)); err != nil {
		t.Fatalf("restoring latest: %v", err)
	}
	restoredSourceLatest := restoredPathFor(restoreLatestDir, sourceDir)
	assertFileContent(t, restoredSourceLatest, "file1.txt", "file1 MODIFIED")
	assertFileContent(t, restoredSourceLatest, "file4.txt", "file4 new")
	if _, err := os.Stat(filepath.Join(restoredSourceLatest, "file2.txt")); !os.IsNotExist(err) {
		t.Fatal("file2.txt should not exist in the latest restore (it was deleted before backup C)")
	}
	if _, err := os.Stat(filepath.Join(restoredSourceLatest, "subdir")); !os.IsNotExist(err) {
		t.Fatal("subdir should not exist in the latest restore (it was deleted before backup C)")
	}

	// M/N: simulate a destination failure and confirm the prior successful
	// snapshot remains completely intact and restorable.
	moved := destDir + "-moved"
	if err := os.Rename(destDir, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := job.Run(ctx, deps, "testjob"); err == nil {
		t.Fatal("expected backup D to fail when the destination is unreachable")
	}
	if err := os.Rename(moved, destDir); err != nil {
		t.Fatal(err)
	}

	snapsAfterFailure, err := r.Snapshots(ctx, nil)
	if err != nil {
		t.Fatalf("listing snapshots after failure: %v", err)
	}
	if len(snapsAfterFailure) != 3 {
		t.Fatalf("expected the same 3 snapshots after a failed run, got %d", len(snapsAfterFailure))
	}

	restoreAfterFailureDir := filepath.Join(root, "restoreAfterFailure")
	if err := r.Restore(ctx, restoreOpts(latestSnapshot, restoreAfterFailureDir)); err != nil {
		t.Fatalf("restore after failure must still work: %v", err)
	}
}

func restoreOpts(snapshotID, target string) restic.RestoreOptions {
	return restic.RestoreOptions{SnapshotID: snapshotID, Target: target}
}

func assertFileContent(t *testing.T, dir, name, want string) {
	t.Helper()
	got := readFile(t, filepath.Join(dir, name))
	if got != want {
		t.Fatalf("%s/%s: got %q, want %q", dir, name, got, want)
	}
}
