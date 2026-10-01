# Use an Existing rclone Remote

| | |
|---|---|
| **Provider ID** | `rclone-existing` |
| **Maturity** | Supported (the escape hatch itself is implemented and tested against local destinations; individual rclone backends used through it are not independently validated by this project) |
| **Backend** | rclone (whatever backend the remote itself uses) |
| **Auth** | Whatever the remote already has configured |
| **Headless** | Direct — the remote is already authenticated |

This is the scaling mechanism for every rclone backend this project doesn't
(yet) have a first-class preset for: configure it yourself with
`rclone config`, then point ABM at it. Adding real first-class support for a
new provider later is a registry entry, not a rewrite — this path is what
makes that possible without an ABM release for every one of rclone's
dozens of backends.

## Setup

```bash
# first, configure the remote yourself if you haven't already:
rclone config --config /etc/auto-backup-manager/rclone.conf

# then register it:
abm storage add --provider rclone-existing --name my-remote --remote my-remote
```

## What ABM does before accepting it

Before registering the destination, ABM runs a repository capability smoke
test (`internal/backend`): it confirms the remote is reachable, then
initializes a disposable restic repository, backs up a tiny test file,
restores it, and verifies the content — the same round-trip any first-class
provider must pass. **A remote that cannot complete this round-trip is not
accepted as a storage destination.**

## Known limitations

ABM does not know this backend's real-world maturity as a restic repository
target the way it does for a first-class preset — some rclone backends are
far more battle-tested than others for this specific use case (large files,
many small objects, frequent listing). The capability test catches outright
incompatibility, not subtle reliability differences under production load.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```
