// Package retention turns a config.Retention policy into the restic "forget"
// arguments that actually enforce it, and provides a pure-Go simulator used
// by tests to verify the policy's behavior without needing a real repository.
package retention

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
)

// DefaultKeepWithinHourly is the project default: 10 days of hourly history,
// expressed as a Go duration string because restic/Go have no "days" unit.
const DefaultKeepWithinHourly = "240h"

// BuildForgetArgs converts r into the flags for `restic forget`. It prefers
// the within-hourly window form (the project default) and falls back to
// explicit per-bucket counts when the config sets those instead.
func BuildForgetArgs(r config.Retention) ([]string, error) {
	if err := config.ValidateRetention(r); err != nil {
		return nil, err
	}

	var args []string
	if r.KeepWithinHourly != "" {
		args = append(args, "--keep-within-hourly", r.KeepWithinHourly)
	}
	if r.KeepHourly > 0 {
		args = append(args, "--keep-hourly", strconv.Itoa(r.KeepHourly))
	}
	if r.KeepDaily > 0 {
		args = append(args, "--keep-daily", strconv.Itoa(r.KeepDaily))
	}
	if r.KeepWeekly > 0 {
		args = append(args, "--keep-weekly", strconv.Itoa(r.KeepWeekly))
	}
	if r.KeepMonthly > 0 {
		args = append(args, "--keep-monthly", strconv.Itoa(r.KeepMonthly))
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("retention policy produced no forget arguments")
	}
	// --group-by ensures counts/windows are applied per job (host+paths+tags)
	// rather than collapsing unrelated jobs sharing a repository together.
	args = append(args, "--group-by", "host,paths,tags")
	return args, nil
}

// DefaultRetention returns the project default policy: retain hourly
// recovery points for the latest 10 days, pruned once a day.
func DefaultRetention() config.Retention {
	return config.Retention{
		KeepWithinHourly: DefaultKeepWithinHourly,
		PruneSchedule:    "daily",
	}
}

// Simulate reproduces restic's --keep-within-hourly bucket semantics in pure
// Go: within each 1-hour bucket that starts on or after (now - window), the
// single most recent snapshot is kept; everything else -- extra snapshots in
// an already-covered bucket, and any snapshot older than the window -- is
// dropped. It exists so retention behavior can be unit-tested without a real
// restic repository; the actual enforcement always runs through restic
// itself via BuildForgetArgs.
func Simulate(snapshots []time.Time, now time.Time, window time.Duration) (kept []time.Time, removed []time.Time) {
	cutoff := now.Add(-window)

	type bucketKey struct {
		year       int
		month      time.Month
		day, hour  int
	}
	bestInBucket := map[bucketKey]time.Time{}

	sorted := make([]time.Time, len(snapshots))
	copy(sorted, snapshots)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })

	for _, t := range sorted {
		if t.Before(cutoff) {
			removed = append(removed, t)
			continue
		}
		k := bucketKey{t.Year(), t.Month(), t.Day(), t.Hour()}
		if existing, ok := bestInBucket[k]; !ok || t.After(existing) {
			bestInBucket[k] = t
		}
	}

	for _, t := range sorted {
		if t.Before(cutoff) {
			continue
		}
		k := bucketKey{t.Year(), t.Month(), t.Day(), t.Hour()}
		if bestInBucket[k].Equal(t) {
			kept = append(kept, t)
		} else {
			removed = append(removed, t)
		}
	}
	return kept, removed
}
