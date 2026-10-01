// Package lock provides a filesystem-based mutex that stops two backup runs
// for the same job (a scheduled run and a manual "abm backup now", or two
// overlapping scheduled runs after a slow prior run) from executing at once.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ErrLocked is returned by Acquire when another process already holds the
// lock. Callers should treat this as "skip this run", not as a fatal error.
type ErrLocked struct {
	Path string
	PID  int
}

func (e *ErrLocked) Error() string {
	return fmt.Sprintf("lock %s held by pid %d", e.Path, e.PID)
}

// FileLock is a simple PID-file lock. It is not distributed and only
// protects a single machine, which matches the threat model: two schedulers
// (systemd timer + manual CLI, or Task Scheduler + manual CLI) on the same
// host racing to back up the same job.
type FileLock struct {
	path string
	file *os.File
}

// New returns a lock backed by a file at path, e.g.
// /var/lib/auto-backup-manager/locks/<job>.lock or
// C:\ProgramData\Auto-Backup-Manager\locks\<job>.lock.
func New(path string) *FileLock {
	return &FileLock{path: path}
}

// Acquire takes the lock or returns ErrLocked if another live process holds
// it. A lock file left behind by a process that no longer exists (crash,
// kill -9, reboot) is treated as stale and reclaimed automatically.
func (l *FileLock) Acquire() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o750); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}

	if pid, err := readPID(l.path); err == nil {
		if processAlive(pid) {
			return &ErrLocked{Path: l.path, PID: pid}
		}
		// Stale lock from a crashed/killed/rebooted process: reclaim it.
		_ = os.Remove(l.path)
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		if os.IsExist(err) {
			if pid, perr := readPID(l.path); perr == nil {
				return &ErrLocked{Path: l.path, PID: pid}
			}
		}
		return fmt.Errorf("acquiring lock %s: %w", l.path, err)
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		f.Close()
		os.Remove(l.path)
		return fmt.Errorf("writing lock pid: %w", err)
	}
	l.file = f
	return nil
}

// Release removes the lock file. Safe to call even if Acquire failed.
func (l *FileLock) Release() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	err := os.Remove(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}
