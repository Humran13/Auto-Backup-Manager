package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
)

func newLogsCmd(a *app) *cobra.Command {
	var tail int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent Auto-Backup-Manager log lines",
		Long: `Show recent Auto-Backup-Manager log lines from ` + paths.LogDir + `.

On Linux this is a plain-file supplement to 'journalctl -u auto-backup-manager';
on Windows it supplements the Task Scheduler history for the scheduled task.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := filepath.Join(paths.LogDir, "abm.log")
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("opening %s: %w", path, err)
			}
			defer f.Close()

			var lines []string
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
				if len(lines) > tail {
					lines = lines[1:]
				}
			}
			for _, l := range lines {
				fmt.Println(l)
			}
			return scanner.Err()
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 100, "number of most recent lines to show")
	return cmd
}
