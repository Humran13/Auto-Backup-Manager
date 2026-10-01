package config

import "testing"

func validBaseConfig() Config {
	return Config{
		Version: CurrentSchemaVersion,
		Global:  Global{DeviceID: "dev-test"},
		Storage: []Storage{{Name: "local-dest", Provider: "local", Options: map[string]string{"path": "/mnt/backup"}}},
		Jobs: map[string]Job{
			"job1": {
				Sources:        []string{"/data"},
				Destinations:   []string{"local-dest"},
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
	job.Destinations = []string{"does-not-exist"}
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a job pointing at an undefined destination")
	}
}

func TestValidate_NoDestinationsRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Destinations = nil
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a job with no destinations")
	}
}

func TestValidate_DuplicateDestinationRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.Destinations = []string{"local-dest", "local-dest"}
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a duplicate destination")
	}
}

func TestValidate_MultipleDestinationsAllowed(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Storage = append(cfg.Storage, Storage{Name: "local-dest-2", Provider: "local", Options: map[string]string{"path": "/mnt/backup2"}})
	job := cfg.Jobs["job1"]
	job.Destinations = []string{"local-dest", "local-dest-2"}
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected a job with two valid destinations to pass, got: %v", err)
	}
}

func TestValidate_UnknownDestinationPolicyModeRejected(t *testing.T) {
	cfg := validBaseConfig()
	job := cfg.Jobs["job1"]
	job.DestinationPolicy.Mode = "sometimes"
	cfg.Jobs["job1"] = job
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for an unknown destination_policy mode")
	}
}

func TestValidate_UnknownProviderRejected(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Storage[0].Provider = "does-not-exist"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for an unknown provider")
	}
}

func TestValidate_MissingRequiredProviderFieldRejected(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Storage[0].Options = map[string]string{} // local requires "path"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error for a provider missing a required non-secret field")
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

func TestEffectivePolicy_DefaultsToPrimaryRequired(t *testing.T) {
	j := Job{Destinations: []string{"a", "b"}}
	if j.EffectivePolicy() != PolicyPrimaryRequired {
		t.Fatalf("expected default policy primary-required, got %q", j.EffectivePolicy())
	}
}

func TestPrimary_ReturnsFirstDestination(t *testing.T) {
	j := Job{Destinations: []string{"a", "b"}}
	if j.Primary() != "a" {
		t.Fatalf("expected primary 'a', got %q", j.Primary())
	}
	if (Job{}).Primary() != "" {
		t.Fatal("expected empty primary for a job with no destinations")
	}
}

func TestParse_V2DocumentRoundTrips(t *testing.T) {
	yamlDoc := []byte(`
version: 2
global:
  device_id: dev-test
storage:
  - name: local
    provider: local
    options:
      path: /mnt/backup
jobs:
  job1:
    sources: ["/data"]
    destinations: ["local"]
    repository_path: p
    enabled: true
    retention:
      keep_within_hourly: 240h
`)
	cfg, err := Parse(yamlDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 2 {
		t.Fatalf("expected version 2, got %d", cfg.Version)
	}
	if cfg.Storage[0].Provider != "local" {
		t.Fatalf("expected provider 'local', got %q", cfg.Storage[0].Provider)
	}
}

func TestMigrate_RejectsFutureVersion(t *testing.T) {
	cfg := &Config{Version: CurrentSchemaVersion + 5}
	if err := Migrate(cfg); err == nil {
		t.Fatal("expected error migrating a config from a newer, unknown schema version")
	}
}
