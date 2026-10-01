package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
)

func newStatusCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the last backup status for every configured job",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			if len(cfg.Jobs) == 0 {
				fmt.Println("no jobs configured")
				return nil
			}
			for name := range cfg.Jobs {
				st, err := job.LoadStatus(paths.StateDir, name)
				if err != nil {
					fmt.Printf("%-20s ERROR: %v\n", name, err)
					continue
				}
				if st.LastSuccess.IsZero() {
					fmt.Printf("%-20s never backed up successfully", name)
					if st.LastError != "" {
						fmt.Printf(" (last error: %s)", st.LastError)
					}
					fmt.Println()
					continue
				}
				fmt.Printf("%-20s last success: %-25s snapshot: %-10s new=%d changed=%d unmodified=%d destination=%s\n",
					name, st.LastSuccess.Format("2006-01-02 15:04:05 MST"), st.LastSnapshotID,
					st.FilesNew, st.FilesChanged, st.FilesUnmodified, st.Destination)
				if st.LastError != "" {
					fmt.Printf("%-20s WARNING: most recent attempt failed: %s\n", "", st.LastError)
				}
			}
			return nil
		},
	}
}
