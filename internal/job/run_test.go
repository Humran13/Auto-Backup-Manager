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

func TestRepositorySpec(t *testing.T) {
	rcloneStorage := config.Storage{Type: config.StorageS3, RcloneRemote: "myremote"}
	j := config.Job{RepositoryPath: "org/dev/job1"}
	if got, want := repositorySpec(rcloneStorage, j), "rclone:myremote:org/dev/job1"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	localStorage := config.Storage{Type: config.StorageLocal, Options: map[string]string{"path": "/mnt/backup"}}
	if got, want := repositorySpec(localStorage, j), "/mnt/backup/org/dev/job1"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
