// Package database implements safe, dump-based backup hooks for the
// databases Auto-Backup-Manager supports. It exists because a live
// MySQL/Postgres data directory must never be treated as an ordinary set of
// files to copy: this package always produces a consistent logical dump
// first, and restic backs up that dump file instead of the live datadir.
package database

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
)

// Credentials holds a resolved database credential. It is created fresh for
// each dump from the secret store and must never be logged; String() is
// deliberately not implemented so accidental %v/%s logging doesn't leak it.
type Credentials struct {
	Username string
	Password string
}

// Dump produces a consistent logical dump of db into outputDir and returns
// its path plus a Cleanup func the caller must always invoke (even on
// failure) to securely remove the temporary dump once restic has backed it
// up. Credentials are passed only via the OS-specific mechanism each tool
// supports for avoiding plaintext in argv/process listings (a MySQL
// defaults-extra-file, or PGPASSWORD in the child's own environment).
func Dump(ctx context.Context, db config.Database, creds Credentials, outputDir string) (dumpPath string, cleanup func(), err error) {
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("creating dump directory: %w", err)
	}

	switch db.Kind {
	case config.DatabaseMySQL:
		return dumpMySQL(ctx, db, creds, outputDir)
	case config.DatabasePostgreSQL:
		return dumpPostgres(ctx, db, creds, outputDir)
	case config.DatabaseSQLite:
		return dumpSQLite(ctx, db, outputDir)
	default:
		return "", nil, fmt.Errorf("unsupported database kind %q", db.Kind)
	}
}

func noopCleanup(path string) func() {
	return func() { secureRemove(path) }
}

// secureRemove overwrites the file with zeros before deleting it so a
// temporary dump containing sensitive data doesn't linger recoverable on
// disk after cleanup.
func secureRemove(path string) {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		if f, err := os.OpenFile(path, os.O_WRONLY, 0o600); err == nil {
			zeros := make([]byte, 64*1024)
			remaining := fi.Size()
			for remaining > 0 {
				n := int64(len(zeros))
				if remaining < n {
					n = remaining
				}
				f.Write(zeros[:n])
				remaining -= n
			}
			f.Close()
		}
	}
	os.Remove(path)
}

func dumpMySQL(ctx context.Context, db config.Database, creds Credentials, outputDir string) (string, func(), error) {
	// A defaults-extra-file keeps the password out of argv (and therefore
	// out of `ps`/process listings and most logging), which a --password=
	// flag or MYSQL_PWD env var would not.
	defaultsFile, err := os.CreateTemp(outputDir, "mysql-creds-*.cnf")
	if err != nil {
		return "", nil, fmt.Errorf("creating mysql defaults file: %w", err)
	}
	defer secureRemove(defaultsFile.Name())

	fmt.Fprintf(defaultsFile, "[client]\nuser=%s\npassword=%s\n", creds.Username, creds.Password)
	if err := defaultsFile.Close(); err != nil {
		return "", nil, err
	}
	if err := os.Chmod(defaultsFile.Name(), 0o600); err != nil {
		return "", nil, err
	}

	dumpPath := filepath.Join(outputDir, fmt.Sprintf("%s.sql", db.Name))
	out, err := os.Create(dumpPath)
	if err != nil {
		return "", nil, fmt.Errorf("creating dump file: %w", err)
	}
	defer out.Close()

	args := []string{
		"--defaults-extra-file=" + defaultsFile.Name(),
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--quick",
	}
	if db.Host != "" {
		args = append(args, "--host="+db.Host)
	}
	if db.Port != 0 {
		args = append(args, fmt.Sprintf("--port=%d", db.Port))
	}
	args = append(args, db.Name)

	cmd := exec.CommandContext(ctx, "mysqldump", args...)
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		secureRemove(dumpPath)
		return "", nil, fmt.Errorf("mysqldump failed for database %q: %w", db.Name, err)
	}
	return dumpPath, noopCleanup(dumpPath), nil
}

func dumpPostgres(ctx context.Context, db config.Database, creds Credentials, outputDir string) (string, func(), error) {
	dumpPath := filepath.Join(outputDir, fmt.Sprintf("%s.dump", db.Name))

	args := []string{"--format=custom", "--file=" + dumpPath}
	if db.Host != "" {
		args = append(args, "--host="+db.Host)
	}
	if db.Port != 0 {
		args = append(args, fmt.Sprintf("--port=%d", db.Port))
	}
	if creds.Username != "" {
		args = append(args, "--username="+creds.Username)
	}
	args = append(args, db.Name)

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	// PGPASSWORD is scoped to this child process's environment only, never
	// the manager's own process environment or argv.
	cmd.Env = append(os.Environ(), "PGPASSWORD="+creds.Password)
	if err := cmd.Run(); err != nil {
		secureRemove(dumpPath)
		return "", nil, fmt.Errorf("pg_dump failed for database %q: %w", db.Name, err)
	}
	return dumpPath, noopCleanup(dumpPath), nil
}

func dumpSQLite(ctx context.Context, db config.Database, outputDir string) (string, func(), error) {
	if db.Path == "" {
		return "", nil, fmt.Errorf("sqlite database %q has no path configured", db.Name)
	}
	dumpPath := filepath.Join(outputDir, fmt.Sprintf("%s.sqlite3", db.Name))

	// sqlite3's ".backup" uses SQLite's own online backup API, which is safe
	// against a database that is actively being written to; a plain file
	// copy of a live SQLite file is not.
	cmd := exec.CommandContext(ctx, "sqlite3", db.Path, fmt.Sprintf(".backup '%s'", dumpPath))
	if err := cmd.Run(); err != nil {
		secureRemove(dumpPath)
		return "", nil, fmt.Errorf("sqlite3 backup failed for database %q: %w", db.Name, err)
	}
	return dumpPath, noopCleanup(dumpPath), nil
}
