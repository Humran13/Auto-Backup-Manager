package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up from the test's working directory (internal/provider)
// to the repository root, so doc-existence checks work regardless of how
// `go test` is invoked.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "..", "..")
}

// TestRegistry_EveryProviderIsFullySpecified prevents a provider from being
// added half-finished: every catalog entry (supported or not) must have the
// metadata the setup wizard, docs-consistency, and secret-redaction coverage
// all depend on.
func TestRegistry_EveryProviderIsFullySpecified(t *testing.T) {
	root := repoRoot(t)
	seen := map[string]bool{}

	for _, p := range Registry {
		t.Run(p.ID, func(t *testing.T) {
			if p.ID == "" {
				t.Fatal("provider has no ID")
			}
			if seen[p.ID] {
				t.Fatalf("duplicate provider ID %q", p.ID)
			}
			seen[p.ID] = true

			if p.DisplayName == "" {
				t.Error("missing DisplayName")
			}
			if p.DocPath == "" {
				t.Error("missing DocPath")
			} else if _, err := os.Stat(filepath.Join(root, p.DocPath)); err != nil {
				t.Errorf("DocPath %q does not exist: %v", p.DocPath, err)
			}
			if p.Maturity == "" {
				t.Error("missing Maturity")
			}

			if p.Unsupported {
				if p.UnsupportedReason == "" {
					t.Error("unsupported provider must explain UnsupportedReason")
				}
				if p.Maturity != MaturityUnavailable {
					t.Error("unsupported provider must have Maturity = unavailable")
				}
				return // the fields below only apply to offered providers
			}

			if p.Family == "" {
				t.Error("missing Family")
			}
			if p.Backend == "" {
				t.Error("missing Backend")
			}
			if p.Backend == BackendRclone && p.RcloneBackend == "" && p.Auth != AuthExisting {
				// AuthExisting (the generic "use an existing rclone remote"
				// escape hatch) deliberately has no fixed backend -- that's
				// the point of it.
				t.Error("rclone-backed provider missing RcloneBackend name")
			}
			if p.Auth == "" {
				t.Error("missing Auth method")
			}
			if p.Headless == "" {
				t.Error("missing Headless support classification")
			}
			if (p.Maturity == MaturityExperimental) != p.Experimental {
				t.Error("Experimental flag must agree with Maturity == experimental")
			}
			if len(p.RequiredFields) == 0 && p.Auth != AuthExisting {
				// AuthExisting (generic rclone) still has one required field
				// (the remote name); every other auth method needs at least
				// one required credential field or the setup flow has
				// nothing to collect.
				t.Error("no RequiredFields: setup wizard would have nothing to ask for")
			}
			for _, f := range append(append([]CredentialField{}, p.RequiredFields...), p.OptionalFields...) {
				if f.Key == "" {
					t.Error("credential field with empty Key")
				}
				if f.Label == "" {
					t.Errorf("credential field %q has no Label", f.Key)
				}
			}
		})
	}
}

// TestRegistry_SecretFieldsAreMarkedSecret is a narrow but important check:
// anything that looks like a password/key/secret by name must be marked
// Secret so it goes through the secret store, never config.yaml.
func TestRegistry_SecretFieldsAreMarkedSecret(t *testing.T) {
	suspicious := map[string]bool{"password": true, "secret_key": true, "client_secret": true, "account_key": true}
	for _, p := range Registry {
		for _, f := range append(append([]CredentialField{}, p.RequiredFields...), p.OptionalFields...) {
			if suspicious[f.Key] && !f.Secret {
				t.Errorf("provider %q field %q looks like a secret but Secret=false", p.ID, f.Key)
			}
		}
	}
}

func TestGet_FindsKnownProvider(t *testing.T) {
	p, ok := Get("google-drive")
	if !ok || p.DisplayName != "Google Drive" {
		t.Fatalf("Get(\"google-drive\") = %+v, %v", p, ok)
	}
}

func TestGet_UnknownReturnsFalse(t *testing.T) {
	if _, ok := Get("does-not-exist"); ok {
		t.Fatal("expected ok=false for an unknown provider ID")
	}
}

func TestByFamily_ExcludesUnsupported(t *testing.T) {
	for _, p := range ByFamily(FamilyCloudDrive) {
		if p.Unsupported {
			t.Errorf("ByFamily returned unsupported provider %q", p.ID)
		}
	}
}

func TestUnsupported_IncludesSyncCom(t *testing.T) {
	found := false
	for _, p := range Unsupported() {
		if p.ID == "sync-com" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected sync-com in Unsupported()")
	}
}

// TestRegistry_CoversRequiredCatalog asserts the specific providers this
// phase of the project was asked to catalog are all present, by ID.
func TestRegistry_CoversRequiredCatalog(t *testing.T) {
	want := []string{
		"google-drive", "onedrive", "dropbox", "box", "pcloud", "mega", "mega-s4",
		"jottacloud", "icloud-drive", "proton-drive", "sync-com", "idrive-e2",
		"aws-s3", "backblaze-b2", "wasabi", "cloudflare-r2", "hetzner-object-storage",
		"digitalocean-spaces", "azure-blob", "google-cloud-storage", "storj", "minio",
		"sftp", "local", "rclone-existing", "generic-s3",
	}
	for _, id := range want {
		if _, ok := Get(id); !ok {
			t.Errorf("expected provider %q in registry", id)
		}
	}
}
