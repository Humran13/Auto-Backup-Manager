# Hetzner Object Storage

| | |
|---|---|
| **Provider ID** | `hetzner-object-storage` |
| **Maturity** | Supported (implemented, not yet validated against a real Hetzner bucket by this project) |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

## Setup

```bash
abm storage add --provider hetzner-object-storage --name hetzner \
    --endpoint <region>.your-objectstorage.com \
    --access-key <key> --secret-key <secret>
```

The endpoint is region-specific (e.g. `fsn1.your-objectstorage.com`,
`nbg1.your-objectstorage.com`) — confirm the exact hostname for your chosen
region in the Hetzner console; it is a comparatively newer Hetzner product
and hostnames are worth double-checking against current docs.

## Security considerations

Generate a project-scoped access key rather than reusing broader
infrastructure credentials.

## Object Lock

Check current Hetzner documentation for Object Lock support status at setup
time before relying on it for ransomware resilience; see
[../IMMUTABILITY.md](../IMMUTABILITY.md).

## Reconnect / restore test

Same as [S3.md](S3.md).
