# Auto-Backup-Manager

A standalone, cross-platform (Ubuntu/Linux + Windows) backup manager built on
[restic](https://restic.net) (encryption, deduplication, snapshots) and
[rclone](https://rclone.org) (cloud transport). It runs independently of any
hosting control panel, backs up on an hourly schedule, keeps 10 days of
hourly recovery points by default, and survives reboots and temporary
network/cloud outages.

> **Status:** early foundation (pre-1.0). The core backup/restore/retention
> engine is implemented and tested end to end (see
> [docs/TESTING.md](docs/TESTING.md)); provider OAuth flows, the full
> interactive setup wizard, and real-world multi-day production soak testing
> are not yet complete. See the final report in the project history for exact
> status.

## Architecture

```
                     ┌─────────────────────────┐
                     │          abm             │   single Go binary,
                     │  (cmd/abm + internal/*)  │   same code on Linux/Windows
                     └────────────┬────────────┘
                                  │
            ┌─────────────────────┼─────────────────────┐
            ▼                     ▼                     ▼
      ┌───────────┐        ┌────────────┐        ┌─────────────┐
      │  restic   │        │   rclone   │        │  mysqldump / │
      │ snapshot, │◄──────►│  transport │        │  pg_dump /   │
      │ encrypt,  │        │  + OAuth   │        │  sqlite3     │
      │ dedupe    │        └─────┬──────┘        └──────────────┘
      └─────┬─────┘              │
            │                    ▼
            │       Google Drive / OneDrive / Dropbox /
            │       S3-compatible / SFTP / local disk
            ▼
   restic repository, scoped per job:
   <destination>/<organization>/<device-id>/<job-name>
```

Scheduling is OS-native, not a resident daemon: a systemd timer on Linux, a
Task Scheduler task (running as SYSTEM) on Windows. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Install

**Linux (Ubuntu 20.04+):**

```bash
curl -fsSL https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.sh | sudo bash
```

**Windows (elevated PowerShell):**

```powershell
irm https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.ps1 | iex
```

Both scripts download pinned, checksum-verified restic/rclone releases (see
`install.sh`/`install.ps1` for the exact versions), are idempotent, and never
touch an existing config, secrets, or backup repository. See
[docs/UBUNTU.md](docs/UBUNTU.md) and [docs/WINDOWS.md](docs/WINDOWS.md) for
the safer download-then-inspect install method and platform specifics.

## Quick start

```bash
abm setup
abm storage add --type s3 --name backblaze --endpoint <url> --access-key <key> --secret-key <secret>
abm job add --name my-job --source /var/www --destination backblaze
abm backup now my-job
abm snapshots my-job
abm schedule set
```

## Commands

| Command | Purpose |
|---|---|
| `abm setup` | First-run device identity + config bootstrap |
| `abm storage add/list/test` | Manage cloud/local destinations |
| `abm job add/list/edit/remove` | Manage backup jobs |
| `abm backup now [job\|--all]` | Run a backup immediately |
| `abm maintain [job\|--all] [--prune]` | Apply retention policy |
| `abm snapshots [job]` | List recoverable snapshots |
| `abm restore <job> <snapshot\|latest> --target DIR` | Restore to a safe target directory |
| `abm check [job]` | Repository integrity check |
| `abm status` | Last backup status per job |
| `abm doctor` | Full environment/config diagnostics |
| `abm schedule show/set` | Manage the hourly scheduler |
| `abm logs` | Recent log lines |
| `abm uninstall` | Remove the scheduler only (never repositories) |

## Documentation

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — design and package layout
- [SECURITY.md](docs/SECURITY.md) — secret handling, encryption
- [THREAT-MODEL.md](docs/THREAT-MODEL.md) — what this does and doesn't protect against
- [GOOGLE-DRIVE.md](docs/GOOGLE-DRIVE.md), [ONEDRIVE.md](docs/ONEDRIVE.md), [DROPBOX.md](docs/DROPBOX.md), [S3.md](docs/S3.md), [SFTP.md](docs/SFTP.md) — per-provider setup
- [UBUNTU.md](docs/UBUNTU.md), [WINDOWS.md](docs/WINDOWS.md) — platform-specific install/operation
- [DATABASES.md](docs/DATABASES.md) — MySQL/PostgreSQL/SQLite-safe backup
- [RESTORE.md](docs/RESTORE.md) — restore workflows and safety rules
- [DISASTER-RECOVERY.md](docs/DISASTER-RECOVERY.md) — rebuilding a lost machine
- [TESTING.md](docs/TESTING.md) — what has and hasn't been tested, and how
- [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — common problems

## License

Not yet specified; treat as all-rights-reserved until a LICENSE file is added.
