# SFTP / Storage Server

| | |
|---|---|
| **Provider ID** | `sftp` |
| **Maturity** | Stable |
| **Backend** | restic's native `sftp` backend (not rclone) |
| **Auth** | SSH key (preferred) or password |
| **Headless** | Direct — no browser involved at all |

Generic SFTP destination. Documented here using
[Hetzner Storage Box](https://www.hetzner.com/storage/storage-box/) as a
concrete example, but any SSH server with SFTP works identically — ABM
doesn't bind to one vendor.

## Why native, not rclone

SFTP is one of restic's oldest, most mature native backends. ABM talks to it
directly (`restic -r sftp:user@host:/path`, via the system `ssh`/`sftp`
client) rather than routing through rclone — one less moving part, and one
fewer thing to misconfigure.

## What you need

- Hostname and port (Hetzner Storage Box: `<id>.your-storagebox.de`, port `23`)
- Username
- An SSH private key (**preferred**) or a password (supported, but
  discouraged — see [../SECURITY.md](../SECURITY.md))
- The host must already be a known host for the account running `abm`
  (`ssh-keyscan`/a normal first `ssh` connection, or an SSH config entry) —
  ABM never silently disables host-key verification.

## Setting it up (SSH key, recommended)

```bash
abm storage add --provider sftp --name storagebox \
    --host u123456.your-storagebox.de --port 23 \
    --user u123456 --key-file /etc/auto-backup-manager/keys/storagebox_ed25519
```

The private key file should be `chmod 600`, owned by root, and itself never
committed to source control.

## Setting it up (password — discouraged)

```bash
abm storage add --provider sftp --name storagebox \
    --host u123456.your-storagebox.de --port 23 --user u123456
# prompts for the password on stdin, never a command-line argument
```

## Testing

```bash
abm storage test storagebox
```

## Reconnecting

SSH keys/passwords for SFTP don't expire the way OAuth tokens do; there is
nothing to "reconnect" unless the key is rotated or the password changes —
re-run `abm storage add` with the same `--name` to update it.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
diff -r /original/source /tmp/restore-test/original/source
```

## Known limitations

- SFTP destinations do not support Object Lock/immutability; if ransomware
  resilience against a compromised machine matters, prefer an S3-compatible
  destination with Object Lock instead, or restic's own `rest-server`
  `--append-only` mode — see [../IMMUTABILITY.md](../IMMUTABILITY.md).
- Some storage-box-style providers enforce per-connection bandwidth/IOPS
  limits; large initial backups may be slower than a cloud object store.
