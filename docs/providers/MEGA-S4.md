# MEGA S4 (Object Storage)

| | |
|---|---|
| **Provider ID** | `mega-s4` |
| **Maturity** | Supported (implemented, not yet validated against a real MEGA S4 account by this project) |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

**MEGA S4 is a distinct product from consumer MEGA file storage** (see
[MEGA.md](MEGA.md)) — different backend, different architecture, not
interchangeable. S4 is MEGA's S3-compatible object-storage offering and uses
this project's generic S3 engine.

## Setup

```bash
abm storage add --provider mega-s4 --name mega-s4 \
    --endpoint <exact-endpoint-from-your-s4-dashboard> \
    --access-key <key> --secret-key <secret>
```

Confirm the exact current endpoint and region naming in your MEGA S4
dashboard rather than assuming a fixed pattern — this is a newer product and
worth double-checking against current MEGA documentation.

## Security considerations

Scope credentials to the bucket if MEGA S4's console supports it.

## Object Lock

Check current MEGA S4 documentation for Object Lock support status at setup
time; see [../IMMUTABILITY.md](../IMMUTABILITY.md).

## Reconnect / restore test

Same as [S3.md](S3.md).
