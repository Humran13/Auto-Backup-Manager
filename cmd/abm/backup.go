package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/job"
)

func newBackupCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Run backups on demand",
	}
	cmd.AddCommand(newBackupNowCmd(a))
	return cmd
}

func newBackupNowCmd(a *app) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "now [job]",
		Short: "Run a backup immediately for one job, or all enabled jobs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			deps := a.jobDeps()
			ctx := context.Background()

			var names []string
			switch {
			case all:
				for name, j := range cfg.Jobs {
					if j.Enabled {
						names = append(names, name)
					}
				}
			case len(args) == 1:
				names = []string{args[0]}
			default:
				return fmt.Errorf("specify a job name or --all")
			}

			failed := false
			for _, name := range names {
				st, err := job.Run(ctx, deps, name)
				if err != nil {
					failed = true
					fmt.Fprintf(os.Stderr, "job %s: FAILED: %v\n", name, err)
					continue
				}
				primary := st.Primary()
				degraded := ""
				if st.Degraded {
					degraded = " (DEGRADED: a secondary destination failed)"
				}
				fmt.Printf("job %s: OK snapshot=%s new=%d changed=%d unmodified=%d%s\n",
					name, primary.LastSnapshotID, primary.FilesNew, primary.FilesChanged, primary.FilesUnmodified, degraded)
			}
			if failed {
				return fmt.Errorf("one or more jobs failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "run every enabled job")
	return cmd
}
