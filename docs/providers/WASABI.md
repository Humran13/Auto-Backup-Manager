# Wasabi

| | |
|---|---|
| **Provider ID** | `wasabi` |
| **Maturity** | Stable |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

## Setup

```bash
abm storage add --provider wasabi --name wasabi \
    --endpoint s3.<your-bucket-region>.wasabisys.com \
    --region <your-bucket-region> \
    --access-key <key> --secret-key <secret>
```

Wasabi's endpoint is region-specific (e.g. `s3.us-east-1.wasabisys.com`);
confirm the exact hostname for your bucket's region in the Wasabi console.

## Security considerations

Create an access key scoped to the minimum policy needed (ideally one bucket)
rather than reusing an account-wide key.

## Object Lock

Wasabi supports native Object Lock (and, notably, offers it at no extra
storage cost on compliance-enabled buckets). See
[../IMMUTABILITY.md](../IMMUTABILITY.md) before relying on it.

## Reconnect / restore test

Same as [S3.md](S3.md).
