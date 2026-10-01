# Troubleshooting

Start with `abm doctor` — it checks restic/rclone availability, config
validity, source path existence, scheduler status, VSS (Windows), and any
database dump tools a configured job actually needs.

## "unable to locate cache directory" (restic)

Restic needs `HOME` (Linux) or `LOCALAPPDATA` (Windows) to locate its local
cache. This should never happen under the installed scheduler (systemd/Task
Scheduler both provide a normal environment), but if you see it running
`abm` manually from a stripped-down shell/service account, ensure that
environment variable is set.

## Installer fails with a raw GitHub API 404 ("Invoke-RestMethod ... Not Found")

Fixed as of the version of `install.ps1`/`install.sh` that ships with this
doc. The old installer logic asked GitHub's `/releases/latest` API for the
`abm` binary, which 404s outright for a repository with no stable release
yet -- true for this entire pre-1.0 phase -- and would keep failing even
after the first release, since `/releases/latest` never returns a
prerelease either. Both installers now resolve the exact approved version
from [RELEASE.json](../RELEASE.json) in this repo instead, which always
points at a real, published release (currently a pre-1.0 release
candidate). If you still hit this, you're running an old copy of the
installer script -- re-download it fresh rather than using a cached copy.

## "No Auto-Backup-Manager release is available for this channel."

This is the installer's own controlled failure message (never a raw GitHub
API response) for when version resolution genuinely can't find a release --
e.g. `RELEASE.json` is unreachable, or you pinned `ABM_VERSION`/`-AbmVersion`
to a tag that was never published. Check
[the repository's releases page](https://github.com/Humran13/Auto-Backup-Manager/releases)
for the exact tags that actually exist.

## "Access is denied" / "VSS error: ... E_ACCESSDENIED" (Windows backup)

Expected when running `abm backup now` from a **non-elevated** PowerShell:
VSS requires administrator privileges. The backup still completes (plain,
non-VSS), with a warning in `abm status`/logs. Scheduled backups run as
SYSTEM and are always elevated, so this does not affect them. To get VSS on
a manual run, use an elevated PowerShell prompt.

## "Access is denied" running `abm schedule set`

Task Scheduler registration requires an elevated PowerShell prompt. Re-run
from "Run as Administrator".

## A backup fails right after a destination outage, but previously worked

This is correct behavior, not a bug: once a job's repository has been
initialized, Auto-Backup-Manager never silently re-creates it. If a
destination is temporarily unreachable, the backup fails loudly (as it
should) and the next scheduled run retries automatically — no history is
lost. Check `abm storage test <name>` and your network/credentials.

## "repository does not exist" when you believe it should

If this is a brand-new job pointed at a brand-new path, this is expected —
the first successful run creates it. If you expected an *existing*
repository (e.g. recovering a job on a replacement machine), make sure
`--repository-path` exactly matches the original `org/device-id/job-name`
path and that you used `abm job add --recover-existing` with the correct
password — see [DISASTER-RECOVERY.md](DISASTER-RECOVERY.md).

## Restored files aren't where I expected

restic restores a snapshot's full **absolute** source path under `--target`
(e.g. a Linux source of `/var/www` restored to `--target /restore` lands at
`/restore/var/www/...`; on Windows, `C:\Data` restored to `C:\restore` lands
at `C:\restore\C\Data\...`). See [RESTORE.md](RESTORE.md).

## `abm doctor` reports a database tool missing

Only a problem if a configured job actually uses that database kind — check
`config.yaml`'s `databases:` entries for the affected job. If no job needs
it, this check reports healthy and is informational only.

## Logs

```bash
abm logs --tail 200          # recent lines from the shared log file
journalctl -u auto-backup-manager    # Linux: scheduled-run output
# Windows: Task Scheduler's own History tab for \Auto-Backup-Manager\HourlyBackup
```

Secrets are redacted from all of the above (`internal/secrets.Redact`); if
you ever see a plaintext password/token in a log line, treat it as a bug and
report it (see [SECURITY.md](SECURITY.md)).
