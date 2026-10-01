package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/backend"
	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
	"github.com/Humran13/Auto-Backup-Manager/internal/rclone"
)

func rcloneRunner() *rclone.Runner {
	return &rclone.Runner{ConfigPath: paths.RcloneConfigFile()}
}

func newStorageCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Manage backup storage destinations",
	}
	cmd.AddCommand(newStorageProvidersCmd(), newStorageListCmd(a), newStorageShowCmd(a),
		newStorageTestCmd(a), newStorageAddCmd(a), newStorageReconnectCmd(a), newStorageRemoveCmd(a))
	return cmd
}

func newStorageProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List every storage provider ABM knows about, with its maturity",
		RunE: func(cmd *cobra.Command, args []string) error {
			rows := append([]provider.Provider{}, provider.Registry...)
			sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
			fmt.Printf("%-24s %-16s %-14s %s\n", "ID", "FAMILY", "MATURITY", "NOTE")
			for _, p := range rows {
				note := ""
				if p.Unsupported {
					note = p.UnsupportedReason
					if len(note) > 60 {
						note = note[:57] + "..."
					}
				} else if p.Experimental {
					note = "see " + p.DocPath
				}
				fmt.Printf("%-24s %-16s %-14s %s\n", p.ID, p.Family, p.Maturity, note)
			}
			return nil
		},
	}
}

func newStorageListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured storage destinations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			for _, s := range cfg.Storage {
				p, _ := provider.Get(s.Provider)
				immutable := ""
				if s.Immutable {
					immutable = " (immutable)"
				}
				fmt.Printf("%-20s provider=%-18s backend=%-8s maturity=%-12s%s\n", s.Name, s.Provider, p.Backend, p.Maturity, immutable)
			}
			return nil
		},
	}
}

func newStorageShowCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show details for one configured storage destination",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			s := findStorageByName(cfg, args[0])
			if s == nil {
				return fmt.Errorf("no such storage %q", args[0])
			}
			p, ok := provider.Get(s.Provider)
			if !ok {
				return fmt.Errorf("storage %q references unknown provider %q", s.Name, s.Provider)
			}
			fmt.Printf("name:        %s\n", s.Name)
			fmt.Printf("provider:    %s (%s)\n", p.ID, p.DisplayName)
			fmt.Printf("family:      %s\n", p.Family)
			fmt.Printf("backend:     %s\n", p.Backend)
			fmt.Printf("maturity:    %s\n", p.Maturity)
			fmt.Printf("headless:    %s\n", p.Headless)
			fmt.Printf("immutable:   %v\n", s.Immutable)
			fmt.Printf("doc:         %s\n", p.DocPath)
			for _, opt := range sortedKeys(s.Options) {
				fmt.Printf("option %s=%s\n", opt, s.Options[opt])
			}
			for _, lim := range p.KnownLimitations {
				fmt.Printf("limitation:  %s\n", lim)
			}
			return nil
		},
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func findStorageByName(cfg *config.Config, name string) *config.Storage {
	for i := range cfg.Storage {
		if cfg.Storage[i].Name == name {
			return &cfg.Storage[i]
		}
	}
	return nil
}

func newStorageTestCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Run the full init/backup/restore capability test against a configured storage destination",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			s := findStorageByName(cfg, args[0])
			if s == nil {
				return fmt.Errorf("no such storage %q", args[0])
			}
			if err := backend.Probe(context.Background(), "restic", *s, a.secrets, paths.RcloneConfigFile()); err != nil {
				return fmt.Errorf("%s: capability test FAILED: %w", s.Name, err)
			}
			fmt.Printf("%s: OK (init/backup/restore round-trip verified)\n", s.Name)
			return nil
		},
	}
}

func newStorageRemoveCmd(a *app) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a storage destination from the configuration (does not delete any remote data)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			if findStorageByName(cfg, args[0]) == nil {
				return fmt.Errorf("no such storage %q", args[0])
			}
			if !yes {
				return fmt.Errorf("pass --yes to confirm removal of storage %q from local config (no remote data is ever deleted)", args[0])
			}
			var kept []config.Storage
			for _, s := range cfg.Storage {
				if s.Name != args[0] {
					kept = append(kept, s)
				}
			}
			cfg.Storage = kept
			return config.Save(paths.ConfigFile(), cfg)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal")
	return cmd
}

