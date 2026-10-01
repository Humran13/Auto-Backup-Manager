//go:build !windows

package main

import "github.com/Humran13/Auto-Backup-Manager/internal/scheduler"

func installScheduler() error {
	return scheduler.Install()
}
