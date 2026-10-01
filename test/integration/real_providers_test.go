// This file implements the project's "real provider test matrix": protocol-
// level integration tests that exercise ABM's actual job.Run/backend.Build
// code (not raw restic calls) against a real S3-compatible server and a
// real SFTP server. Both are gated behind environment variables and skip
// cleanly when unset, so normal `go test ./...` and CI never require them;
// CI opts in by starting disposable local containers (adobe/s3mock,
// atmoz/sftp) and setting the variables, and a developer can do the same
// locally. Neither test ever needs or accepts real cloud credentials.
package integration

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/job"
)

// TestRealProvider_S3 exercises the generic-s3 provider (internal/backend's
// native S3 engine, shared by every S3-compatible preset) against a real
// S3-protocol server.
//
// Set ABM_TEST_S3_ENDPOINT (host:port, no scheme), ABM_TEST_S3_ACCESS_KEY,
// ABM_TEST_S3_SECRET_KEY, and ABM_TEST_S3_BUCKET to run it. CI starts
// adobe/s3mock for this; MinIO's Docker Hub images now require
// authentication to pull (verified during this project's development --
// see docs/TESTING.md), so this project uses s3mock as its disposable local
// S3-compatible test server instead.
func TestRealProvider_S3(t *testing.T) {
	endpoint := os.Getenv("ABM_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("ABM_TEST_S3_ENDPOINT not set; skipping real S3 protocol test")
	}
	accessKey := os.Getenv("ABM_TEST_S3_ACCESS_KEY")
	secretKey := os.Getenv("ABM_TEST_S3_SECRET_KEY")
	bucket := os.Getenv("ABM_TEST_S3_BUCKET")
	requireRestic(t)

	root := testRoot(t)
	sourceDir := filepath.Join(root, "source")
	writeFile(t, filepath.Join(sourceDir, "file1.txt"), "s3 real-provider test content")

	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Global:  config.Global{DeviceID: "dev-test", Organization: "testorg"},
		Storage: []config.Storage{{
			Name: "s3test", Provider: "generic-s3",
			Options: map[string]string{"endpoint": endpoint, "bucket": bucket},
		}},
		Jobs: map[string]config.Job{
			"s3job": {
				Sources:        []string{sourceDir},
				Destinations:   []string{"s3test"},
				RepositoryPath: "testorg/dev-test/s3job",
				Retention:      &config.Retention{KeepWithinHourly: "240h"},
				Enabled:        true,
			},
		},
	}

	store := newMemStore(t)
	store.Set("storage-s3test-access_key", accessKey)
	store.Set("storage-s3test-secret_key", secretKey)
	store.Set(job.ResticPasswordKey("s3job", "s3test"), "test-password-not-a-real-secret")

	deps := &job.Deps{
		Config:   cfg,
		StateDir: filepath.Join(root, "state"),
		LockDir:  filepath.Join(root, "locks"),
		DumpDir:  filepath.Join(root, "dumps"),
		Secrets:  store,
		DBCreds:  store,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	ctx := context.Background()

	st, err := job.Run(ctx, deps, "s3job")
	if err != nil {
		t.Fatalf("real S3 backup failed: %v", err)
	}
	snapshotID := st.Primary().LastSnapshotID
	if snapshotID == "" {
		t.Fatal("no snapshot id after real S3 backup")
	}

	time.Sleep(1100 * time.Millisecond)
	writeFile(t, filepath.Join(sourceDir, "file1.txt"), "s3 real-provider test content MODIFIED")
	st2, err := job.Run(ctx, deps, "s3job")
	if err != nil {
		t.Fatalf("real S3 second backup failed: %v", err)
	}
	if st2.Primary().FilesChanged != 1 {
		t.Fatalf("expected 1 changed file on the second backup, got %d", st2.Primary().FilesChanged)
	}

	r, _, err := job.ResticRunner(deps, "s3job", "")
	if err != nil {
		t.Fatal(err)
	}
	restoreDir := filepath.Join(root, "restore")
	if err := r.Restore(ctx, restoreOpts("latest", restoreDir)); err != nil {
		t.Fatalf("real S3 restore failed: %v", err)
	}
	restored := restoredPathFor(restoreDir, sourceDir)
	assertFileContent(t, restored, "file1.txt", "s3 real-provider test content MODIFIED")
}

// TestRealProvider_SFTP exercises the sftp provider (restic's native sftp
// backend) against a real SFTP server, including the non-default-port /
// key-file "-o sftp.command" override path (internal/backend.buildSFTP) --
// the exact construction a real bug was found and fixed in during this
// project's development (see docs/TESTING.md): restic's plain
// "sftp:user@host:/path" syntax has no way to express a non-default port.
//
// Set ABM_TEST_SFTP_HOST, ABM_TEST_SFTP_PORT, ABM_TEST_SFTP_USER, and
// ABM_TEST_SFTP_KEY_FILE to run it. CI starts atmoz/sftp with a freshly
// generated disposable key pair for this.
func TestRealProvider_SFTP(t *testing.T) {
	host := os.Getenv("ABM_TEST_SFTP_HOST")
	if host == "" {
		t.Skip("ABM_TEST_SFTP_HOST not set; skipping real SFTP protocol test")
	}
	port := os.Getenv("ABM_TEST_SFTP_PORT")
	user := os.Getenv("ABM_TEST_SFTP_USER")
	keyFile := os.Getenv("ABM_TEST_SFTP_KEY_FILE")
	requireRestic(t)

	root := testRoot(t)
	sourceDir := filepath.Join(root, "source")
	writeFile(t, filepath.Join(sourceDir, "file1.txt"), "sftp real-provider test content")

	cfg := &config.Config{
		Version: config.CurrentSchemaVersion,
		Global:  config.Global{DeviceID: "dev-test", Organization: "testorg"},
		Storage: []config.Storage{{
			Name: "sftptest", Provider: "sftp",
			Options: map[string]string{"host": host, "port": port, "username": user, "key_file": keyFile, "path_prefix": "upload"},
		}},
		Jobs: map[string]config.Job{
			"sftpjob": {
				Sources:        []string{sourceDir},
				Destinations:   []string{"sftptest"},
				RepositoryPath: "testorg/dev-test/sftpjob",
				Retention:      &config.Retention{KeepWithinHourly: "240h"},
				Enabled:        true,
			},
		},
	}

	store := newMemStore(t)
	store.Set(job.ResticPasswordKey("sftpjob", "sftptest"), "test-password-not-a-real-secret")

	deps := &job.Deps{
		Config:   cfg,
		StateDir: filepath.Join(root, "state"),
		LockDir:  filepath.Join(root, "locks"),
		DumpDir:  filepath.Join(root, "dumps"),
		Secrets:  store,
		DBCreds:  store,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	ctx := context.Background()

	st, err := job.Run(ctx, deps, "sftpjob")
	if err != nil {
		t.Fatalf("real SFTP backup failed: %v", err)
	}
	if st.Primary().LastSnapshotID == "" {
		t.Fatal("no snapshot id after real SFTP backup")
	}

	r, _, err := job.ResticRunner(deps, "sftpjob", "")
	if err != nil {
		t.Fatal(err)
	}
	restoreDir := filepath.Join(root, "restore")
	if err := r.Restore(ctx, restoreOpts("latest", restoreDir)); err != nil {
		t.Fatalf("real SFTP restore failed: %v", err)
	}
	restored := restoredPathFor(restoreDir, sourceDir)
	assertFileContent(t, restored, "file1.txt", "sftp real-provider test content")
}
