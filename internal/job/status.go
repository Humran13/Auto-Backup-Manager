// Package job orchestrates a single backup job end to end: acquiring its
// lock, dumping any configured databases, running restic against one or more
// destinations, verifying the result, and recording status for `abm
// status`/`abm doctor` to report.
package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DestinationResult is one destination's outcome from the most recent run
// that touched it. A job with multiple destinations has one of these per
// destination, so a degraded secondary never hides behind an overall
// "success" the way a single flat status field would.
type DestinationResult struct {
	Name            string    `json:"name"`
	Initialized     bool      `json:"initialized,omitempty"`
	LastAttempt     time.Time `json:"last_attempt,omitempty"`
	LastSuccess     time.Time `json:"last_success,omitempty"`
	LastSnapshotID  string    `json:"last_snapshot_id,omitempty"`
	LastDuration    float64   `json:"last_duration_seconds,omitempty"`
	FilesNew        int       `json:"files_new,omitempty"`
	FilesChanged    int       `json:"files_changed,omitempty"`
	FilesUnmodified int       `json:"files_unmodified,omitempty"`
	DataAddedBytes  uint64    `json:"data_added_bytes,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
}

// Status is the persisted record of a job's most recent run. It is the
// single source of truth `abm status` reads, so a run must update it exactly
// once it knows the real outcome -- never optimistically before restic
// confirms success.
type Status struct {
	Job         string    `json:"job"`
	LastAttempt time.Time `json:"last_attempt"`
	// LastSuccess/LastError/LastWarning reflect the *overall run* outcome
	// under the job's destination policy (see config.Job.EffectivePolicy),
	// not any one destination -- see Destinations for the per-destination
	// detail backing this summary.
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	LastWarning string    `json:"last_warning,omitempty"`
	// Degraded is true when the primary destination succeeded but at least
	// one secondary destination failed under PolicyPrimaryRequired -- a real
	// condition worth surfacing, not a silent partial success.
	Degraded         bool                `json:"degraded,omitempty"`
	Destinations     []DestinationResult `json:"destinations,omitempty"`
	NextScheduledRun time.Time           `json:"next_scheduled_run,omitempty"`
}

// Destination returns the stored result for name, creating a zero-value
// entry (appended to s.Destinations) if none exists yet.
func (s *Status) Destination(name string) *DestinationResult {
	for i := range s.Destinations {
		if s.Destinations[i].Name == name {
			return &s.Destinations[i]
		}
	}
	s.Destinations = append(s.Destinations, DestinationResult{Name: name})
	return &s.Destinations[len(s.Destinations)-1]
}

// Primary returns the result for whichever destination was recorded first
// in this status, which is the job's actual primary as long as the job's
// destination order hasn't changed since the status was first written. If
// an admin reorders an existing job's destinations, display code reading
// Primary() may label the old primary's result until a fresh run rewrites
// status for the new order; this only affects cosmetic labeling, not which
// destination Run() actually treats as primary (it always uses the current
// config's Destinations[0] directly). Returns nil if the job has no
// recorded destinations yet.
func (s *Status) Primary() *DestinationResult {
	if len(s.Destinations) == 0 {
		return nil
	}
	return &s.Destinations[0]
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
