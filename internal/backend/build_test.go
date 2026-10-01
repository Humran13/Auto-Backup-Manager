package backend

import (
	"strings"
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
func (f *fakeStore) Set(key, value string) error     { f.values[key] = value; return nil }
func (f *fakeStore) Path(key string) (string, error) { return f.Get(key) }

func TestBuild_Local(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "local", Options: map[string]string{"path": "/mnt/backup"}}
	target, err := Build(s, "org/dev/job1", &fakeStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Spec != "/mnt/backup/org/dev/job1" {
		t.Fatalf("got %q", target.Spec)
	}
}

func TestBuild_SFTP_DefaultPortNoKeyFile(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "sftp", Options: map[string]string{
		"host": "box.example.com", "username": "u123",
	}}
	target, err := Build(s, "org/dev/job1", &fakeStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Spec != "sftp:u123@box.example.com:org/dev/job1" {
		t.Fatalf("got %q", target.Spec)
	}
	if len(target.ExtraArgs) != 0 {
		t.Fatalf("expected no sftp.command override for the default port/no key file, got %v", target.ExtraArgs)
	}
}

// TestBuild_SFTP_NonDefaultPortUsesCommandOverride guards a real bug found
// during manual testing against a disposable SFTP container: restic's plain
// "sftp:user@host:/path" syntax has no way to express a non-default port
// (e.g. Hetzner Storage Box's port 23, called out in docs/providers/SFTP.md)
// -- "host:port" is not valid restic syntax and silently produces a bogus
// hostname restic can't connect to. A non-default port must instead become
// an explicit `-o sftp.command=...` override.
func TestBuild_SFTP_NonDefaultPortUsesCommandOverride(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "sftp", Options: map[string]string{
		"host": "box.example.com", "port": "23", "username": "u123",
	}}
	target, err := Build(s, "org/dev/job1", &fakeStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Spec != "sftp:u123@box.example.com:org/dev/job1" {
		t.Fatalf("got %q", target.Spec)
	}
	if len(target.ExtraArgs) != 2 || target.ExtraArgs[0] != "-o" {
		t.Fatalf("expected a -o sftp.command override, got %v", target.ExtraArgs)
	}
	if !strings.Contains(target.ExtraArgs[1], "-p 23") {
		t.Fatalf("expected the override to include -p 23, got %q", target.ExtraArgs[1])
	}
}

func TestBuild_SFTP_KeyFileUsesCommandOverride(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "sftp", Options: map[string]string{
		"host": "box.example.com", "username": "u123", "key_file": "/keys/id_ed25519",
	}}
	target, err := Build(s, "org/dev/job1", &fakeStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(target.ExtraArgs) != 2 || !strings.Contains(target.ExtraArgs[1], "-i /keys/id_ed25519") {
		t.Fatalf("expected the override to include -i /keys/id_ed25519, got %v", target.ExtraArgs)
	}
}

// TestBuild_SFTP_WindowsKeyFilePathUsesForwardSlashes guards a real bug
// found during manual testing against a disposable SFTP container: restic
// parses sftp.command with POSIX shell word-splitting, where backslash is
// an escape character, so a raw Windows path (C:\keys\id_ed25519) was
// silently mangled and ssh ended up trying to resolve a path fragment as a
// hostname. Forward slashes survive that parsing intact and Windows OpenSSH
// accepts them just as well.
func TestBuild_SFTP_WindowsKeyFilePathUsesForwardSlashes(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "sftp", Options: map[string]string{
		"host": "box.example.com", "username": "u123", "key_file": `C:\keys\id_ed25519`,
	}}
	target, err := Build(s, "org/dev/job1", &fakeStore{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(target.ExtraArgs[1], `\`) {
		t.Fatalf("expected no backslashes in the sftp.command override, got %q", target.ExtraArgs[1])
	}
	if !strings.Contains(target.ExtraArgs[1], "-i C:/keys/id_ed25519") {
		t.Fatalf("expected -i C:/keys/id_ed25519, got %q", target.ExtraArgs[1])
	}
}

func TestBuild_S3_UsesSecretStoreAndSetsEnv(t *testing.T) {
	store := &fakeStore{values: map[string]string{
		SecretKey("d", "access_key"): "AKIA_FAKE",
		SecretKey("d", "secret_key"): "fake-secret",
	}}
	s := config.Storage{Name: "d", Provider: "generic-s3", Options: map[string]string{
		"bucket": "mybucket", "endpoint": "s3.example.com", "region": "us-west-1",
	}}
	target, err := Build(s, "org/dev/job1", store, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Spec != "s3:https://s3.example.com/mybucket/org/dev/job1" {
		t.Fatalf("got %q", target.Spec)
	}
	if !containsEnv(target.Env, "AWS_ACCESS_KEY_ID=AKIA_FAKE") || !containsEnv(target.Env, "AWS_SECRET_ACCESS_KEY=fake-secret") {
		t.Fatalf("missing credential env vars: %v", target.Env)
	}
}

func TestBuild_S3_MissingCredentialsProducesActionableError(t *testing.T) {
	s := config.Storage{Name: "mystorage", Provider: "generic-s3", Options: map[string]string{"bucket": "b", "endpoint": "e"}}
	_, err := Build(s, "p", &fakeStore{}, "")
	if err == nil {
		t.Fatal("expected an error for missing S3 credentials")
	}
	if !strings.Contains(err.Error(), "abm storage add --provider generic-s3 --name mystorage") {
		t.Fatalf("error should name the exact fix command, got: %v", err)
	}
}

func TestBuild_Rclone(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "onedrive", Options: map[string]string{"remote": "gdrive-primary"}}
	target, err := Build(s, "org/dev/job1", &fakeStore{}, "/etc/auto-backup-manager/rclone.conf")
	if err != nil {
		t.Fatal(err)
	}
	if target.Spec != "rclone:gdrive-primary:org/dev/job1" {
		t.Fatalf("got %q", target.Spec)
	}
	if target.RcloneConfig == "" {
		t.Fatal("expected RcloneConfig to be set for an rclone-backed target")
	}
}

func TestBuild_UnknownProvider(t *testing.T) {
	s := config.Storage{Name: "d", Provider: "does-not-exist"}
	if _, err := Build(s, "p", &fakeStore{}, ""); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}
