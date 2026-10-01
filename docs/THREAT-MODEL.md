# Threat Model

| Threat | Protected? | How |
|---|---|---|
| VPS/PC total loss (hardware failure, provider termination) | Yes | Full history lives in the remote repository, independent of the source machine. See [DISASTER-RECOVERY.md](DISASTER-RECOVERY.md). |
| Disk failure on the source machine | Yes | Same as above — the backup never depends on the source disk surviving. |
| Accidental deletion of files/data | Yes, within the retention window | Any of the last ~240 hourly snapshots (10 days) can be restored; see [RESTORE.md](RESTORE.md). Deleting a file locally never deletes it from prior snapshots. |
| Ransomware encrypting the live machine | Partially | Prior snapshots in the remote repository are unaffected *if* the ransomware never obtained the repository password or destination credentials. If it did (e.g. it ran as the same user/root), see "stolen credentials" below. |
| Stolen VPS/PC root/admin credentials | Partially | An attacker with root/admin can read `rclone.conf` and the restic password, and therefore could delete or corrupt the backup repository too. Mitigate with an **immutable/Object-Lock** destination (see [S3.md](S3.md)) where even the holder of valid credentials cannot delete objects within the lock period. |
| Stolen cloud credentials alone (not the machine) | Partially | Same mitigation: Object Lock is the only destination type in this project that resists deletion even by someone holding valid write credentials. Plain OAuth (Drive/OneDrive/Dropbox) and non-locked S3/SFTP do not. |
| Backup repository deletion (accidental or malicious) | Not prevented, only mitigated | Nothing in this project deletes a repository, but neither does anything stop another actor with valid destination credentials from deleting it (unless Object Lock is used). Multiple-destination support is designed for (see config schema) specifically so a second, differently-credentialed destination survives even if the primary is destroyed; only a single primary destination per job is implemented so far. |
| Repository corruption | Detected, not prevented | `abm check` (restic's structural + optional data-subset check) detects corruption. restic's deduplicated pack-file format means a single corrupted pack can affect multiple snapshots; keeping `abm check` on a regular schedule and retaining history across multiple time points is the mitigation. |
| Transient network/cloud outage during a scheduled backup | Yes | A failed run never reports success and never touches prior snapshots (see [ARCHITECTURE.md](ARCHITECTURE.md) on repository-initialization safety); the next hourly run retries automatically. |
| Two backups racing for the same job | Yes | `internal/lock` file-based locking with stale-lock reclaim. |
| Open/locked files on Windows (databases, Office documents) | Partially | VSS (`--use-fs-snapshot`) is used only when the process is elevated (always true for the scheduled Task Scheduler run, which executes as SYSTEM). An unelevated manual `abm backup now` degrades to a plain backup with a clearly reported warning rather than failing or silently skipping consistency. |
| Live database copied as a raw file | Prevented by design | Database backups always go through a logical dump (`mysqldump`/`pg_dump`/SQLite's own backup API) — see [DATABASES.md](DATABASES.md). There is no code path that backs up a live datadir directly. |
| Secrets leaking via logs/diagnostics | Mitigated | `internal/secrets.Redact` runs over all log output; see [SECURITY.md](SECURITY.md). |
| Secrets committed to the public/shared Git history | Mitigated | `.gitignore` excludes local secret-shaped files, and CI runs a secret scan (`.github/workflows/ci.yml`) on every push. |

## Explicitly out of scope for v1

- Bare-metal/full disk-image OS recovery (bootloader, every installed
  application). Disaster recovery restores selected files and database
  dumps and collects an inventory to assist manually rebuilding the OS —
  it does not recreate the OS itself. See [DISASTER-RECOVERY.md](DISASTER-RECOVERY.md).
- Protecting against an attacker who already has full root/admin on the
  source machine *and* valid write credentials to a non-immutable
  destination — at that point the backup itself is not relied upon as the
  defense; only an immutable destination (Object Lock) resists this.
