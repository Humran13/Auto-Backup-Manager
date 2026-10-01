# Ubuntu / Linux

## Supported versions

Targeted: Ubuntu 20.04, 22.04, 24.04, and 26.04 (once published). Ubuntu
18.04 is end-of-life and its `apt` Go toolchain is too old to build this
project from source; the project doesn't exclude it from running a
pre-built release binary, but it is not part of automated CI coverage (see
[TESTING.md](TESTING.md)).

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.sh | sudo bash
```

Safer alternative — download, read, then run:

```bash
curl -fsSLO https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.sh
less install.sh
sudo bash install.sh
```

This installs pinned, checksum-verified restic/rclone builds plus the `abm`
binary to `/usr/local/bin`, and creates:

| Path | Purpose |
|---|---|
| `/etc/auto-backup-manager/config.yaml` | Configuration (no secrets) |
| `/etc/auto-backup-manager/secrets/` | Repository passwords, DB credentials (mode `0600`) |
| `/etc/auto-backup-manager/rclone.conf` | Cloud remote config/tokens (mode `0600`) |
| `/var/lib/auto-backup-manager/` | Locks, temp DB dumps, status |
| `/var/log/auto-backup-manager/` | Logs |

## Scheduling

`abm schedule set` installs:

- `auto-backup-manager.timer` — hourly, `Persistent=true` (missed runs while
  the VPS was off execute once at the next boot), `RandomizedDelaySec=120`
  to avoid thundering-herd load if many devices share a schedule.
- `auto-backup-manager-maintain.timer` — daily retention/prune.

Both run `Type=oneshot` units, so there is no resident daemon. Check status
with `systemctl status auto-backup-manager.timer` or `abm schedule show`;
view logs with `journalctl -u auto-backup-manager` or `abm logs`.

## Permissions

Everything under `/etc/auto-backup-manager` runs as root by design — backups
commonly need to read files across multiple users' home directories and
system config. Run `abm` itself via `sudo` for any command that reads
config/secrets.

## Doctor

```bash
abm doctor
```

Checks restic/rclone availability, config validity, source path existence,
scheduler status, and (if any job uses one) the required database dump
tool's presence.
