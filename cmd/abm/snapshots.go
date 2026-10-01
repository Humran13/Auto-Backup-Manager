package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
)

func newSnapshotsCmd(a *app) *cobra.Command {
	var destination string
	cmd := &cobra.Command{
		Use:   "snapshots [job]",
		Short: "List recoverable snapshots for one job, or all jobs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			ctx := context.Background()

			names := args
			if len(names) == 0 {
				for name := range cfg.Jobs {
					names = append(names, name)
				}
			}

			for _, name := range names {
				r, err := resticRunnerFor(a, name, destination)
				if err != nil {
					fmt.Printf("%s: %v\n", name, err)
					continue
				}
				snaps, err := r.Snapshots(ctx, nil)
				if err != nil {
					fmt.Printf("%s: %v\n", name, err)
					continue
				}
				latest, _ := restic.Latest(snaps)
				fmt.Printf("job: %s (%d snapshot(s))\n", name, len(snaps))
				for _, s := range snaps {
					marker := ""
					if latest != nil && s.ID == latest.ID {
						marker = "  <- latest"
					}
					fmt.Printf("  %-10s %s%s\n", s.ShortID, s.Time.Format("2006-01-02 15:04:05 MST"), marker)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&destination, "destination", "", "destination name (default: the job's primary destination)")
	return cmd
}
