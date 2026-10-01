package config

import "testing"

func validBaseConfig() Config {
	return Config{
		Version: CurrentSchemaVersion,
		Global:  Global{DeviceID: "dev-test"},
		Storage: []Storage{{Name: "s3-dest", Type: StorageS3, RcloneRemote: "s3-dest"}},
		Jobs: map[string]Job{
			"job1": {
				Sources:        []string{"/data"},
				Destination:    "s3-dest",
				RepositoryPath: "org/dev-test/job1",
				Retention:      &Retention{KeepWithinHourly: "240h"},
				Enabled:        true,
			},
		},
	}
}

func TestValidate_ValidConfigPasses(t *testing.T) {
	cfg := validBaseConfig()
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected valid config to pass, got: %v", err)
	}
}

func TestValidate_ZeroJobsIsAllowed(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Jobs = map[string]Job{}
	if err := Validate(&cfg); err != nil {
		t.Fatalf("a freshly bootstrapped config with no jobs yet must be valid, got: %v", err)
	}
}

func TestValidate_MissingDeviceIDRejected(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Global.DeviceID = ""
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for missing device id")
	}
}

func TestValidate_UnknownDestinationRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Destination = "does-not-exist"
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a job pointing at an undefined destination")
	}
}

func TestValidate_EmptySourcesRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Sources = nil
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a job with no sources")
	}
}

func TestValidate_DuplicateSourcesRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Sources = []string{"/data", "/data"}
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for duplicate source paths")
	}
}

func TestValidate_InvalidJobNameRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	delete(cfg.Jobs, "job1")
	cfg.Jobs["../escape"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for an unsafe job name")
	}
}

func TestValidate_FutureSchemaVersionRejected(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Version = CurrentSchemaVersion + 1
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error when config version is newer than this build supports")
	}
}

func TestValidate_MySQLWithoutCredentialsRefRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Databases = []Database{{Kind: DatabaseMySQL, Name: "app"}}
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a MySQL database without credentials_ref")
	}
}

func TestValidate_SQLiteWithoutPathRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Databases = []Database{{Kind: DatabaseSQLite, Name: "app"}}
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a SQLite database without a path")
	}
}

func TestParse_MissingVersionDefaultsToOne(t *testing.T) {
	yamlDoc := []byte(`
global:
  device_id: dev-test
storage:
  - name: local
    type: local
jobs:
  job1:
    sources: ["/data"]
    destination: local
    repository_path: p
    enabled: true
    retention:
      keep_within_hourly: 240h
`)
	cfg, err := Parse(yamlDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("expected version to default to 1, got %d", cfg.Version)
	}
}

func TestMigrate_RejectsFutureVersion(t *testing.T) {
	cfg := &Config{Version: CurrentSchemaVersion + 5}
	if err := Migrate(cfg); err == nil {
		t.Fatal("expected error migrating a config from a newer, unknown schema version")
	}
}
