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
  rejected); storage lookup; per-destination password key resolution
  (prefers the new per-destination secret key, falls back to the legacy
  pre-multi-destination key for a single-destination job); status JSON
  save/load round-trip including multiple destination results, including
  that a never-run job correctly reports as not yet initialized.
- **Provider registry** (`internal/provider`): every one of the 26 cataloged
  providers has complete metadata (display name, family, backend, maturity,
  auth method, headless classification, at least one required field, a doc
  file that actually exists on disk) -- a provider cannot be added
  half-finished without a test failing; secret-shaped field names are
  verified to be marked `Secret`; the specific provider set this phase was
  asked to cover is asserted present by ID.
- **Repository backend construction** (`internal/backend`): local, SFTP
  (default and non-default port/key-file forms), S3 (secret resolution,
  env var construction, missing-credential error names the exact fix
  command), and rclone repository-spec construction, each checked against
  the exact restic `-r` string and environment/extra-args produced; an
  unknown provider is rejected.
- **Config migration** (`internal/config`): a v1 document (old fixed
  `type`/`rclone_remote` schema, single `destination` string) is parsed with
  its own v1 shape and converted to v2 -- verified separately for the
  "needs no further action" case (Google Drive: the old rclone remote
  carries forward as the new `remote` option, repository_path unchanged,
  migrated config validates immediately) and the "needs reconfiguration"
  case (S3/SFTP: provider mapping and non-secret options like `endpoint`
  carry forward, but credentials that lived only in the old `rclone.conf`
  cannot be and are not silently fabricated).

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

## Real protocol-level integration tests (`test/integration`, `TestRealProvider_*`)

Beyond the acceptance test's local-filesystem destination, two tests
exercise ABM's actual `job.Run`/`backend.Build` code (not raw restic calls)
against real protocol servers, gated behind environment variables so they
skip cleanly without credentials and are never required for a plain
`go test ./...`:

