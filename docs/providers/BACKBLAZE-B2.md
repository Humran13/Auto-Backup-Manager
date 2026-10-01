# Backblaze B2

| | |
|---|---|
| **Provider ID** | `backblaze-b2` |
| **Maturity** | Stable |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

Uses Backblaze B2's S3-compatible API, through the same native-S3 engine as
every other object-storage preset (restic has a separate native B2 backend
too, but this project standardizes on the S3-compatible path for one shared
engine rather than two parallel implementations).

## Setup

```bash
abm storage add --provider backblaze-b2 --name b2 \
    --endpoint s3.<your-bucket-region>.backblazeb2.com \
    --access-key <application-key-id> --secret-key <application-key>
```

The endpoint's region segment (e.g. `us-west-002`, `eu-central-003`) is
whatever region you chose when creating the bucket — check the bucket's
details page in the B2 console for its exact S3 endpoint.

## Security considerations

Create an **Application Key scoped to one bucket** rather than your master
key — least privilege, and a leaked key can't reach your other buckets.

## Object Lock

Backblaze B2 supports native Object Lock. See [../IMMUTABILITY.md](../IMMUTABILITY.md)
before relying on it — enabling it changes how `restic prune` must be run.

## Reconnect / restore test

Same as [S3.md](S3.md) — keys don't expire; `abm restore <job> latest --target ...` to verify.
