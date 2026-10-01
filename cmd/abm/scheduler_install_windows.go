//go:build windows

package main

import (
	"os"

	"github.com/Humran13/Auto-Backup-Manager/internal/scheduler"
)

func installScheduler() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return scheduler.Install(exe)
}
