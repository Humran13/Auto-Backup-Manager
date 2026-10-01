// Package job orchestrates a single backup job end to end: acquiring its
// lock, dumping any configured databases, running restic, verifying the
// result, and recording status for `abm status`/`abm doctor` to report.
package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Status is the persisted record of a job's most recent runs. It is the
// single source of truth `abm status` reads, so a run must update it exactly
// once it knows the real outcome -- never optimistically before restic
// confirms success.
type Status struct {
	Job              string    `json:"job"`
	LastAttempt      time.Time `json:"last_attempt"`
	LastSuccess      time.Time `json:"last_success,omitempty"`
	LastSnapshotID   string    `json:"last_snapshot_id,omitempty"`
	LastDuration     float64   `json:"last_duration_seconds,omitempty"`
	FilesNew         int       `json:"files_new,omitempty"`
	FilesChanged     int       `json:"files_changed,omitempty"`
	FilesUnmodified  int       `json:"files_unmodified,omitempty"`
	DataAddedBytes   uint64    `json:"data_added_bytes,omitempty"`
	Destination      string    `json:"destination,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
	LastWarning      string    `json:"last_warning,omitempty"`
	// Initialized is set true the first time this job's repository is
	// successfully created (or confirmed to already exist). Once true, Run
	// never calls restic init again: a repository that has become
	// unreachable (network blip, deleted bucket, revoked credentials) must
	// surface as a failed backup, never be silently recreated as an empty
	// repository, which would look like "it's just a new job" while quietly
	// orphaning every prior snapshot.
	Initialized bool `json:"initialized,omitempty"`
	NextScheduledRun time.Time `json:"next_scheduled_run,omitempty"`
}

func statusPath(stateDir, jobName string) string {
	return filepath.Join(stateDir, "status", jobName+".json")
}

// LoadStatus reads a job's last recorded status, returning a zero-value
// Status (never an error) if the job has never run.
func LoadStatus(stateDir, jobName string) (*Status, error) {
	data, err := os.ReadFile(statusPath(stateDir, jobName))
	if os.IsNotExist(err) {
		return &Status{Job: jobName}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading status for %s: %w", jobName, err)
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing status for %s: %w", jobName, err)
	}
	return &s, nil
}

// SaveStatus atomically persists s.
func SaveStatus(stateDir string, s *Status) error {
	path := statusPath(stateDir, s.Job)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating status directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
