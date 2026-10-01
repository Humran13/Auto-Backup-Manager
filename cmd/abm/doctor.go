package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/doctor"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/scheduler"
)

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common backup deployment problems",
		RunE: func(cmd *cobra.Command, args []string) error {
			report := doctor.Run(context.Background(), doctor.Options{
				Config:          a.cfg,
				StateDir:        paths.StateDir,
				SchedulerStatus: scheduler.Status,
			})
			for _, c := range report.Checks {
				status := "OK  "
				if !c.OK {
					status = "FAIL"
				}
				fmt.Printf("[%s] %-30s %s\n", status, c.Name, c.Detail)
			}
			if report.Failed() {
				return fmt.Errorf("one or more checks failed")
			}
			return nil
		},
	}
}
