# DigitalOcean Spaces

| | |
|---|---|
| **Provider ID** | `digitalocean-spaces` |
| **Maturity** | Supported (implemented, not yet validated against a real Spaces bucket by this project) |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

## Setup

```bash
abm storage add --provider digitalocean-spaces --name spaces \
    --endpoint <region>.digitaloceanspaces.com \
    --region <region> \
    --access-key <key> --secret-key <secret>
```

The endpoint is region-specific (e.g. `nyc3.digitaloceanspaces.com`).

## Security considerations

Generate a Spaces access key scoped as narrowly as DigitalOcean's key
management allows.

## Object Lock

Check current DigitalOcean documentation for Object Lock support status at
setup time; see [../IMMUTABILITY.md](../IMMUTABILITY.md).

## Reconnect / restore test

Same as [S3.md](S3.md).