// newStorageReconnectCmd re-runs the OAuth flow for an rclone-backed remote
// whose token has expired or been revoked -- the normal, expected path for
// providers like iCloud Drive/Proton Drive/Jottacloud where periodic
// reauthentication is part of how the provider works, not a failure.
func newStorageReconnectCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "reconnect <name>",
		Short: "Re-authenticate an rclone-backed storage destination (OAuth token expired/revoked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			s := findStorageByName(cfg, args[0])
			if s == nil {
				return fmt.Errorf("no such storage %q", args[0])
			}
			p, ok := provider.Get(s.Provider)
			if !ok {
				return fmt.Errorf("unknown provider %q", s.Provider)
			}
			if p.Backend != provider.BackendRclone {
				return fmt.Errorf("storage %q (provider %q) is not rclone-backed; its credentials don't expire the way OAuth tokens do", s.Name, s.Provider)
			}
			remote := s.Options["remote"]
			if remote == "" {
				return fmt.Errorf("storage %q has no rclone remote configured", s.Name)
			}
			fmt.Printf("Reconnecting rclone remote %q (provider %s)...\n", remote, p.DisplayName)
			if err := rcloneRunner().Reconnect(context.Background(), remote); err != nil {
				return fmt.Errorf("reconnect failed: %w", err)
			}
			fmt.Println("reconnected. Run 'abm storage test", s.Name, "' to confirm.")
			return nil
		},
	}
}

// namedFieldFlags maps convenient CLI flag names to the CredentialField.Key
// values the provider registry uses, so common fields (bucket, endpoint,
// access keys, ...) get ergonomic flags while anything a preset doesn't
// special-case is still reachable via the generic --set key=value escape
// hatch below.
var namedFieldFlags = map[string]string{
	"remote":           "remote",
	"endpoint":         "endpoint",
	"region":           "region",
	"bucket":           "bucket",
	"access-key":       "access_key",
	"secret-key":       "secret_key",
	"host":             "host",
	"port":             "port",
	"username":         "username",
	"password":         "password",
	"key-file":         "key_file",
	"path":             "path",
	"path-prefix":      "path_prefix",
	"account-name":     "account_name",
	"account-key":      "account_key",
	"container":        "container",
	"project-id":       "project_id",
	"credentials-file": "credentials_file",
	"auth-url":         "auth_url",
	"tenant":           "tenant",
}

