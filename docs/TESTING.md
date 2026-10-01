# Testing

This documents exactly what has been tested, how, and what has not — so
nothing here is claimed as verified that wasn't.

## Automated unit tests (`go test ./...`)

Run in CI on both Ubuntu and Windows (`.github/workflows/ci.yml`):

- **Config** (`internal/config`): schema validation (valid config passes;
  missing device ID, unknown destination, empty/duplicate sources, unsafe
  job names, future schema versions, missing MySQL `credentials_ref`,
  missing SQLite path all correctly rejected); zero-jobs bootstrap state
  allowed; schema-version defaulting and migration rejection of future
  versions.
- **Retention** (`internal/retention`): `restic forget` argument
  construction for both the default `--keep-within-hourly` policy and
  explicit count-based policies; rejection of an all-zero policy and an
  invalid duration string; a pure-Go simulation of 15 days of perfectly
  on-schedule hourly snapshots against the default 240-hour window,
  asserting exactly 240 are kept and the rest removed (the project's
  required "10-day/hourly retention simulation"); same-hour-bucket
  deduplication (only the most recent of two same-hour snapshots survives);
  and a snapshot list entirely outside the window keeping nothing.
- **Secrets** (`internal/secrets`): redaction of `key=value`/`key: value`
  secret-shaped text and `user:pass@host` URLs; confirms non-secret text is
  left untouched.
- **Locking** (`internal/lock`): acquire/release; a second acquire fails
  while the first holds the lock; a lock is correctly reclaimed after
  release; a **stale lock left by a dead PID is correctly reclaimed**
  (simulating a crashed/killed/rebooted prior run); release without acquire
  is a safe no-op.
- **restic wrapper** (`internal/restic`): backup summary parsing finds the
  terminal `"summary"` message and treats a run that never produced one as a
  failure (never a false success); `Latest()` correctly picks the newest
  snapshot by timestamp and errors on an empty list; `isAlreadyInitialized`
  correctly distinguishes "repository already exists" from a real
  connectivity/missing-repository error.
- **Job orchestration** (`internal/job`): database credential resolution
  (`username:password` parsing, SQLite needing none, malformed refs
  rejected); storage lookup; repository path construction for both
  rclone-backed and local destinations; status JSON save/load round-trip,
  including that a never-run job correctly reports as not yet initialized.

## Integration test against real restic (`test/integration`)

`TestAcceptance_BackupModifyDeleteRestore` automates the project's full
acceptance scenario end to end against a real `restic` binary and a local
filesystem destination (skipped automatically if `restic` isn't on `PATH`;
CI installs it explicitly so this always runs there):

1. Create sample files (3 files across a subdirectory).
2. Backup (snapshot A).
3. Modify one file, add a new one.
4. Backup (snapshot B) — asserts exactly 1 new + 1 changed + 2 unmodified
   file, confirming deduplication is actually happening, not re-uploading
   everything.
5. Delete a file and a subdirectory.
6. Backup (snapshot C).
7. List all snapshots — asserts exactly 3 exist.
8. Restore snapshot A to a clean directory — asserts its contents exactly
   match the original pre-modification state (including the deleted file
   and subdirectory being present, and the later-added file being absent).
9. Restore `latest` to a clean directory — asserts it matches the expected
   post-deletion state (modified content present, new file present, deleted
   file/subdirectory absent).
10. **Simulate a destination failure** (rename the destination directory
    away) and run a backup — asserts it fails loudly rather than reporting
    success.
11. Restore the destination and assert **all 3 original snapshots are still
    present** (the failed run did not corrupt or lose history) and that
    `latest` is still restorable afterward.

This test was run manually, found, and was used to find and fix two real
bugs during development (see below), then automated so it runs on every CI
push.

## Manual testing performed during development

