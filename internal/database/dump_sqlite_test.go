package database

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
)

func requireSQLite3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not found on PATH; skipping real SQLite dump/restore test")
	}
}

// sqliteExec runs one or more SQL statements against dbPath via the sqlite3
// CLI -- the same tool (and the same way) ABM's own code shells out to it,
// rather than importing a SQL driver package (which this project
// deliberately never does; internal/database only ever shells out to each
// database's native CLI tool).
func sqliteExec(t *testing.T, dbPath, sql string) {
	t.Helper()
	cmd := exec.Command("sqlite3", dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 exec failed: %v: %s", err, out)
	}
}

func sqliteQuery(t *testing.T, dbPath, sql string) string {
	t.Helper()
	cmd := exec.Command("sqlite3", dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 query failed: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestDumpSQLite_RealCreateBackupDestroyRestoreVerify is the project's
// create -> backup -> destroy -> restore -> verify cycle for SQLite,
// against a real on-disk database (no container needed, unlike
// MySQL/PostgreSQL) with real rows, using ABM's actual dump code
// (internal/database.Dump), which in turn uses SQLite's own safe online
// backup mechanism (".backup"), not a raw file copy.
func TestDumpSQLite_RealCreateBackupDestroyRestoreVerify(t *testing.T) {
	requireSQLite3(t)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")

	// Create a real database with a table and rows.
	sqliteExec(t, dbPath, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
INSERT INTO users (id, name, email) VALUES
  (1, 'Alice', 'alice@example.com'),
  (2, 'Bob', 'bob@example.com'),
  (3, 'Carol', 'carol@example.com');`)

	originalRows := sqliteQuery(t, dbPath, "SELECT id, name, email FROM users ORDER BY id;")
	if strings.Count(originalRows, "\n") != 2 { // 3 lines, 2 newlines
		t.Fatalf("expected 3 rows in the original database, got: %q", originalRows)
	}

	// Backup via ABM's actual dump code path (internal/database.Dump), the
	// same one a real backup job invokes.
	outputDir := filepath.Join(dir, "dump")
	dbConfig := config.Database{Kind: config.DatabaseSQLite, Name: "app", Path: dbPath}
	dumpPath, cleanup, err := Dump(context.Background(), dbConfig, Credentials{}, outputDir)
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(dumpPath); err != nil {
		t.Fatalf("dump file missing: %v", err)
	}

	// Destroy the "original" database, simulating total loss.
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("destroying original database: %v", err)
	}

	// Restore: the dump IS a valid standalone SQLite database file (that's
	// what ".backup" produces), so "restoring" it is just putting it back in
	// place -- exactly what a real restic restore followed by a file move
	// would do in production, as described in docs/RESTORE.md's "Database
	// restore" section.
	restoredPath := filepath.Join(dir, "app-restored.db")
	dumpBytes, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("reading dump: %v", err)
	}
	if err := os.WriteFile(restoredPath, dumpBytes, 0o644); err != nil {
		t.Fatalf("writing restored database: %v", err)
	}

	// Verify: every row survived, with correct values -- the actual
	// release-gate requirement ("create records -> backup -> destroy ->
	// restore -> validate records").
	restoredRows := sqliteQuery(t, restoredPath, "SELECT id, name, email FROM users ORDER BY id;")
	if restoredRows != originalRows {
		t.Fatalf("restored rows don't match original:\n got:  %q\n want: %q", restoredRows, originalRows)
	}
}
