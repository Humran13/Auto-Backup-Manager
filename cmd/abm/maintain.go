package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/job"
)

func newMaintainCmd(a *app) *cobra.Command {
	var all, prune bool
	cmd := &cobra.Command{
		Use:   "maintain [job]",
		Short: "Apply retention policy (restic forget), optionally pruning",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			deps := a.jobDeps()
			ctx := context.Background()

			var names []string
			switch {
			case all:
				for name := range cfg.Jobs {
					names = append(names, name)
				}
			case len(args) == 1:
				names = []string{args[0]}
			default:
				return fmt.Errorf("specify a job name or --all")
			}

			failed := false
			for _, name := range names {
				if err := job.Maintain(ctx, deps, name, prune); err != nil {
					failed = true
					fmt.Fprintf(os.Stderr, "job %s: FAILED: %v\n", name, err)
					continue
				}
				fmt.Printf("job %s: retention applied (prune=%v)\n", name, prune)
			}
			if failed {
				return fmt.Errorf("one or more jobs failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "apply retention to every job")
	cmd.Flags().BoolVar(&prune, "prune", false, "also reclaim space for removed snapshots (run at most daily)")
	return cmd
}