Beyond the automated suite above, the following were exercised manually on
Windows (this project's development machine) via the built `abm.exe`:
`setup`, `storage add --type local`, `job add`, `backup now` (including the
full modify/delete/backup cycle above before it was automated), `snapshots`,
`restore` (snapshot-by-ID and `latest`, both verified byte-for-byte against
expected file contents), `status`, `check`, `maintain` (with and without
`--prune`), `doctor`, and `schedule set` (confirmed it correctly **fails**
with "Access is denied" when not run elevated, rather than silently
succeeding — Task Scheduler registration genuinely requires admin rights).

## Bugs found and fixed during this testing

1. **`restic.Runner.run` replaced the child process environment instead of
   inheriting it**, so restic lost `PATH`/`LOCALAPPDATA`/`HOME` and failed
   to locate its own cache directory. Fixed by starting from `os.Environ()`.
2. **VSS was unconditionally forced on every Windows backup**, so any
   unelevated `abm backup now` always failed with
   `E_ACCESSDENIED`. Fixed by checking process elevation
   (`internal/job.canUseFSSnapshot`) and degrading to a plain backup with a
   clearly reported warning when not elevated, while scheduled (SYSTEM,
   always elevated) runs are unaffected.
3. **A temporarily unreachable destination was silently reinitialized as a
   brand-new, empty repository**, because the pre-backup "does this
   repository exist?" check couldn't distinguish "genuinely new" from
   "temporarily unreachable" — exactly the kind of failure the project's own
   threat model calls out (repository deletion/corruption). Fixed by
   tracking initialization state locally (`Status.Initialized`) and never
   calling `restic init` again after the first successful one; a repository
   that later becomes unreachable now fails the backup loudly instead.
4. **`abm doctor` treated VSS being `STOPPED` as a failure**, when VSS is a
   normal demand-start service that is healthy while stopped. Fixed to check
   whether the service is disabled, not whether it's currently running.
5. **`abm doctor` failed the entire report whenever an unused database dump
   tool (e.g. `pg_dump` with no PostgreSQL job configured) wasn't
   installed.** Fixed to only fail a tool check when a configured job
   actually depends on it.
6. **(Found by CI, not locally)** `go.mod`'s `go` directive had been
   auto-bumped by `go mod tidy` to `1.26.0` using this machine's installed
   Go 1.27 toolchain. That patch-version directive format broke every older
   Go toolchain trying to parse it, and separately, Ubuntu 20.04/22.04's
   `apt` `golang-go` packages (1.13/1.18) predate `log/slog` entirely and
   can't build this code at any `go.mod` version. Fixed by pinning
   `go 1.22` and having CI's Ubuntu-version matrix install a known-good Go
   toolchain directly from go.dev rather than trusting each distro's apt
   package, the same pin-and-verify approach already used for restic/rclone.
7. **(Found by CI, not locally)** the gitleaks secret-scan job flagged the
   intentionally fake, secret-shaped test fixtures in
   `internal/secrets/redact_test.go` (e.g. an AWS-access-key-shaped string
   used to test the redactor itself). These were never real credentials.
   Fixed with a narrowly-scoped `.gitleaks.toml` allowlist for that one test
   file only — every other file is still scanned normally.

## Tests that could NOT be performed, and why

- **Windows VSS actually engaging** (an elevated, scheduled backup of a
  genuinely open/locked file) was not verified in this sandboxed development
  environment, which runs unelevated; `canUseFSSnapshot`'s elevation check
  itself was exercised (confirmed `false` when unelevated, triggering the
  documented warning), but the elevated code path through actual VSS
  shadow-copy creation was not. This needs real-world testing on an
  administrator-run scheduled task.
- **Real cloud provider round-trips** (Google Drive, OneDrive, Dropbox, a
  real S3-compatible bucket, a real SFTP server) were not tested — doing so
  would require real credentials/accounts, which weren't available in this
  environment. Only the local-filesystem destination path was exercised.
  The rclone wrapper functions (`CreateS3Remote`, `CreateSFTPRemote`,
  `AuthorizeOAuth`, `CreateOAuthRemote`, `Test`) are implemented and unit-
  testable in isolation but have no network-backed integration test.
- **Database-aware backup/restore** (`mysqldump`/`pg_dump`/`sqlite3`) was
  not run end-to-end in this environment because none of those tools nor a
  database server were installed; `internal/database` has no automated test
  exercising a real dump/restore cycle. Standing up disposable MySQL/
  PostgreSQL containers for this (as the original project brief calls for)
  requires a running container runtime, which wasn't available in this
  session (Docker Desktop's engine was not running).
- **Multi-day/real hourly schedule behavior** (does the systemd timer/Task
  Scheduler task actually fire on the hour, every hour, across a real
  reboot, for days) was not observed in real time; only the policy logic
  (`internal/retention.Simulate`) and the generated unit/task definitions
  were verified, not a live multi-day run.
- **A real Ubuntu container matrix run** (20.04/22.04/24.04/26.04 via
  `.github/workflows/ci.yml`'s `ubuntu-matrix` job) has not executed yet —
  it runs on the next push to GitHub Actions, not in this local sandbox.
- **Linux install.sh / systemd timer installation** was not run end-to-end
  on an actual Ubuntu machine in this session (development happened on
  Windows); the Linux build was cross-compiled and `go vet`-checked, and the
  retention/config/lock/secrets packages have Linux-specific code paths
  (`_unix.go`/`_linux.go` build-tagged files) that compile cleanly for
  `GOOS=linux` but were not exercised at runtime here.
- **Repository corruption detection** (`abm check --read-data-subset`)
  was exercised only on a healthy repository; deliberately corrupting a
  repository to confirm `check` catches it was not attempted.

## Known functional gaps (not bugs — not yet built)

- No fully interactive, single-flow setup wizard chaining storage → job →
  first backup → schedule in one guided session; `abm setup` bootstraps
  device identity/config and prints the next commands to run separately.
- No interactive restore browser (choose job → browse snapshot contents →
  preview → restore); `abm snapshots` + `abm restore --include` cover the
  same ground non-interactively.
- No automated disaster-recovery inventory command; see
  [DISASTER-RECOVERY.md](DISASTER-RECOVERY.md) for the manual commands to
  run instead.
- No automated database/Docker-bind-mount auto-detection.
- Only a single primary destination per job is implemented; the config
  schema and repository-path design intentionally leave room for a second,
  differently-credentialed destination later without a breaking change, but
  multi-destination replication itself is not built.
