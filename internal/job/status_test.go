package job

import (
	"testing"
	"time"
)

func TestStatus_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &Status{
		Job:         "myjob",
		LastAttempt: time.Now().Truncate(time.Second),
		LastSuccess: time.Now().Truncate(time.Second),
	}
	dr := original.Destination("dest1")
	dr.LastSnapshotID = "abc123"
	dr.Initialized = true
	dr.FilesNew = 5

	if err := SaveStatus(dir, original); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}

	loaded, err := LoadStatus(dir, "myjob")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	primary := loaded.Primary()
	if primary == nil {
		t.Fatal("expected a primary destination result after round-trip")
	}
	if primary.LastSnapshotID != "abc123" || primary.FilesNew != 5 || !primary.Initialized {
		t.Fatalf("round-tripped status mismatch: got %+v", primary)
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
	if st.Primary() != nil {
		t.Fatal("a never-run job must have no destination results")
	}
}

func TestStatus_DestinationCreatesIfMissing(t *testing.T) {
	s := &Status{Job: "j"}
	d1 := s.Destination("a")
	d1.FilesNew = 1
	d2 := s.Destination("a") // same name: must return the same entry, not a duplicate
	if len(s.Destinations) != 1 {
		t.Fatalf("expected 1 destination entry, got %d", len(s.Destinations))
	}
	if d2.FilesNew != 1 {
		t.Fatalf("expected to retrieve the same entry, got %+v", d2)
	}
}

func TestStatus_PrimaryIsFirstDestinationAdded(t *testing.T) {
	s := &Status{Job: "j"}
	s.Destination("a").LastSnapshotID = "a-snap"
	s.Destination("b").LastSnapshotID = "b-snap"
	if s.Primary().Name != "a" {
		t.Fatalf("expected primary to be the first destination added, got %q", s.Primary().Name)
	}
}
