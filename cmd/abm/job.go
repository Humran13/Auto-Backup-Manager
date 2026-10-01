package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/retention"
)

func newJobCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Manage backup jobs",
	}
	cmd.AddCommand(newJobListCmd(a), newJobAddCmd(a), newJobRemoveCmd(a), newJobEditCmd(a), newJobDBCredentialsCmd(a))
	return cmd
}

func newJobDBCredentialsCmd(a *app) *cobra.Command {
	var username string
	cmd := &cobra.Command{
		Use:   "set-db-credentials <credentials_ref>",
		Short: "Store a database credential referenced by a job's database hook (credentials_ref)",
		Long: `Store a database credential referenced by a job's database hook.

<credentials_ref> must match the credentials_ref value used in a job's
"databases:" entry in config.yaml. The password is read from stdin, never
passed as a command-line argument, so it never ends up in shell history or a
process listing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			if username == "" {
				return fmt.Errorf("--username is required")
			}
			fmt.Print("Enter the database password: ")
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			password := strings.TrimSpace(line)
			if password == "" {
				return fmt.Errorf("no password entered")
			}
			if err := a.dbSecrets.Set(ref, username+":"+password); err != nil {
				return fmt.Errorf("storing credential: %w", err)
			}
			fmt.Printf("credential %q stored\n", ref)
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "database username")
	return cmd
}

func newJobListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			for name, j := range cfg.Jobs {
				state := "enabled"
				if !j.Enabled {
					state = "disabled"
				}
				fmt.Printf("%-20s %-9s destinations=%v (policy=%s) sources=%v\n", name, state, j.Destinations, j.EffectivePolicy(), j.Sources)
			}
			return nil
		},
	}
}

func newJobEditCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Print the config file path to edit directly",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Edit jobs directly in:", paths.ConfigFile())
			fmt.Println("After editing, run 'abm doctor' to validate before the next scheduled run.")
			return nil
		},
	}
}

func newJobRemoveCmd(a *app) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a job from the configuration (does not delete its backup repository)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			if _, ok := cfg.Jobs[args[0]]; !ok {
				return fmt.Errorf("no such job %q", args[0])
			}
			if !yes {
				return fmt.Errorf("pass --yes to confirm removal of job %q from local config (the remote backup repository is never deleted)", args[0])
			}
			delete(cfg.Jobs, args[0])
			return config.Save(paths.ConfigFile(), cfg)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal")
	return cmd
}

func newJobAddCmd(a *app) *cobra.Command {
	var (
		name, repoPath, keepWithin, policy string
		sources, excludes, tags, dests     []string
		maxFileSizeMB                      int64
		disabled, recoverExisting          bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a backup job",
		Long: `Add a backup job.

--destination may be given more than once to back the job up to multiple
destinations; the first one given is the primary. By default (--policy
primary-required) a secondary destination failing only marks the run
degraded, while the primary succeeding still counts as a success; pass
--policy all-required to instead require every destination to succeed.

By default this generates a brand-new repository password for each new
destination. To instead recover a repository that already exists at a
destination/path (disaster recovery onto a replacement machine), pass
--recover-existing with exactly one --destination and
--repository-path pointing at the ORIGINAL org/device-id/job-name path; you
will be prompted for the existing password on stdin, never as a command-line
argument, so it never ends up in shell history or a process listing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if len(sources) == 0 {
				return fmt.Errorf("at least one --source is required")
			}
			if len(dests) == 0 {
				return fmt.Errorf("at least one --destination is required (see 'abm storage list')")
			}
			if recoverExisting && len(dests) != 1 {
				return fmt.Errorf("--recover-existing requires exactly one --destination")
			}
			switch config.DestinationPolicyMode(policy) {
			case "", config.PolicyAllRequired, config.PolicyPrimaryRequired:
			default:
				return fmt.Errorf("--policy must be %q or %q", config.PolicyAllRequired, config.PolicyPrimaryRequired)
			}
			cfg := a.cfg
			if cfg == nil {
				return fmt.Errorf("no configuration found. Run 'abm setup' first")
			}
			if repoPath == "" {
				repoPath = cfg.Global.Organization + "/" + cfg.Global.DeviceID + "/" + name
			}
			if keepWithin == "" {
				keepWithin = retention.DefaultKeepWithinHourly
			}

			j := config.Job{
				Sources:           sources,
				Excludes:          excludes,
				MaxFileSizeMB:     maxFileSizeMB,
				Destinations:      dests,
				DestinationPolicy: config.DestinationPolicy{Mode: config.DestinationPolicyMode(policy)},
				RepositoryPath:    repoPath,
				Schedule:          "hourly",
				Retention:         &config.Retention{KeepWithinHourly: keepWithin, PruneSchedule: "daily"},
				Tags:              tags,
				Enabled:           !disabled,
			}

			if cfg.Jobs == nil {
				cfg.Jobs = map[string]config.Job{}
			}
			cfg.Jobs[name] = j
			if err := config.Save(paths.ConfigFile(), cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			for _, dest := range dests {
				var password string
				if recoverExisting {
					fmt.Printf("Enter the existing repository password for destination %q: ", dest)
					reader := bufio.NewReader(os.Stdin)
					line, _ := reader.ReadString('\n')
					password = strings.TrimSpace(line)
					if password == "" {
						return fmt.Errorf("no password entered")
					}
				} else {
					var err error
					password, err = randomPassword()
					if err != nil {
						return fmt.Errorf("generating repository password: %w", err)
					}
				}
				if err := a.secrets.Set(job.ResticPasswordKey(name, dest), password); err != nil {
					return fmt.Errorf("storing repository password for %q: %w", dest, err)
				}
			}

			if recoverExisting {
				fmt.Printf("job %q added, using the provided existing repository password.\n", name)
				fmt.Println("Run 'abm snapshots", name, "' to confirm you can see the expected history before restoring.")
			} else {
				fmt.Printf("job %q added with %d destination(s). New repository password(s) were generated and stored securely.\n", name, len(dests))
				fmt.Println("Run 'abm backup now", name, "' to take the first backup.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "job name")
	cmd.Flags().StringArrayVar(&sources, "source", nil, "source path to back up (repeatable)")
	cmd.Flags().StringArrayVar(&excludes, "exclude", nil, "exclusion pattern (repeatable)")
	cmd.Flags().StringArrayVar(&dests, "destination", nil, "storage destination name (repeatable; first is primary; see 'abm storage list')")
	cmd.Flags().StringVar(&policy, "policy", "", "multi-destination policy: all-required|primary-required (default primary-required)")
	cmd.Flags().StringVar(&repoPath, "repository-path", "", "path within the destination (default: org/device-id/job-name)")
	cmd.Flags().StringVar(&keepWithin, "keep-within-hourly", "", "retention window, e.g. 240h for 10 days (default)")
	cmd.Flags().Int64Var(&maxFileSizeMB, "max-file-size-mb", 0, "skip files larger than this (0 = unlimited)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "snapshot tag (repeatable)")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "create the job disabled")
	cmd.Flags().BoolVar(&recoverExisting, "recover-existing", false, "recover an existing repository (disaster recovery) instead of creating a new one; prompts for its password on stdin")
	return cmd
}

func randomPassword() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
