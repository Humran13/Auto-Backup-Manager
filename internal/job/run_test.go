package job

import (
	"testing"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

type fakeStore struct{ values map[string]string }

func (f *fakeStore) Get(key string) (string, error) {
	v, ok := f.values[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}
func (f *fakeStore) Set(key, value string) error { f.values[key] = value; return nil }
func (f *fakeStore) Path(key string) (string, error) {
	v, err := f.Get(key)
	if err != nil {
		return "", err
	}
	return v, nil
}

func TestResolveDBCredentials_ParsesUserPassword(t *testing.T) {
	store := &fakeStore{values: map[string]string{"db-app": "appuser:s3cret"}}
	creds, err := resolveDBCredentials(store, config.Database{Kind: config.DatabaseMySQL, CredentialsRef: "db-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Username != "appuser" || creds.Password != "s3cret" {
		t.Fatalf("got %+v", creds)
	}
}

func TestResolveDBCredentials_SQLiteNeedsNoCredentials(t *testing.T) {
	store := &fakeStore{values: map[string]string{}}
	creds, err := resolveDBCredentials(store, config.Database{Kind: config.DatabaseSQLite})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Username != "" || creds.Password != "" {
		t.Fatalf("expected empty credentials for sqlite, got %+v", creds)
	}
}

func TestResolveDBCredentials_MalformedRefRejected(t *testing.T) {
	store := &fakeStore{values: map[string]string{"db-app": "no-colon-here"}}
	if _, err := resolveDBCredentials(store, config.Database{Kind: config.DatabaseMySQL, CredentialsRef: "db-app"}); err == nil {
		t.Fatal("expected an error for a credentials_ref not in username:password form")
	}
}

func TestFindStorage(t *testing.T) {
	all := []config.Storage{{Name: "a"}, {Name: "b"}}
	if s := findStorage(all, "b"); s == nil || s.Name != "b" {
		t.Fatalf("expected to find storage b, got %+v", s)
	}
	if s := findStorage(all, "missing"); s != nil {
		t.Fatalf("expected nil for a missing storage name, got %+v", s)
	}
}

func TestResolvePasswordFile_PrefersPerDestinationKey(t *testing.T) {
	store := &fakeStore{values: map[string]string{
		ResticPasswordKey("job1", "dest1"): "new-password",
		legacyResticPasswordKey("job1"):    "legacy-password",
	}}
	path, err := resolvePasswordFile(store, "job1", "dest1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "new-password" {
		t.Fatalf("expected the per-destination key to win, got %q", path)
	}
}

// TestResolvePasswordFile_FallsBackToLegacyKey covers a job migrated from
// before multi-destination support: its password was stored under the old
// "restic-password-<job>" key (no destination suffix), and must still be
// found without requiring a secret-store migration step, as long as the job
// still has exactly one destination.
func TestResolvePasswordFile_FallsBackToLegacyKey(t *testing.T) {
	store := &fakeStore{values: map[string]string{
		legacyResticPasswordKey("job1"): "legacy-password",
	}}
	path, err := resolvePasswordFile(store, "job1", "dest1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "legacy-password" {
		t.Fatalf("expected fallback to the legacy key, got %q", path)
	}
}

func TestResolvePasswordFile_NoFallbackWithMultipleDestinations(t *testing.T) {
	store := &fakeStore{values: map[string]string{
		legacyResticPasswordKey("job1"): "legacy-password",
	}}
	if _, err := resolvePasswordFile(store, "job1", "dest1", 2); err == nil {
		t.Fatal("expected no legacy fallback when the job has more than one destination")
	}
}
