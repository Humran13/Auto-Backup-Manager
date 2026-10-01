# Google Cloud Storage

| | |
|---|---|
| **Provider ID** | `google-cloud-storage` |
| **Maturity** | Supported (implemented, not yet validated against a real GCS bucket by this project) |
| **Backend** | restic's native `gs` backend (not rclone) |
| **Auth** | Service-account JSON key file |
| **Headless** | Direct |

Not to be confused with Google **Drive** (a different provider, different
backend — see [GOOGLE-DRIVE.md](GOOGLE-DRIVE.md)).

## What you need

- A GCP project ID
- A bucket
- A service-account JSON key file with write access to that bucket

## Setup

```bash
abm storage add --provider google-cloud-storage --name gcs \
    --project-id <project-id> --bucket <bucket> \
    --credentials-file /etc/auto-backup-manager/keys/gcs-service-account.json
```

## Security considerations

The service-account JSON key file is as sensitive as any other credential:
restrict it to the bucket it needs, store it with the same file permissions
as other ABM secrets, and never commit it to source control.

## Reconnect / restore test

Service-account keys don't expire the way user OAuth tokens do; rotate the
key in GCP IAM and update the credentials file if needed.

```bash
abm restore <job> latest --target /tmp/restore-test
```

## Known limitations

GCS's own Bucket Lock (retention policies) is not evaluated here for
interaction with `restic prune` — see [../IMMUTABILITY.md](../IMMUTABILITY.md).