- **`TestRealProvider_S3`** against a disposable
  [adobe/s3mock](https://hub.docker.com/r/adobe/s3mock) container: backup,
  modify, second backup (asserts exactly 1 changed file), restore `latest`,
  verify content. This is a real S3-protocol round-trip through restic's
  native `s3` backend, not a mock of our own code.
- **`TestRealProvider_SFTP`** against a disposable
  [atmoz/sftp](https://hub.docker.com/r/atmoz/sftp) container with a
  freshly generated SSH key: backup, restore `latest`, verify content, via
  restic's native `sftp` backend and ABM's `-o sftp.command` override path
  (the one a real bug was found in -- see below).

CI runs both automatically (`.github/workflows/ci.yml`'s
`protocol-integration` job) by starting these containers and setting the
required environment variables; a developer can do the same locally. Neither
test uses or accepts real cloud credentials. **MinIO was the original choice
for the S3 test** but its Docker Hub images (`minio/minio`, every tag) now
require authentication to pull, discovered while setting this up in this
session -- `adobe/s3mock` was substituted as a freely-pullable alternative.

## Manual testing performed during development

Beyond the automated suite above, the following were exercised manually on
Windows (this project's development machine) via the built `abm.exe`:
`setup`, `storage providers`, `storage add` (generic provider-driven flow,
local provider and two local destinations for multi-destination testing),
`storage show`, `job add` (including `--destination` given twice with the
default `primary-required` policy), `backup now` (including the full
modify/delete/backup cycle above before it was automated, and a simulated
**secondary**-destination failure that correctly reported the run as
successful-but-degraded rather than failed), `snapshots`, `restore`
(snapshot-by-ID and `latest`, both verified byte-for-byte against expected
file contents), `status` (including the per-destination/degraded display),
`check`, `maintain` (with and without `--prune`), `doctor`, and
`schedule set` (confirmed it correctly **fails** with "Access is denied"
when not run elevated, rather than silently succeeding -- Task Scheduler
registration genuinely requires admin rights). The real S3/SFTP protocol
tests above were also run manually against locally started containers
before being wired into CI.

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
8. **The capability probe (`abm storage add`'s init/backup/restore
   round-trip) inherited the same Windows-unelevated-ACL quirk as the
   original acceptance test** (restic failing to fix up a parent directory's
   timestamp under a deeply-nested `%TEMP%` path). Fixed the same way:
   `internal/backend.probeTempDir` uses a shallow root directly under the
   temp drive on Windows.
9. **`internal/backend.buildS3` hardcoded the `https://` scheme**, so any
   plain-HTTP S3-compatible server (the `adobe/s3mock` test server used
   above, and potentially some on-prem/self-hosted S3-compatible setups)
   failed with "server gave HTTP response to HTTPS client." Found by
   `TestRealProvider_S3` failing on first run. Fixed to respect an explicit
   `http://` prefix on the configured endpoint, defaulting to `https://`
   otherwise.
10. **`internal/backend.buildSFTP`'s non-default-port repository spec was
    invalid restic syntax**: it built `sftp:user@host:port:path`, but
    restic's sftp backend has no such form -- the "host:port" fragment gets
    treated as a single malformed hostname, which fails to connect. Found by
    `TestRealProvider_SFTP` failing on first run against a container on a
    non-default port. Fixed to use restic's `-o sftp.command=...` override
    (an explicit `ssh -p <port> ...` invocation) whenever a non-default port
    or a specific key file is configured, which is also needed for a key
    file in the first place (restic's plain form has no syntax for either).
11. **That same `sftp.command` override broke on Windows-style backslash
    paths**: restic parses the option value with POSIX shell word-splitting,
    where backslash is an escape character, so a raw `C:\keys\id_ed25519`
    key-file path got silently mangled and ssh tried to resolve a path
    fragment as a hostname. Found manually while validating the fix above
    against a real SFTP container from a Windows shell. Fixed by converting
    the key-file path to forward slashes (`filepath.ToSlash`) before
    inserting it into the override string; Windows OpenSSH accepts forward
    slashes identically.
12. **An SFTP connection to a host whose key isn't yet trusted could hang
    for minutes** (observed: ~4 minutes) before ssh gave up on its own,
    because nothing told ssh to fail fast instead of waiting for an
    interactive prompt that can never come when restic runs it unattended.
    This would hang a scheduled hourly backup. Fixed by always including
    `-o BatchMode=yes` in the constructed `sftp.command`.
13. **`internal/backend.Probe` had no timeout at all**, so a capability test
    against a slow or hung destination could block `abm storage add`
    indefinitely. Fixed by giving the probe's `restic.Runner` a 2-minute
    timeout.

## Tests that could NOT be performed, and why

- **Windows VSS actually engaging** (an elevated, scheduled backup of a
  genuinely open/locked file) was not verified in this sandboxed development
  environment, which runs unelevated; `canUseFSSnapshot`'s elevation check
  itself was exercised (confirmed `false` when unelevated, triggering the
  documented warning), but the elevated code path through actual VSS
  shadow-copy creation was not. This needs real-world testing on an
  administrator-run scheduled task.
- **Real OAuth cloud-drive provider round-trips** (Google Drive, OneDrive,
  Dropbox, Box, pCloud, MEGA, Jottacloud, iCloud Drive, Proton Drive) were
  not tested — every one of them requires either a real account and a human
  OAuth approval, or (MEGA) a real account's credentials, neither of which
  is something this project can safely automate or was given access to. The
  provider registry metadata, field schemas, and rclone remote-registration
  code path are implemented and unit-tested in isolation, but have no
  network-backed integration test. This is why their maturity is labeled
  `stable`/`supported` (implemented, matches upstream rclone backend
  maturity) rather than independently validated by this project against a
  real account — see the provider matrix in the project status report.
- **S3 and SFTP, by contrast, were validated against real protocol servers**
  (see above) — restic's native backends for both are exercised for real,
  including the exact bugs that only a real server (not a unit test)
  surfaces. Every other object-storage preset (Backblaze B2, Wasabi,
  Cloudflare R2, Hetzner, DigitalOcean Spaces, IDrive e2, Storj, MEGA S4,
  Azure Blob, Google Cloud Storage, OpenStack Swift) shares the exact same
  `internal/backend` S3/Azure/GS/Swift construction code already proven
  against s3mock, but each provider's *specific* real endpoint/account was
  not individually tested, since that requires a real account per provider.
- **SQLite dump/restore was validated for real**: `internal/database`'s
  `TestDumpSQLite_RealCreateBackupDestroyRestoreVerify` creates a real
  on-disk SQLite database with a table and rows via the `sqlite3` CLI, backs
  it up through ABM's actual `Dump` code (SQLite's own `.backup` mechanism,
  not a raw file copy), destroys the original, restores the dump in its
  place, and verifies every row survived with correct values -- the exact
  create→backup→destroy→restore→validate cycle the project's release gates
  call for. CI installs `sqlite3` so this runs on every push.
- **MySQL/PostgreSQL dump/restore was not run end-to-end.** Docker Desktop's
  engine became available partway through this phase of development and was
  used for the S3/SFTP protocol tests above, but standing up disposable
  MySQL/PostgreSQL containers *and* installing `mysqldump`/`pg_dump` client
  binaries on this Windows development machine (neither ships with the
  server-only containers; a client needs its own install) was not completed
  in the remaining time. `internal/database`'s MySQL/PostgreSQL dump
  functions are implemented and use the documented safe flags
  (`--single-transaction --routines --triggers --events` /
  `--format=custom`) but have no automated test exercising a real server.
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

- No fully interactive, single-flow setup wizard chaining provider category
  → provider → auth → job → first backup → test restore → schedule in one
  guided terminal session; `abm setup` bootstraps device identity/config and
  `abm storage providers`/`abm storage add`/`abm job add` are separate,
  scriptable steps. The spec's 16-step wizard concept is not implemented as
  one flow.
- No interactive restore browser (choose job → browse snapshot contents →
  preview → restore); `abm snapshots` + `abm restore --include` cover the
  same ground non-interactively.
- No automated disaster-recovery inventory command; see
  [DISASTER-RECOVERY.md](DISASTER-RECOVERY.md) for the manual commands to
  run instead.
- No automated database/Docker-bind-mount auto-detection.
- `rest-server --append-only` (restic's own recommended separate-maintenance-
  authority design for ransomware resistance) is documented as the
  recommended hardened architecture in [IMMUTABILITY.md](docs/IMMUTABILITY.md)
  but is not wired into the provider registry as a first-class destination;
  reaching it today requires the generic rclone/manual-restic path outside
  ABM's normal flow.
- Object Lock is supported as advisory metadata (`abm storage add
  --immutable`) but ABM does not configure a bucket's Object Lock settings
  itself, nor does it implement any special pruning behavior for a locked
  destination -- see [IMMUTABILITY.md](docs/IMMUTABILITY.md) for the real
  operational tradeoff this involves.
- `abm storage reconnect` re-runs `rclone config reconnect`, which depends
  on the installed rclone version's own reconnect support for that backend;
  not independently verified against a real expired token for any provider.
