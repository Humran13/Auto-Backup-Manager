# Architecture

## Why Go + restic + rclone

A single Go binary runs unmodified on Linux and Windows, with no runtime
dependency (Python/Node/JVM) beyond the two external tools it shells out to:

- **restic** does all snapshotting, content-defined deduplication, and
  client-side AES-256 encryption. Auto-Backup-Manager never implements its
  own crypto or chunking — that would be a serious, unnecessary risk.
- **rclone** is used only as restic's data-transport backend
  (`rclone:<remote>:<path>` repository spec) and, separately, as the thin
  wrapper this project uses to create/test named remotes and run OAuth flows
  for Google Drive/OneDrive/Dropbox. Auto-Backup-Manager does not implement
  its own OAuth client; it drives rclone's.

OS-specific behavior (scheduling, VSS, service accounts, file permissions) is
isolated behind Go build tags (`_windows.go` / `_unix.go` / `_linux.go`)
rather than branching at runtime wherever possible, so each platform's code
path is simple to audit independently.

## Package layout

```
cmd/abm/                 CLI entry point and command wiring (cobra)
internal/config/         YAML schema, validation, schema migration
internal/job/            Orchestrates one backup run: lock -> db dump -> restic backup -> verify -> status
internal/restic/         restic CLI wrapper (init/backup/snapshots/forget/restore/check)
internal/rclone/         rclone CLI wrapper (remote config, OAuth, connectivity test)
internal/retention/      Translates a retention policy into restic forget args; pure-Go simulator for tests
internal/database/       mysqldump/pg_dump/sqlite3-based safe dump hooks
internal/lock/           Cross-run file lock (prevents overlapping backups)
internal/secrets/        Secret storage (Linux: 0600 files; Windows: DPAPI) + log redaction
internal/scheduler/      systemd unit/timer generation (Linux); Task Scheduler XML (Windows)
internal/doctor/         Environment/config diagnostics behind `abm doctor`
internal/deviceid/       Stable per-machine device identifier
internal/paths/          Centralized standard install/config/state paths per OS
internal/logging/        Structured (slog) logging with secret redaction
test/integration/        End-to-end tests against a real restic binary
```

## Repository layout on the destination

```
<destination>/<organization>/<device-id>/<job-name>
```

Each job gets its own restic repository (its own encryption password), so a
compromised job's password never exposes another job's — or another
machine's — history. `device-id` is a random identifier generated once and
persisted locally (`internal/deviceid`); it is stable for the life of the
machine and is what prevents unrelated computers from ever being combined
under one shared, uncontrolled path.

## The backup model: additive, never destructive

Every `abm backup now` run is a plain `restic backup`: it adds a new snapshot
on top of the existing repository history. It never deletes or overwrites a
prior snapshot. "Latest" is resolved purely by comparing snapshot timestamps
(`internal/restic.Latest`) — there is no special "latest" pointer to
corrupt, and restoring an old snapshot never requires undoing newer ones.

Retention (`internal/retention`) is enforced by a *separate* step,
`abm maintain`, which runs `restic forget` (and, with `--prune`, reclaims
space). The default policy is `--keep-within-hourly 240h` — the most recent
snapshot in every 1-hour bucket over the last 10 days — matching the "~240
hourly restore points" requirement. Forgetting is scheduled daily, not after
every hourly backup, to avoid needless cloud load from frequent pruning.

## Repository initialization safety

A repository is only ever initialized (`restic init`) once per job, tracked
by a local `Initialized` flag in that job's status file
(`internal/job.Status`). After the first successful init, Auto-Backup-Manager
never calls `restic init` again automatically. This matters: an earlier
design mistake (fixed during development, see `test/integration` and the
project's test history) checked "does the repository exist?" before every
run and silently re-initialized an empty repository whenever that check
failed — which is exactly what happens when a destination is just
temporarily unreachable, not actually missing. The fixed design means a truly
unreachable destination now fails the backup loudly, as it must, instead of
quietly discarding history under a fresh empty repository at the same path.

## Status and verification

A backup is only ever reported successful when restic's own terminal
`"summary"` JSON message confirms a snapshot ID
(`internal/restic.parseBackupSummary`). Any other outcome — a crash, a killed
process, a truncated run — is treated as a failure. Status is persisted to a
per-job JSON file (`internal/job.Status`) that `abm status`/`abm doctor` read
directly; it is updated exactly once per run, after the real outcome is
known, never optimistically beforehand.
