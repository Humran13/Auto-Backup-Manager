package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/scheduler"
)

func newUninstallCmd(a *app) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the scheduled backup task/timer (never deletes backup repositories or config)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Print("This removes the OS scheduler entry only. Config, secrets and remote backup repositories are never touched. Continue? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				line, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(line)) != "y" {
					fmt.Println("aborted")
					return nil
				}
			}
			if err := scheduler.Uninstall(); err != nil {
				return err
			}
			fmt.Println("scheduler removed. Backup repositories were not touched.")
			fmt.Println("To also remove local config/state, delete the Auto-Backup-Manager config/state/log directories manually.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}
