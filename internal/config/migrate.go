package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ---- v1 document shape (kept only so Migrate can read it) ----
//
// v1's Storage used a fixed type/rclone_remote enum; v1's Job used a single
// Destination string. Once those field names changed in v2, unmarshaling an
// old document straight into the current Config struct would silently drop
// the old fields (YAML ignores keys with no matching field) before Migrate
// ever saw them -- so a v1 document must be parsed with its *own* shape
// first, then converted, rather than "repaired" after the fact.

type configV1 struct {
	Version int              `yaml:"version"`
	Global  Global           `yaml:"global"`
	Storage []storageV1      `yaml:"storage"`
	Jobs    map[string]jobV1 `yaml:"jobs"`
}

type storageV1 struct {
	Name         string            `yaml:"name"`
	Type         string            `yaml:"type"`
	RcloneRemote string            `yaml:"rclone_remote"`
	Options      map[string]string `yaml:"options,omitempty"`
	Immutable    bool              `yaml:"immutable,omitempty"`
}

type jobV1 struct {
	Sources        []string   `yaml:"sources"`
	Excludes       []string   `yaml:"excludes,omitempty"`
	MaxFileSizeMB  int64      `yaml:"max_file_size_mb,omitempty"`
	Destination    string     `yaml:"destination"`
	RepositoryPath string     `yaml:"repository_path"`
	Schedule       string     `yaml:"schedule,omitempty"`
	Retention      *Retention `yaml:"retention,omitempty"`
	Databases      []Database `yaml:"databases,omitempty"`
	Tags           []string   `yaml:"tags,omitempty"`
	Enabled        bool       `yaml:"enabled"`
}

// ParseLegacyV1 parses data as a v1 document and converts it to the current
// (v2) Config shape. Exported so tests outside this package can exercise the
// exact migration a real on-disk v1 config would go through.
func ParseLegacyV1(data []byte) (*Config, error) {
	var v1 configV1
	if err := yaml.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("parsing v1 config: %w", err)
	}
	return migrateV1ToV2(v1), nil
}

// migrateV1ToV2 converts a parsed v1 document to v2.
//
// For Google Drive/OneDrive/Dropbox, the old rclone_remote carries forward
// directly as the new "remote" option -- nothing about how those are reached
// changed, so existing repositories stay reachable with no other action.
//
// For S3 and SFTP, v2 moved from a rclone-mediated repository to restic's
// native backend for each (see docs/ARCHITECTURE.md). Their credentials
// lived only inside rclone.conf before, opaque to this config file, so they
// cannot be carried forward automatically. Migration preserves the storage
// entry's name, maps it to the new provider ID, and carries over any
// non-secret options it already knows (e.g. S3's endpoint), but the admin
// must re-run `abm storage add` with the same --name to supply credentials
// in the new scheme before that destination works again -- `abm doctor`
// reports exactly this missing-field case. The existing remote repository's
// actual data and path are untouched by migration; re-adding the storage
// with the same repository_path reconnects to the same bytes.
func migrateV1ToV2(v1 configV1) *Config {
	cfg := &Config{
		Version: CurrentSchemaVersion,
		Global:  v1.Global,
		Jobs:    map[string]Job{},
	}

	for _, s := range v1.Storage {
		providerID := legacyProviderMapping(s.Type)
		options := map[string]string{}
		for k, v := range s.Options {
			if k == "bucket_endpoint" {
				options["endpoint"] = v
				continue
			}
			options[k] = v
		}
		if s.RcloneRemote != "" {
			switch providerID {
			case "google-drive", "onedrive", "dropbox":
				options["remote"] = s.RcloneRemote
			default:
				// s3/sftp: the old rclone remote is no longer used by the
				// new native backend; keep it only for operator reference.
				options["_migrated_v1_rclone_remote"] = s.RcloneRemote
			}
		}
		cfg.Storage = append(cfg.Storage, Storage{
			Name:      s.Name,
			Provider:  providerID,
			Options:   options,
			Immutable: s.Immutable,
		})
	}

	for name, j := range v1.Jobs {
		dest := []string{}
		if j.Destination != "" {
			dest = []string{j.Destination}
		}
		cfg.Jobs[name] = Job{
			Sources:        j.Sources,
			Excludes:       j.Excludes,
			MaxFileSizeMB:  j.MaxFileSizeMB,
			Destinations:   dest,
			RepositoryPath: j.RepositoryPath,
			Schedule:       j.Schedule,
			Retention:      j.Retention,
			Databases:      j.Databases,
			Tags:           j.Tags,
			Enabled:        j.Enabled,
		}
	}

	return cfg
}

func legacyProviderMapping(legacyType string) string {
	switch legacyType {
	case "gdrive":
		return "google-drive"
	case "onedrive":
		return "onedrive"
	case "dropbox":
		return "dropbox"
	case "s3":
		return "generic-s3"
	case "sftp":
		return "sftp"
	case "local":
		return "local"
	default:
		return legacyType
	}
}

// Migrate upgrades cfg in place to CurrentSchemaVersion. It only handles
// version bookkeeping and future (structurally-compatible) migrations here;
// the v1 -> v2 field-shape change is handled separately by ParseLegacyV1,
// which Parse calls instead of a plain yaml.Unmarshal when it detects a v1
// document, since that conversion cannot be done after the fact once v1's
// field names have already been lost to a v2-shaped unmarshal.
func Migrate(cfg *Config) error {
	if cfg.Version == 0 {
		cfg.Version = CurrentSchemaVersion
	}
	if cfg.Version > CurrentSchemaVersion {
		return fmt.Errorf("unsupported future schema version %d", cfg.Version)
	}
	// Future migrations append here, e.g.:
	// if cfg.Version == 2 { migrateV2ToV3(cfg); cfg.Version = 3 }
	return nil
}
