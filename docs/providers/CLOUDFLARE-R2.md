# Cloudflare R2

| | |
|---|---|
| **Provider ID** | `cloudflare-r2` |
| **Maturity** | Supported (implemented, not yet validated against a real R2 bucket by this project) |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

## Setup

```bash
abm storage add --provider cloudflare-r2 --name r2 \
    --endpoint <account-id>.r2.cloudflarestorage.com \
    --access-key <key> --secret-key <secret>
```

The endpoint is account-specific (`<account-id>.r2.cloudflarestorage.com`);
find your account ID and generate S3 API credentials from the R2 dashboard.
R2 does not use the `region` field the way AWS does — leave it unset unless
Cloudflare's current documentation says otherwise for your setup.

## Security considerations

Scope the API token to the specific bucket via R2's token permissions, not an
account-wide token.

## Object Lock

R2 supports Object Lock on buckets with it enabled at creation time. See
[../IMMUTABILITY.md](../IMMUTABILITY.md) before relying on it.

## Reconnect / restore test

Same as [S3.md](S3.md).
