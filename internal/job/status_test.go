package job

import (
	"testing"
	"time"
)

func TestStatus_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &Status{
		Job:            "myjob",
		LastAttempt:    time.Now().Truncate(time.Second),
		LastSuccess:    time.Now().Truncate(time.Second),
		LastSnapshotID: "abc123",
		Initialized:    true,
		FilesNew:       5,
	}
	if err := SaveStatus(dir, original); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}

	loaded, err := LoadStatus(dir, "myjob")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if loaded.LastSnapshotID != original.LastSnapshotID || loaded.FilesNew != original.FilesNew || !loaded.Initialized {
		t.Fatalf("round-tripped status mismatch: got %+v, want %+v", loaded, original)
	}
}

func TestStatus_LoadMissingReturnsZeroValueNotError(t *testing.T) {
	dir := t.TempDir()
	st, err := LoadStatus(dir, "never-run")
	if err != nil {
		t.Fatalf("expected no error for a job that has never run, got: %v", err)
	}
	if !st.LastSuccess.IsZero() {
		t.Fatalf("expected zero-value LastSuccess, got %v", st.LastSuccess)
	}
	if st.Initialized {
		t.Fatal("a never-run job must not report as Initialized")
	}
}
