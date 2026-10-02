# Restore

Restore is implemented in `internal/restic.Restore` and exposed as
`abm restore`. It is deliberately as important and as tested as backup
itself — see [TESTING.md](TESTING.md) for the acceptance test that exercises
exactly this.

## Basic usage

```bash
abm snapshots my-job                                   # list recoverable points, "latest" marked
abm restore my-job latest --target /safe/restore/dir   # restore the newest successful snapshot
abm restore my-job a1b2c3d4 --target /safe/restore/dir # restore a specific snapshot by its short ID
abm restore my-job latest --target /tmp/x --include /var/www/html  # restore only a subpath
```

## Where restored files land

restic restores a snapshot's full **absolute** path under `--target`. If a
job's source was `/var/www`, restoring to `--target /restore` produces files
at `/restore/var/www/...`, not `/restore/...`. On Windows, the drive letter
becomes a folder: a source of `C:\CompanyData` restored to `--target C:\restore`
lands at `C:\restore\C\CompanyData\...`. This is restic's own behavior, not
something Auto-Backup-Manager changes; design job sources at a shallow,
intentional path (e.g. `/var/www`, `C:\CompanyData`) so restored layouts stay
predictable.

## Safety rules

- **`--target` is required** unless you explicitly pass `--in-place`.
- **`--in-place` requires confirmation** (an interactive `y/N` prompt, or
  `--yes` to skip it non-interactively) because it restores directly over
  the job's original source paths, overwriting whatever is there now. On
  Windows, ABM first performs Restic's verified restore into a private staging
  directory, then copies only the configured source trees back to their true
  drive locations. This prevents Restic's drive component from creating an
  incorrect nested path such as `C:\C\CompanyData` and also supports jobs
  whose sources span multiple drives.
- Restoring never requires "undoing" a newer snapshot — every snapshot is
  independently restorable at any time, because backups are additive (see
  [ARCHITECTURE.md](ARCHITECTURE.md)).

## Database restore

Auto-Backup-Manager restores the database **dump file** (the `.sql`/`.dump`/
`.sqlite3` file captured at backup time) via the normal file-restore path
above. Loading that dump back into a live database server is a deliberate
manual step, not automated, specifically so a restore can never silently
replace a live production database:

```bash
abm restore my-job latest --target /tmp/db-restore --include "*/myapp.sql"
# review, then explicitly:
mysql myapp_recovery < /tmp/db-restore/.../myapp.sql     # into a new/temp database, or
mysql myapp < /tmp/db-restore/.../myapp.sql              # into the live database, with the live service stopped
```

Prefer restoring into a new/temporary database first and validating before
ever pointing an application at a restored database.

## Graphical restore

The local GUI provides the normal restore workflow: choose a backup and
recovery point, browse its contents, optionally select paths, and restore to a
new safe directory. Advanced restore to original locations requires both the
warning confirmation and typing `RESTORE ORIGINAL`.
