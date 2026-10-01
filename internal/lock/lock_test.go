package lock

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFileLock_AcquireRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.lock")
	l := New(path)
	if err := l.Acquire(); err != nil {
		t.Fatalf("unexpected error acquiring free lock: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("unexpected error releasing lock: %v", err)
	}
}

func TestFileLock_SecondAcquireFailsWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.lock")
	first := New(path)
	if err := first.Acquire(); err != nil {
		t.Fatalf("unexpected error on first acquire: %v", err)
	}
	defer first.Release()

	second := New(path)
	err := second.Acquire()
	if err == nil {
		t.Fatal("expected second acquire to fail while the first process holds the lock")
	}
	if _, ok := err.(*ErrLocked); !ok {
		t.Fatalf("expected *ErrLocked, got %T: %v", err, err)
	}
}

func TestFileLock_ReclaimsAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.lock")
	first := New(path)
	if err := first.Acquire(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	second := New(path)
	if err := second.Acquire(); err != nil {
		t.Fatalf("expected to reacquire a released lock, got: %v", err)
	}
	second.Release()
}

// TestFileLock_ReclaimsStaleLockFromDeadProcess covers a lock file left
// behind by a process that crashed, was killed, or the machine rebooted
// without a clean shutdown: since no live process holds it, a new run must
// reclaim it rather than refusing to ever run again.
func TestFileLock_ReclaimsStaleLockFromDeadProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job.lock")
	// A PID essentially guaranteed not to correspond to a live process.
	const deadPID = 999999999
	if err := os.WriteFile(path, []byte(strconv.Itoa(deadPID)), 0o640); err != nil {
		t.Fatalf("setting up stale lock file: %v", err)
	}

	l := New(path)
	if err := l.Acquire(); err != nil {
		t.Fatalf("expected to reclaim a stale lock from a dead pid, got: %v", err)
	}
	l.Release()
}

func TestFileLock_ReleaseWithoutAcquireIsSafe(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "never-acquired.lock"))
	if err := l.Release(); err != nil {
		t.Fatalf("Release on a never-acquired lock should be a no-op, got: %v", err)
	}
}
