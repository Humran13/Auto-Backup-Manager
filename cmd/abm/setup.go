package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/deviceid"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
)

// newSetupCmd implements the first-run bootstrap: device identity, standard
// directories, and an initial config.yaml. It deliberately stops short of
// also running the storage/job wizard inline -- those are `abm storage add`
// and `abm job add`, which setup prints as the next steps -- so each phase
// stays independently scriptable for automated/CI setup as well as
// interactive use.
func newSetupCmd(a *app) *cobra.Command {
	var deviceName, organization string
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "First-run setup: device identity and initial configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Auto-Backup-Manager setup (%s)\n\n", runtime.GOOS)

			reader := bufio.NewReader(os.Stdin)
			if deviceName == "" && !nonInteractive {
				deviceName = prompt(reader, "Name this device", defaultDeviceName())
			}
			if deviceName == "" {
				deviceName = defaultDeviceName()
			}
			if organization == "" && !nonInteractive {
				organization = prompt(reader, "Organization (used to scope repository paths)", "default-org")
			}
			if organization == "" {
				organization = "default-org"
			}

			for _, dir := range []string{paths.ConfigDir, paths.StateDir, paths.LockDir, paths.DumpDir, paths.LogDir, paths.SecretsDir} {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					return fmt.Errorf("creating %s: %w", dir, err)
				}
			}

			id, err := deviceid.LoadOrCreate(paths.DeviceIDFile())
			if err != nil {
				return fmt.Errorf("device id: %w", err)
			}

			cfg := a.cfg
			if cfg == nil {
				cfg = &config.Config{
					Version: config.CurrentSchemaVersion,
					Jobs:    map[string]config.Job{},
				}
			}
			cfg.Global.DeviceName = deviceName
			cfg.Global.DeviceID = id
			cfg.Global.Organization = organization
			if cfg.Global.LogLevel == "" {
				cfg.Global.LogLevel = "info"
			}

			data, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			if err := os.WriteFile(paths.ConfigFile(), data, 0o644); err != nil {
				return fmt.Errorf("writing config: %w", err)
			}

			fmt.Printf(`
Device registered:
  name:        %s
  device id:   %s
  organization: %s
  config file: %s

Next steps:
  1. See every supported storage provider and its maturity:
       abm storage providers
  2. Add a storage destination (example: a generic S3-compatible bucket):
       abm storage add --provider generic-s3 --name backblaze --endpoint <url> --access-key <key> --secret-key <secret>
     (see docs/providers/ for Google Drive, OneDrive, Dropbox, SFTP, local,
     and every other provider's exact setup steps)
  3. Add a backup job:
       abm job add --name my-job --source /path/to/data --destination backblaze
  4. Take the first backup and verify it:
       abm backup now my-job
       abm snapshots my-job
  5. Enable the hourly schedule:
       abm schedule set
`, deviceName, id, organization, paths.ConfigFile())
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceName, "device-name", "", "name for this device (skips the prompt)")
	cmd.Flags().StringVar(&organization, "organization", "", "organization name used in repository paths")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "don't prompt; use flags/defaults only")
	return cmd
}

func defaultDeviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "device-" + runtime.GOOS
}

func prompt(r *bufio.Reader, question, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", question, def)
	} else {
		fmt.Printf("%s: ", question)
	}
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}
