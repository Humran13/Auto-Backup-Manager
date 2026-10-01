# SFTP / Storage Server

Generic SFTP destination. Documented here using
[Hetzner Storage Box](https://www.hetzner.com/storage/storage-box/) as a
concrete example, but any SSH server with SFTP works identically.

## What you need

- Hostname and port (Hetzner Storage Box: `<id>.your-storagebox.de`, port `23`)
- Username
- An SSH private key (**preferred**) or a password (supported, but
  discouraged — see [SECURITY.md](SECURITY.md))

## Setting it up (SSH key, recommended)

```bash
abm storage add --type sftp --name storagebox \
    --host u123456.your-storagebox.de --port 23 \
    --user u123456 --key-file /etc/auto-backup-manager/keys/storagebox_ed25519
```

The private key file should be `chmod 600`, owned by root, and itself never
committed to source control.

## Setting it up (password — discouraged)

A password can be configured through `rclone config` directly
(`internal/rclone.CreateSFTPRemote` obscures it with rclone's own
`rclone obscure`, which is reversible and not a substitute for real
encryption — treat an SFTP password the same as any other secret, protected
only by the OS permissions on `rclone.conf`).

## Testing

```bash
abm storage test storagebox
```

## Notes

- SFTP destinations do not support Object Lock/immutability; if ransomware
  resilience against a compromised machine matters, prefer an S3-compatible
  destination with Object Lock instead (see [S3.md](S3.md)).
- Some storage-box-style providers enforce per-connection bandwidth/IOPS
  limits; large initial backups may be slower than a cloud object store.
