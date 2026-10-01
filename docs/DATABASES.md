# Database Backups

**Never back up a live MySQL/PostgreSQL data directory as plain files.**
A database engine's on-disk files are only consistent while the engine
controls writes to them; copying them directly (or renaming a live datadir)
risks a backup that looks complete but is actually corrupt or
transactionally inconsistent. Auto-Backup-Manager has no code path that
backs up a raw datadir — every supported database kind goes through a safe
logical dump first (`internal/database.Dump`).

## Supported databases (v1)

| Kind | Tool used | Options |
|---|---|---|
| MySQL / Percona / MariaDB | `mysqldump` | `--single-transaction --routines --triggers --events --quick` |
| PostgreSQL | `pg_dump` | `--format=custom` |
| SQLite | `sqlite3 .backup` | SQLite's own online backup API, safe against concurrent writers |

## Configuring a database hook

```yaml
jobs:
  my-app:
    sources: ["/var/www/my-app"]
    databases:
      - kind: mysql
        name: my_app_db
        host: localhost
        credentials_ref: db-my-app     # resolved from the secret store at run time
```

Credentials are never written to `config.yaml`. Store them once:

```bash
abm job set-db-credentials db-my-app --username dbuser
# prompts for the password on stdin -- never a command-line argument
```

## What happens during a backup

```
pre-backup
   -> safe logical dump to a 0700 temp directory (internal/database.Dump)
   -> restic backs up the dump file alongside the job's normal sources
   -> restic confirms the snapshot (internal/restic.parseBackupSummary)
   -> the temp dump is overwritten with zeros and deleted (secureRemove),
      whether the backup succeeded or failed
```

Credentials never appear in process listings: MySQL credentials go through a
temporary `--defaults-extra-file`, and PostgreSQL's through `PGPASSWORD`
scoped to that one child process's environment only.

## Restoring a database

See [RESTORE.md](RESTORE.md#database-restore). The dump file is restored
like any other file; loading it back into a live database server is always
an explicit, separate manual step so a restore can never silently replace a
live production database.

## Auto-detection

Auto-detecting installed database engines (so the setup wizard can suggest
them automatically) is not implemented yet. Add database hooks to
`config.yaml` manually, as above.