func newStorageAddCmd(a *app) *cobra.Command {
	var (
		providerID, name string
		immutable        bool
		skipProbe        bool
		set              []string
	)
	values := map[string]*string{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a storage destination",
		Long: `Add a storage destination.

Run 'abm storage providers' to see every supported provider ID and its
maturity. Each provider declares which fields it needs (see 'abm storage
show' after adding, or the provider's doc under docs/providers/); supply them
with the named flags below, or with repeatable --set key=value for any field
a named flag doesn't cover. Secret fields (passwords, access/secret keys)
that you don't pass on the command line are prompted for on stdin instead --
never required as a command-line argument, so they never end up in shell
history or a process listing.

For OAuth cloud-drive providers (Google Drive, OneDrive, Dropbox, Box,
pCloud, MEGA, Jottacloud, iCloud Drive, Proton Drive), first create the
remote yourself with 'rclone config' (see that provider's doc for the
browser/headless flow), then register it here with --set remote=<name>.

Before saving, ABM runs a full init/backup/restore capability test against
the destination (internal/backend.Probe) and refuses to add it if that test
fails -- pass --skip-probe to register anyway (e.g. to fix connectivity
after the fact).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if providerID == "" {
				return fmt.Errorf("--provider is required (see 'abm storage providers')")
			}
			p, ok := provider.Get(providerID)
			if !ok {
				return fmt.Errorf("unknown provider %q (see 'abm storage providers')", providerID)
			}
			if p.Unsupported {
				return fmt.Errorf("provider %q is not supported: %s", providerID, p.UnsupportedReason)
			}

			collected := map[string]string{}
			for flagName, key := range namedFieldFlags {
				if v := values[flagName]; v != nil && *v != "" {
					collected[key] = *v
				}
			}
			for _, kv := range set {
				k, v, found := strings.Cut(kv, "=")
				if !found {
					return fmt.Errorf("--set value %q must be key=value", kv)
				}
				collected[k] = v
			}

			reader := bufio.NewReader(os.Stdin)
			allFields := append(append([]provider.CredentialField{}, p.RequiredFields...), p.OptionalFields...)
			secretVals := map[string]string{}
			options := map[string]string{}
			for _, f := range allFields {
				v, have := collected[f.Key]
				if !have && f.Secret && f.Required {
					fmt.Printf("Enter %s: ", f.Label)
					line, _ := reader.ReadString('\n')
					v = strings.TrimSpace(line)
				}
				if v == "" {
					v = f.Default
				}
				if v == "" {
					if f.Required {
						return fmt.Errorf("missing required field %q (%s)", f.Key, f.Label)
					}
					continue
				}
				if f.Secret {
					secretVals[f.Key] = v
				} else {
					options[f.Key] = v
				}
			}

			storage := config.Storage{Name: name, Provider: providerID, Options: options, Immutable: immutable}

			if p.Experimental {
				fmt.Printf("NOTE: %q is an EXPERIMENTAL provider. See %s for its limitations before relying on it.\n", providerID, p.DocPath)
			}

			if !skipProbe {
				fmt.Println("running capability test (init/backup/restore round-trip)...")
				probeStore := &inMemoryOverlayStore{base: a.secrets, overlay: map[string]string{}}
				for k, v := range secretVals {
					probeStore.overlay[backend.SecretKey(name, k)] = v
				}
				if err := backend.Probe(context.Background(), "restic", storage, probeStore, paths.RcloneConfigFile()); err != nil {
					return fmt.Errorf("capability test failed, storage NOT added (use --skip-probe to override): %w", err)
				}
				fmt.Println("capability test passed.")
			}

			for k, v := range secretVals {
				if err := a.secrets.Set(backend.SecretKey(name, k), v); err != nil {
					return fmt.Errorf("storing credential %q: %w", k, err)
				}
			}

			cfg := a.cfg
			if cfg == nil {
				return fmt.Errorf("no configuration found. Run 'abm setup' first")
			}
			cfg.Storage = append(cfg.Storage, storage)
			if err := config.Save(paths.ConfigFile(), cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Printf("storage %q added (provider=%s)\n", name, providerID)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerID, "provider", "", "provider ID (see 'abm storage providers')")
	cmd.Flags().StringVar(&name, "name", "", "storage destination name")
	cmd.Flags().BoolVar(&immutable, "immutable", false, "mark this destination as Object-Lock/WORM capable (advisory; see docs/IMMUTABILITY.md)")
	cmd.Flags().BoolVar(&skipProbe, "skip-probe", false, "skip the init/backup/restore capability test")
	cmd.Flags().StringArrayVar(&set, "set", nil, "set an arbitrary provider field: --set key=value (repeatable)")
	for flagName := range namedFieldFlags {
		v := new(string)
		values[flagName] = v
		secret := flagName == "secret-key" || flagName == "password" || flagName == "account-key"
		help := flagName
		if secret {
			help += " (prompted on stdin if omitted, never required as a flag)"
		}
		cmd.Flags().StringVar(v, flagName, "", help)
	}
	return cmd
}

// inMemoryOverlayStore lets the capability probe see secrets that haven't
// been persisted to the real store yet (storage add runs the probe before
// committing), without touching the real store until the probe passes.
type inMemoryOverlayStore struct {
	base interface {
		Get(string) (string, error)
		Set(string, string) error
		Path(string) (string, error)
	}
	overlay map[string]string
}

func (s *inMemoryOverlayStore) Get(key string) (string, error) {
	if v, ok := s.overlay[key]; ok {
		return v, nil
	}
	return s.base.Get(key)
}
func (s *inMemoryOverlayStore) Set(key, value string) error { s.overlay[key] = value; return nil }
func (s *inMemoryOverlayStore) Path(key string) (string, error) {
	v, err := s.Get(key)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "abm-overlay-secret-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(v); err != nil {
		return "", err
	}
	return f.Name(), nil
}
