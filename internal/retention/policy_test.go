package retention

import (
	"testing"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
)

func TestBuildForgetArgs_DefaultKeepWithinHourly(t *testing.T) {
	args, err := BuildForgetArgs(DefaultRetention())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--keep-within-hourly", "240h", "--group-by", "host,paths,tags"}
	if !equalArgs(args, want) {
		t.Fatalf("got %v, want %v", args, want)
	}
}

func TestBuildForgetArgs_CountBased(t *testing.T) {
	args, err := BuildForgetArgs(config.Retention{KeepHourly: 24, KeepDaily: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--keep-hourly", "24", "--keep-daily", "10", "--group-by", "host,paths,tags"}
	if !equalArgs(args, want) {
		t.Fatalf("got %v, want %v", args, want)
	}
}

func TestBuildForgetArgs_EmptyPolicyRejected(t *testing.T) {
	if _, err := BuildForgetArgs(config.Retention{}); err == nil {
		t.Fatal("expected error for an all-zero retention policy")
	}
}

func TestBuildForgetArgs_InvalidWindowRejected(t *testing.T) {
	if _, err := BuildForgetArgs(config.Retention{KeepWithinHourly: "not-a-duration"}); err == nil {
		t.Fatal("expected error for an invalid duration string")
	}
}

// TestSimulate_TenDayHourlyRetention is the "10-day/hourly retention
// simulation" the project's test plan calls for: 15 days of perfectly
// on-schedule hourly snapshots should leave exactly the most recent 240
// (10 days * 24 hours) after applying the default project retention window.
func TestSimulate_TenDayHourlyRetention(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	start := now.Add(-15 * 24 * time.Hour)

	var snapshots []time.Time
	for t := start; t.Before(now); t = t.Add(time.Hour) {
		snapshots = append(snapshots, t)
	}

	kept, removed := Simulate(snapshots, now, 240*time.Hour)

	if len(kept) != 240 {
		t.Fatalf("expected 240 kept snapshots, got %d", len(kept))
	}
	if len(removed) != len(snapshots)-240 {
		t.Fatalf("expected %d removed snapshots, got %d", len(snapshots)-240, len(removed))
	}
	cutoff := now.Add(-240 * time.Hour)
	for _, k := range kept {
		if k.Before(cutoff) {
			t.Fatalf("kept snapshot %v is older than the retention window", k)
		}
	}
	for _, rm := range removed {
		if !rm.Before(cutoff) {
			t.Fatalf("removed snapshot %v was actually within the retention window", rm)
		}
	}
}

// TestSimulate_MultipleSnapshotsSameHour verifies that when a bucket has more
// than one snapshot (e.g. a manual "backup now" plus the scheduled hourly
// run), only the most recent one in that hour survives.
func TestSimulate_MultipleSnapshotsSameHour(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	older := now.Add(-30 * time.Minute)
	newer := now.Add(-5 * time.Minute)

	kept, removed := Simulate([]time.Time{older, newer}, now, 24*time.Hour)

	if len(kept) != 1 || !kept[0].Equal(newer) {
		t.Fatalf("expected only the newer same-hour snapshot to be kept, got kept=%v removed=%v", kept, removed)
	}
	if len(removed) != 1 || !removed[0].Equal(older) {
		t.Fatalf("expected the older same-hour snapshot to be removed, got %v", removed)
	}
}

// TestSimulate_NothingWithinWindow ensures a snapshot list entirely outside
// the retention window keeps nothing, rather than defaulting to "keep all".
func TestSimulate_NothingWithinWindow(t *testing.T) {
	now := time.Now()
	old := now.Add(-30 * 24 * time.Hour)
	kept, removed := Simulate([]time.Time{old}, now, 240*time.Hour)
	if len(kept) != 0 {
		t.Fatalf("expected nothing kept, got %v", kept)
	}
	if len(removed) != 1 {
		t.Fatalf("expected the one old snapshot to be removed, got %v", removed)
	}
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
