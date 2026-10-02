# Auto-Backup-Manager

A standalone, cross-platform (Ubuntu/Linux + Windows) backup manager built on
[restic](https://restic.net) (encryption, deduplication, snapshots) and
[rclone](https://rclone.org) (cloud transport for providers with no
restic-native backend). It runs independently of any hosting control panel,
backs up on an hourly schedule, keeps 10 days of hourly recovery points by
default, supports 25+ storage providers through a scalable provider
registry, and survives reboots and temporary network/cloud outages.

> **Status:** pre-1.0 release candidate. The selective backup/restore/retention/
> multi-destination engine and graphical onboarding are implemented and tested end to end, including
> real protocol-level round-trips against S3 and SFTP test servers (see
> [docs/TESTING.md](docs/TESTING.md)). Most cloud providers are implemented
> and documented but not yet validated against a real account (they are
> labeled accordingly — run `abm storage providers` to see exact maturity).
> No v1.0.0 has been tagged; the install commands below resolve the current
> pre-1.0 **release candidate** (see [RELEASE.json](RELEASE.json)), never a
> stable release that doesn't exist yet.

## Architecture

```
                     ┌─────────────────────────┐
                     │          abm             │   single Go binary,
                     │  (cmd/abm + internal/*)  │   same code on Linux/Windows
                     └────────────┬────────────┘
                                  │
                       internal/provider (registry)
                       internal/backend (repository construction)
                                  │
            ┌─────────────────────┼─────────────────────┐
            ▼                     ▼                     ▼
      ┌───────────┐        ┌────────────┐        ┌──────────────┐
      │  restic   │        │   rclone   │        │  mysqldump / │
      │ snapshot, │◄──────►│  (cloud-   │        │  pg_dump /   │
      │ encrypt,  │        │  drive     │        │  sqlite3     │
      │ dedupe    │        │  providers │        └──────────────┘
      └─────┬─────┘        │  only)     │
            │              └─────┬──────┘
            │                    │
            ▼                    ▼
   restic native backends:   rclone remotes:
   local, sftp, s3, azure,   Google Drive, OneDrive, Dropbox, Box,
   gs, swift                 pCloud, MEGA, Jottacloud, iCloud, Proton

   restic repository, scoped per (job, destination):
   <destination>/<organization>/<device-id>/<job-name>
```

Adding a provider that reuses an existing backend (another S3-compatible
preset, another rclone remote) is a registry entry in
`internal/provider/registry.go` — no change to backup, retention,
scheduling, or restore logic. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

Backup scheduling is OS-native: a systemd timer on Linux and a Task Scheduler
task (running as SYSTEM) on Windows. A separate localhost-only GUI service is
started at boot so ordinary configuration remains graphical.

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
touch an existing config, secrets, or backup repository. Neither script ever
queries GitHub's `/releases/latest` API (it 404s for a repo with no stable
release, and never returns a prerelease even once one exists) -- they
resolve the exact approved version from [RELEASE.json](RELEASE.json)
instead. To pin a specific version yourself: `ABM_VERSION=v0.9.2-rc.1 curl
... | sudo bash` (Linux) or `irm ... -OutFile install.ps1; .\install.ps1
-AbmVersion v0.9.2-rc.1` (Windows, since piping to `iex` can't pass
parameters). See [docs/UBUNTU.md](docs/UBUNTU.md) and
[docs/WINDOWS.md](docs/WINDOWS.md) for
the safer download-then-inspect install method and platform specifics. The
installer starts the local GUI automatically. Windows opens the setup wizard;
Linux starts a localhost-only systemd service and prints the secure SSH-tunnel
access instruction for a VPS. No `abm setup`, `abm gui`, or `rclone config`
command is part of the normal setup path.

## Quick start

Run the one-line installer above, then use the browser wizard to name the
organization/device, select multiple server folders, connect storage, choose
retention, test the destination, run the first backup, and verify a recovery
point. Advanced CLI commands remain available for automation but are not
required for ordinary setup.

## Web GUI

A local web interface is the normal product workflow — same engine underneath,
not a separate implementation:

