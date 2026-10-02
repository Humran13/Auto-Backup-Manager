package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
)

func newRestoreCmd(a *app) *cobra.Command {
	var target string
	var include []string
	var inPlace bool
	var yes bool
	var destination string

	cmd := &cobra.Command{
		Use:   "restore <job> <snapshot|latest>",
		Short: "Restore a job's snapshot to a target directory",
		Long: `Restore a job's snapshot to a target directory.

By default restore goes to a separate --target directory; it never overwrites
live data unless --in-place and --yes are both given, matching the project's
restore-safety rule that a restore must never silently clobber production
data.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobName, snapshotID := args[0], args[1]
			cfg := a.requireConfig()
			job, ok := cfg.Jobs[jobName]
			if !ok {
				return fmt.Errorf("no such job %q", jobName)
			}

			originalInPlace := false
			if inPlace {
				if target != "" {
					return fmt.Errorf("--target cannot be combined with --in-place; original paths come from the recovery manifest")
				}
				if err := validateOriginalSources(runtime.GOOS, job.Sources); err != nil {
					return err
				}
				if !yes {
					fmt.Printf("This will restore %q to %v, OVERWRITING existing files. Continue? [y/N]: ", snapshotID, job.Sources)
					reader := bufio.NewReader(os.Stdin)
					line, _ := reader.ReadString('\n')
					if strings.TrimSpace(strings.ToLower(line)) != "y" {
						fmt.Println("aborted")
						return nil
					}
				}
				target = defaultRestoreTarget(jobName + "-original")
				originalInPlace = true
			} else {
				if target == "" {
					return fmt.Errorf("--target is required unless --in-place is given")
				}
				if err := validateSafeRestoreTarget(runtime.GOOS, target, job.Sources); err != nil {
					return err
				}
			}

			r, err := resticRunnerFor(a, jobName, destination)
			if err != nil {
				return err
			}
			ctx := context.Background()
			if err := r.Restore(ctx, restic.RestoreOptions{
				SnapshotID: snapshotID,
				Target:     target,
				Include:    include,
			}); err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}
			if originalInPlace {
				if err := restoreOriginalSources(runtime.GOOS, target, job.Sources); err != nil {
					return fmt.Errorf("copying verified restore to original locations (staging retained at %s): %w", target, err)
				}
				_ = os.RemoveAll(target)
			}
			if inPlace {
				fmt.Printf("restored %s snapshot %s to its configured original locations\n", jobName, snapshotID)
			} else {
				fmt.Printf("restored %s snapshot %s to %s\n", jobName, snapshotID, target)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&target, "target", "", "directory to restore into (required unless --in-place)")
	cmd.Flags().StringArrayVar(&include, "include", nil, "restrict restore to matching path(s)/pattern(s)")
	cmd.Flags().BoolVar(&inPlace, "in-place", false, "restore over the job's original source path (destructive, requires confirmation)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt for --in-place")
	cmd.Flags().StringVar(&destination, "destination", "", "destination name (default: the job's primary destination)")
	return cmd
}
