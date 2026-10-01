# Windows

## Supported versions

Windows 10, Windows 11, and Windows Server versions able to run restic and
rclone (effectively any still-supported Server release).

## Install

From an **elevated** PowerShell prompt:

```powershell
irm https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.ps1 | iex
```

Safer alternative — download, read, then run:

```powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.ps1 -OutFile install.ps1
Get-Content install.ps1 | more
.\install.ps1
```

This installs pinned, checksum-verified restic/rclone builds plus `abm.exe`
to `C:\Program Files\Auto-Backup-Manager`, adds that directory to the
machine `PATH`, and creates under `C:\ProgramData\Auto-Backup-Manager\`:
`config.yaml`, `secrets\` (DPAPI-protected), `rclone.conf`, `state\`,
`locks\`, `dumps\`, `logs\`.

## Scheduling

`abm schedule set` (must be run elevated) registers two Task Scheduler
tasks under `\Auto-Backup-Manager\`:

- **HourlyBackup** — hourly, `StartWhenAvailable=true` (a backup missed
  because the PC was off/asleep runs as soon as it's back), runs as
  `SYSTEM` so it works with nobody logged in, `MultipleInstancesPolicy=
  IgnoreNew` to prevent overlap, `RunLevel=HighestAvailable` for VSS.
- **DailyMaintain** — daily retention/prune.

Check status with `abm schedule show` or Task Scheduler's own UI/history.

## Volume Shadow Copy Service (VSS)

VSS lets restic back up open/locked files (a database file, an open Office
document) without requiring the user to close them. Auto-Backup-Manager uses
`restic --use-fs-snapshot` only when the running process is **elevated**
(`internal/job.canUseFSSnapshot`, checked via the process token):

- The scheduled Task Scheduler run (runs as SYSTEM) is always elevated, so
  scheduled hourly backups always get VSS.
- A manual `abm backup now` from a non-elevated shell degrades to a plain
  backup and reports a clear warning (`LastWarning` in `abm status`,
  logged at WARN level) rather than silently skipping consistency or
  failing outright.

`abm doctor` reports whether the VSS service itself is available (present
and not disabled; VSS is normally `STOPPED` until something needs it, which
is healthy, not a failure).

## Permissions

Secrets are protected with `CRYPTPROTECT_LOCAL_MACHINE` DPAPI, decryptable by
any admin process on the machine (matching an unattended SYSTEM-run backup).
`install.ps1` additionally locks down `C:\ProgramData\Auto-Backup-Manager`
(which holds `rclone.conf` and the secrets directory) with NTFS ACLs
restricting access to `SYSTEM` and `Administrators` only, replacing the
default, more permissive inherited `ProgramData` permissions.
