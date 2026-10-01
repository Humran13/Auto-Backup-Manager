package main

import (
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
)

func (a *app) jobDeps() *job.Deps {
	return &job.Deps{
		Config:       a.requireConfig(),
		ResticBinary: "restic",
		RcloneConfig: paths.RcloneConfigFile(),
		StateDir:     paths.StateDir,
		LockDir:      paths.LockDir,
		DumpDir:      paths.DumpDir,
		Secrets:      a.secrets,
		DBCreds:      a.dbSecrets,
		Logger:       a.log,
		Now:          time.Now,
	}
}

// resticRunnerFor resolves jobName's (optionally specific) destination and
// password into a ready *restic.Runner for commands (snapshots/restore/
// check) that talk to restic directly rather than through the full job.Run
// orchestration. An empty destName resolves to the job's primary destination.
func resticRunnerFor(a *app, jobName, destName string) (*restic.Runner, error) {
	r, _, err := job.ResticRunner(a.jobDeps(), jobName, destName)
	return r, err
}
