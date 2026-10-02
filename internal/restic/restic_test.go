package restic

import (
	"fmt"
	"testing"
	"time"
)

func TestParseBackupSummary_FindsTerminalSummary(t *testing.T) {
	out := []byte(`{"message_type":"status","percent_done":0.5}
{"message_type":"status","percent_done":1.0}
{"message_type":"summary","files_new":3,"files_changed":1,"files_unmodified":2,"data_added":1024,"total_duration":1.5,"snapshot_id":"abc123"}
`)
	summary, err := parseBackupSummary(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.SnapshotID != "abc123" {
		t.Fatalf("expected snapshot id abc123, got %q", summary.SnapshotID)
	}
	if summary.FilesNew != 3 || summary.FilesChanged != 1 || summary.FilesUnmodified != 2 {
		t.Fatalf("unexpected file counts: %+v", summary)
	}
}

// TestParseBackupSummary_NoSummaryMeansFailure guards the invariant that a
// backup is only ever reported successful when restic's own terminal summary
// message confirms it -- a truncated/killed run must not be mistaken for
// success just because some status lines were printed.
func TestParseBackupSummary_NoSummaryMeansFailure(t *testing.T) {
	out := []byte(`{"message_type":"status","percent_done":0.5}
`)
	if _, err := parseBackupSummary(out); err == nil {
		t.Fatal("expected an error when restic never emitted a summary message")
	}
}

func TestParseBackupSummary_EmptyOutputIsFailure(t *testing.T) {
	if _, err := parseBackupSummary(nil); err == nil {
		t.Fatal("expected an error for empty restic output")
	}
}

func TestLatest_PicksNewestByTime(t *testing.T) {
	older := Snapshot{ID: "old", Time: time.Now().Add(-2 * time.Hour)}
	newer := Snapshot{ID: "new", Time: time.Now()}
	middle := Snapshot{ID: "mid", Time: time.Now().Add(-1 * time.Hour)}

	latest, err := Latest([]Snapshot{older, newer, middle})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest.ID != "new" {
		t.Fatalf("expected newest snapshot, got %q", latest.ID)
	}
}

func TestLatest_EmptyListIsError(t *testing.T) {
	if _, err := Latest(nil); err == nil {
		t.Fatal("expected an error for an empty snapshot list")
	}
}

func TestIsAlreadyInitialized(t *testing.T) {
	cases := map[string]bool{
		"repository master key and config already initialized":  true,
		"config file already exists":                            true,
		"unable to open config file: no such file or directory": false,
		"connection refused":                                    false,
	}
	for msg, want := range cases {
		got := isAlreadyInitialized(errString(msg))
		if got != want {
			t.Errorf("isAlreadyInitialized(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestWindowsTimestampOnlyRestoreError(t *testing.T) {
	metadataOnly := fmt.Errorf(`restic restore: exit status 1: failed to restore timestamp of "C:\\target\\C\\Users": Access is denied. Fatal: There were 1 errors`)
	if !isWindowsTimestampOnlyRestoreError(metadataOnly) {
		t.Fatal("expected timestamp-only restore error to be recognized")
	}
	contentFailure := fmt.Errorf(`failed to restore timestamp; verification failed for file.txt`)
	if isWindowsTimestampOnlyRestoreError(contentFailure) {
		t.Fatal("content verification failures must remain fatal")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
