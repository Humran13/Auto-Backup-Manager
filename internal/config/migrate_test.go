package config

import "testing"

// TestMigrateV1ToV2_CloudDriveCarriesRemoteForward checks the "no action
// needed" case: Google Drive/OneDrive/Dropbox didn't change transport, so
// their old rclone_remote must survive as the new "remote" option and the
// job must keep pointing at the exact same repository_path -- an existing
// repository must stay reachable with zero admin action.
func TestMigrateV1ToV2_CloudDriveCarriesRemoteForward(t *testing.T) {
	v1 := []byte(`
version: 1
global:
  device_id: dev-abc
  organization: acme
storage:
  - name: gdrive
    type: gdrive
    rclone_remote: my-gdrive-remote
jobs:
  website:
    sources: ["/var/www"]
    destination: gdrive
    repository_path: acme/dev-abc/website
    enabled: true
    retention:
      keep_within_hourly: 240h
`)
	cfg, err := ParseLegacyV1(v1)
	if err != nil {
		t.Fatalf("ParseLegacyV1: %v", err)
	}
	if cfg.Version != CurrentSchemaVersion {
		t.Fatalf("expected migrated version %d, got %d", CurrentSchemaVersion, cfg.Version)
	}
	if len(cfg.Storage) != 1 {
		t.Fatalf("expected 1 storage entry, got %d", len(cfg.Storage))
	}
	s := cfg.Storage[0]
	if s.Provider != "google-drive" {
		t.Fatalf("expected provider 'google-drive', got %q", s.Provider)
	}
	if s.Options["remote"] != "my-gdrive-remote" {
		t.Fatalf("expected the old rclone_remote to carry forward as options.remote, got %q", s.Options["remote"])
	}

	job, ok := cfg.Jobs["website"]
	if !ok {
		t.Fatal("expected job 'website' to survive migration")
	}
	if len(job.Destinations) != 1 || job.Destinations[0] != "gdrive" {
		t.Fatalf("expected destinations=[gdrive], got %v", job.Destinations)
	}
	if job.RepositoryPath != "acme/dev-abc/website" {
		t.Fatalf("repository_path must be unchanged by migration (same repository bytes), got %q", job.RepositoryPath)
	}

	// The migrated config must actually validate -- a cloud-drive migration
	// needs no further admin action.
	if err := Validate(cfg); err != nil {
		t.Fatalf("migrated cloud-drive config should validate with no further action, got: %v", err)
	}
}

// TestMigrateV1ToV2_S3MapsProviderAndPreservesEndpoint checks the "admin
// action needed" case: S3 moved from rclone-mediated to restic's native
// backend, so the old rclone_remote's credentials (held only in rclone.conf)
// cannot be carried forward. The provider mapping and repository_path must
// still be preserved (so re-adding the storage reconnects to the same
// bytes); this test verifies the non-secret 'endpoint' carries forward and
// the provider mapping is correct. internal/backend.Build is what actually
// refuses to run without access_key/secret_key (see internal/backend's own
// tests) -- Validate here only checks non-secret fields.
func TestMigrateV1ToV2_S3MapsProviderAndPreservesEndpoint(t *testing.T) {
	v1 := []byte(`
version: 1
global:
  device_id: dev-abc
  organization: acme
storage:
  - name: b2
    type: s3
    rclone_remote: b2-old-remote
    options:
      bucket_endpoint: s3.us-west-002.backblazeb2.com
jobs:
  website:
    sources: ["/var/www"]
    destination: b2
    repository_path: acme/dev-abc/website
    enabled: true
    retention:
      keep_within_hourly: 240h
`)
	cfg, err := ParseLegacyV1(v1)
	if err != nil {
		t.Fatalf("ParseLegacyV1: %v", err)
	}
	s := cfg.Storage[0]
	if s.Provider != "generic-s3" {
		t.Fatalf("expected provider 'generic-s3', got %q", s.Provider)
	}
	if s.Options["endpoint"] != "s3.us-west-002.backblazeb2.com" {
		t.Fatalf("expected bucket_endpoint to be renamed to endpoint, got options=%v", s.Options)
	}
	if cfg.Jobs["website"].RepositoryPath != "acme/dev-abc/website" {
		t.Fatal("repository_path must be unchanged by migration")
	}
	// access_key/secret_key were never in config.yaml (they lived in
	// rclone.conf) so they are legitimately absent after migration --
	// Validate doesn't check secret fields, only internal/backend.Build does
	// at run time, which is what actually prompts the admin to reconfigure.
}

func TestMigrateV1ToV2_LocalAndSFTPMapCorrectly(t *testing.T) {
	v1 := []byte(`
version: 1
global:
  device_id: dev-abc
storage:
  - name: localdisk
    type: local
    options:
      path: /mnt/backup
  - name: box
    type: sftp
    rclone_remote: box-old-remote
jobs:
  j:
    sources: ["/data"]
    destination: localdisk
    repository_path: p
    enabled: true
    retention:
      keep_within_hourly: 240h
`)
	cfg, err := ParseLegacyV1(v1)
	if err != nil {
		t.Fatalf("ParseLegacyV1: %v", err)
	}
	byName := map[string]Storage{}
	for _, s := range cfg.Storage {
		byName[s.Name] = s
	}
	if byName["localdisk"].Provider != "local" || byName["localdisk"].Options["path"] != "/mnt/backup" {
		t.Fatalf("local migration incorrect: %+v", byName["localdisk"])
	}
	if byName["box"].Provider != "sftp" {
		t.Fatalf("expected sftp provider mapping, got %q", byName["box"].Provider)
	}
}
