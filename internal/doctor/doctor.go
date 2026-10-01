// Package doctor implements the checks behind `abm doctor`: everything that
// commonly goes wrong with a backup deployment, surfaced in one place rather
// than discovered hours later as a silent missed backup.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/job"
)

// Check is a single diagnostic result.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Report is the full set of diagnostic results from one `abm doctor` run.
type Report struct {
	Checks []Check
}

// Failed reports whether any check in the report failed.
func (r Report) Failed() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return true
		}
	}
	return false
}

func (r *Report) add(name string, ok bool, detail string) {
	r.Checks = append(r.Checks, Check{Name: name, OK: ok, Detail: detail})
}

// Options configures which environment doctor inspects.
type Options struct {
	ResticBinary    string
	RcloneBinary    string
	Config          *config.Config
	StateDir        string
	SchedulerStatus func() (string, error)
}

// Run executes every check and returns the aggregate report. It never
// returns an error itself -- a failing check is data in the report, not a Go
// error, since the whole point of `abm doctor` is to surface many partial
// failures at once.
func Run(ctx context.Context, opts Options) Report {
	var r Report

	checkBinary(&r, "restic", opts.ResticBinary)
	checkBinary(&r, "rclone", opts.RcloneBinary)

	if opts.Config == nil {
		r.add("config", false, "no configuration loaded")
	} else {
		r.add("config", true, fmt.Sprintf("%d job(s), %d storage destination(s)", len(opts.Config.Jobs), len(opts.Config.Storage)))
		for name, j := range opts.Config.Jobs {
			checkJobSources(&r, name, j)
			checkJobStatus(&r, opts.StateDir, name)
		}
	}

	if opts.SchedulerStatus != nil {
		out, err := opts.SchedulerStatus()
		r.add("scheduler", err == nil, firstLine(out, err))
	}

	checkDatabaseTools(&r, opts.Config)
	if runtime.GOOS == "windows" {
		checkVSS(&r)
	}

	return r
}

func checkBinary(r *Report, name, path string) {
	if path == "" {
		path = name
	}
	out, err := exec.Command(path, "version").CombinedOutput()
	r.add(name, err == nil, firstLine(string(out), err))
}

func checkJobSources(r *Report, name string, j config.Job) {
	for _, src := range j.Sources {
		if _, err := os.Stat(src); err != nil {
			r.add("job:"+name+":source:"+src, false, err.Error())
			return
		}
	}
	r.add("job:"+name+":sources", true, fmt.Sprintf("%d source path(s) present", len(j.Sources)))
}

func checkJobStatus(r *Report, stateDir, name string) {
	st, err := job.LoadStatus(stateDir, name)
	if err != nil {
		r.add("job:"+name+":status", false, err.Error())
		return
	}
	if st.LastSuccess.IsZero() {
		r.add("job:"+name+":last-backup", false, "no successful backup recorded yet")
		return
	}
	age := time.Since(st.LastSuccess)
	ok := age < 25*time.Hour // hourly schedule; allow slack for one missed run
	snapshot := ""
	if primary := st.Primary(); primary != nil {
		snapshot = primary.LastSnapshotID
	}
	detail := fmt.Sprintf("last success %s ago (snapshot %s)", age.Round(time.Minute), snapshot)
	if st.Degraded {
		detail += " [degraded: a secondary destination is failing]"
	}
	r.add("job:"+name+":last-backup", ok, detail)
}

// requiredTools maps each database kind to the dump tool it needs.
var requiredTools = map[config.DatabaseKind]string{
	config.DatabaseMySQL:      "mysqldump",
	config.DatabasePostgreSQL: "pg_dump",
	config.DatabaseSQLite:     "sqlite3",
}

// checkDatabaseTools only fails a tool check when some configured job
// actually depends on it; an unused dump tool being absent is not a problem
// worth failing `abm doctor` over.
func checkDatabaseTools(r *Report, cfg *config.Config) {
	needed := map[string]bool{}
	if cfg != nil {
		for _, j := range cfg.Jobs {
			for _, db := range j.Databases {
				if tool, ok := requiredTools[db.Kind]; ok {
					needed[tool] = true
				}
			}
		}
	}
	for _, tool := range []string{"mysqldump", "pg_dump", "sqlite3"} {
		_, err := exec.LookPath(tool)
		if err == nil {
			r.add("db-tool:"+tool, true, "available")
			continue
		}
		if needed[tool] {
			r.add("db-tool:"+tool, false, "required by a configured job but not installed")
		} else {
			r.add("db-tool:"+tool, true, "not installed (not used by any configured job)")
		}
	}
}

func firstLine(s string, err error) string {
	for i, c := range s {
		if c == '\n' {
			s = s[:i]
			break
		}
	}
	if err != nil && s == "" {
		return err.Error()
	}
	return s
}
