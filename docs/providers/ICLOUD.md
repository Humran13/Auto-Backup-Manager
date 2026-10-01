# Apple iCloud Drive — EXPERIMENTAL

| | |
|---|---|
| **Provider ID** | `icloud-drive` |
| **Maturity** | **Experimental** |
| **Backend** | rclone `iclouddrive` (itself an experimental rclone backend) |
| **Auth** | Apple ID + 2FA, producing a session/trust token |
| **Headless** | Difficult |

**This provider is marked experimental for reasons outside ABM's control:**
rclone's own iCloud Drive backend is experimental, Apple's authentication
model requires interactive 2FA, and the resulting session is not guaranteed
to last indefinitely. Do not choose this provider if you need guaranteed,
unattended, multi-month operation without any manual intervention.

## What to expect

- Initial setup requires completing Apple's 2FA challenge interactively —
  this cannot be done unattended on a headless server with no way to receive
  the 2FA prompt. Complete setup from a machine/session where you can
  respond to it, or use `rclone config` run via an SSH session with your
  phone nearby for the 2FA code.
- App-specific passwords are **not** a reliable substitute for the full
  interactive auth flow for this backend — verify current rclone
  documentation before assuming otherwise.
- **Periodic reauthentication is expected**, not a bug. `abm doctor` and
  `abm storage test icloud` will report an authentication failure when the
  session expires; this is the normal way to find out it's time to run:

```bash
abm storage reconnect icloud
```

## Setup

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. icloud) -> type "iclouddrive"
# complete Apple ID + 2FA authentication when prompted

abm storage add --provider icloud-drive --name icloud --remote icloud
```

## Security considerations

Never store your raw Apple ID password in ABM configuration or anywhere
outside Apple's own authentication flow. Not immutable/WORM storage.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```

Given the experimental status, verify a real restore on this destination
more frequently than you would for a Tier-1 provider.
