# Proton Drive — EXPERIMENTAL

| | |
|---|---|
| **Provider ID** | `proton-drive` |
| **Maturity** | **Experimental** |
| **Backend** | rclone `protondrive` (itself an experimental rclone backend) |
| **Auth** | Proton account credentials + 2FA, producing a session |
| **Headless** | Difficult |

**This provider is marked experimental for reasons outside ABM's control:**
rclone's Proton Drive support is built against a reverse-engineered,
non-public API, not an official Proton SDK. Proton could change that API at
any time without notice, independent of anything ABM does.

## Before relying on this provider

- Check current rclone release notes for the pinned rclone version
  specifically for any Proton Drive backend changes or known issues before
  trusting it with real backups.
- Treat any future rclone version bump that touches this backend as
  higher-risk: review its changelog, verify checksums, and re-run the
  regression suite (including a real restore test against this provider)
  before rolling it out.
- The session may require periodic reauthentication.

## Setup

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. proton) -> type "protondrive"
# complete Proton account authentication (and 2FA if enabled) when prompted

abm storage add --provider proton-drive --name proton --remote proton
```

## Reconnect

```bash
abm storage reconnect proton
```

## Security considerations

Never store your raw Proton account password in ABM configuration or
anywhere outside Proton's own authentication flow. Not immutable/WORM
storage.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```

Given the experimental status, verify a real restore on this destination
more frequently than you would for a Tier-1 provider.