```bash
abm gui
```

The installer starts it at `http://127.0.0.1:8765` (and opens it on Windows).
If it's your first run, the complete setup wizard appears automatically.
The GUI binds to `127.0.0.1` only — it is never exposed on a public or LAN
interface — and every operation it performs calls the exact same Go
functions the CLI commands do (`internal/job`, `internal/backend`,
`internal/provider`, `internal/doctor`, `internal/scheduler`): nothing is
possible through the GUI that isn't also possible through the CLI, and
nothing shells out to `abm` itself. See
[docs/GUI.md](docs/GUI.md) for the full page-by-page walkthrough and current
limitations. Google Drive, OneDrive, and Dropbox authorization is orchestrated
inside the GUI; normal users do not run `rclone config`.

```
abm gui --port 9000      # use a different port
abm gui --no-open        # print the URL only, don't launch a browser
```

The CLI remains fully supported and is the better fit for scripting/CI;
nothing in this project requires the GUI.

## Commands

| Command | Purpose |
|---|---|
| `abm setup` | First-run device identity + config bootstrap |
| `abm gui` | Start the local web GUI (127.0.0.1 only) |
| `abm storage providers` | List every supported provider and its maturity |
| `abm storage add/list/show/test/reconnect/remove` | Manage storage destinations |
| `abm job add/list/edit/remove/set-db-credentials` | Manage backup jobs |
| `abm backup now [job\|--all]` | Run a backup immediately |
| `abm maintain [job\|--all] [--prune]` | Apply retention policy |
| `abm snapshots [job] [--destination]` | List recoverable snapshots |
| `abm restore <job> <snapshot\|latest> --target DIR` | Restore to a safe target directory |
| `abm check [job]` | Repository integrity check |
| `abm status` | Last backup status per job, per destination |
| `abm doctor` | Full environment/config diagnostics |
| `abm schedule show/set` | Manage the hourly scheduler |
| `abm logs` | Recent log lines |
| `abm uninstall` | Remove the scheduler only (never repositories) |

## Supported storage providers

25+ providers across cloud drives, object storage, SFTP, local disk, and a
generic rclone passthrough for anything else — see `abm storage providers`
for the live, evidence-based maturity of each, and
[docs/providers/](docs/providers/) for per-provider setup. Highlights:

| Family | Providers |
|---|---|
| Cloud drives | Google Drive, OneDrive, Dropbox, Box, pCloud, MEGA, Jottacloud, iCloud Drive*, Proton Drive* |
| Object storage | AWS S3, Backblaze B2, Wasabi, Cloudflare R2, Hetzner, DigitalOcean Spaces, IDrive e2, Storj, MEGA S4, MinIO, Azure Blob, Google Cloud Storage, OpenStack Swift, generic S3-compatible |
| Other | SFTP, local/external disk, any existing rclone remote |

\* marked **experimental** — see their docs for why.

## Documentation

- [GUI.md](docs/GUI.md) — web GUI pages, setup wizard, security, limitations
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — design and package layout
- [SECURITY.md](docs/SECURITY.md) — secret handling, encryption
- [THREAT-MODEL.md](docs/THREAT-MODEL.md) — what this does and doesn't protect against
- [IMMUTABILITY.md](docs/IMMUTABILITY.md) — ransomware resilience, Object Lock, append-only design
- [docs/providers/](docs/providers/) — setup for every storage provider
- [UBUNTU.md](docs/UBUNTU.md), [WINDOWS.md](docs/WINDOWS.md) — platform-specific install/operation
- [DATABASES.md](docs/DATABASES.md) — MySQL/PostgreSQL/SQLite-safe backup
- [RESTORE.md](docs/RESTORE.md) — restore workflows and safety rules
- [DISASTER-RECOVERY.md](docs/DISASTER-RECOVERY.md) — rebuilding a lost machine
- [TESTING.md](docs/TESTING.md) — what has and hasn't been tested, and how
- [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) — common problems

## License

Not yet specified; treat as all-rights-reserved until a LICENSE file is added.
