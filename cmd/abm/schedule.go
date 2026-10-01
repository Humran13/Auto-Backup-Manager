package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/scheduler"
)

func newScheduleCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage the OS-native hourly backup schedule",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show the current scheduler status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := scheduler.Status()
			fmt.Println(out)
			return err
		},
	})

	var interval string
	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Install/refresh the scheduled hourly backup and daily maintenance tasks",
		Long: `Install/refresh the scheduled hourly backup and daily maintenance tasks.

On Linux this writes and enables systemd service+timer units. On Windows it
registers Task Scheduler tasks running as SYSTEM. Both run 'abm backup now
--all' hourly (persisting across reboot and catching up missed runs) and
'abm maintain --all --prune' daily.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval != "" && interval != "hourly" {
				return fmt.Errorf("only --interval hourly is currently supported")
			}
			if err := installScheduler(); err != nil {
				return fmt.Errorf("installing scheduler: %w", err)
			}
			fmt.Println("hourly backup schedule installed")
			return nil
		},
	}
	setCmd.Flags().StringVar(&interval, "interval", "hourly", "backup interval (currently only 'hourly' is supported)")
	cmd.AddCommand(setCmd)

	return cmd
}
