# Security Model

## Encryption

All cloud backups are encrypted client-side by restic (AES-256 + Poly1305)
before any data leaves the machine. Auto-Backup-Manager never implements its
own cryptography; it only generates and stores the repository password.

## Where secrets live

| Secret | Linux | Windows |
|---|---|---|
| Restic repository password | `/etc/auto-backup-manager/secrets/restic-password-<job>`, mode `0600`, owned by root | DPAPI-protected (`CRYPTPROTECT_LOCAL_MACHINE`) under `C:\ProgramData\Auto-Backup-Manager\secrets` |
| Database credentials | same secrets directory, `<credentials_ref>` key | same DPAPI store |
| rclone OAuth tokens / S3 / SFTP credentials | `/etc/auto-backup-manager/rclone.conf`, mode `0600` | `C:\ProgramData\Auto-Backup-Manager\rclone.conf`, protected by NTFS ACLs limited to Administrators/SYSTEM |

Secrets are never:
- committed to this Git repository (see `.gitignore` and the CI secret-scan job),
- passed as command-line arguments (which would appear in `ps`/process listings),
- written to `config.yaml` (its schema, `internal/config.Config`, has no field capable of holding one),
- logged in plaintext — `internal/secrets.Redact` runs over every log line
  (`internal/logging`) before it reaches disk, masking anything shaped like a
  `password=`/`token=`/`secret_key=` assignment or a `user:pass@host` URL.

MySQL credentials are passed to `mysqldump` via a temporary
`--defaults-extra-file` (not `--password=` or `MYSQL_PWD`), and PostgreSQL
credentials via `PGPASSWORD` scoped to that one child process's environment —
both specifically to avoid the password appearing in `ps`. Temporary database
dump files are overwritten with zeros before deletion
(`internal/database.secureRemove`).

## Repository isolation

Every job gets its own restic repository and its own password
(`restic-password-<job>`). A leaked password for one job's repository never
exposes another job's, or another device's, history — see
[ARCHITECTURE.md](ARCHITECTURE.md) for the repository path layout.

## Locking down what runs

- Scheduled backups run via systemd (`Type=oneshot`, locked down with `Nice`/`IOScheduling`) or Task Scheduler (as `SYSTEM`, `RunLevel=HighestAvailable` for VSS).
- `internal/lock` prevents two backup runs for the same job from executing concurrently, using a PID-file lock that correctly reclaims a stale lock left by a crashed process (checked via `kill -0` on Linux, `OpenProcess`+exit-code on Windows) rather than refusing to run forever.
- `abm uninstall` removes only the scheduler entry. It never deletes config, secrets, or a backup repository — there is no code path in this project that deletes a remote repository at all.

## What this does *not* protect against

See [THREAT-MODEL.md](THREAT-MODEL.md) for the full breakdown, but
importantly: **standard OAuth-based Google Drive/OneDrive/Dropbox storage is
not immutable/WORM storage.** Credentials stolen from the machine (or a
compromised rclone token) can delete or encrypt those backups just like any
other file in that account. For ransomware/stolen-credential resilience, use
an Object-Lock-capable S3-compatible destination (Backblaze B2, Wasabi, AWS
S3) as described in [providers/S3.md](providers/S3.md) and [IMMUTABILITY.md](IMMUTABILITY.md).

## Reporting a vulnerability

This is an early-stage internal project without a public disclosure process
yet; report issues directly to the repository owner.
