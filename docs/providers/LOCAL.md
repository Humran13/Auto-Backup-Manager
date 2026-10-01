# Local / External Disk

| | |
|---|---|
| **Provider ID** | `local` |
| **Maturity** | Stable |
| **Backend** | restic's native `local` backend |
| **Auth** | None |
| **Headless** | Direct |

## Setup

```bash
# Linux: a mounted path
abm storage add --provider local --name usb --path /mnt/backup

# Windows: a drive or folder
abm storage add --provider local --name usb --path D:\Backups
```

## Behavior when the disk is disconnected

ABM detects an unavailable path at backup time and fails the run loudly —
it never reports a backup as successful when the destination path doesn't
exist or isn't writable. The next scheduled run retries automatically once
the disk is reconnected.

## Security considerations

**A permanently attached local or external disk is not sufficient as the
only protection against ransomware, fire, theft, or total server/PC loss.**
If it's connected when the machine is compromised or destroyed, it's exposed
to the same event. Use it as a fast local recovery tier alongside an
off-site destination, not as your only backup.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
diff -r /original/source /tmp/restore-test/original/source
```
