---
title: Backup & Restore
weight: 100
next: cli
prev: secrets
---

ClawMachine supports backup and restore via S3-compatible storage.

## Modes

### `filesystem`

- Archives workspace as `tar.gz`
- Used by OpenClaw and PicoClaw charts by default
- Writes timestamped objects plus `latest.tar.gz`

### `pgdump`

- Runs `pg_dump`, compresses output as `sql.gz`
- Used by IronClaw chart by default
- Restore pipes decompressed SQL into `psql`

## CLI

```bash
clawmachine backup --mode filesystem --bucket my-bucket
clawmachine backup --mode pgdump --bucket my-bucket
clawmachine restore --mode filesystem --bucket my-bucket
clawmachine restore --mode pgdump --bucket my-bucket
```

Required flag: `--bucket`.

Common options:

- `--endpoint`
- `--region`
- `--prefix`
- `--workspace` (filesystem mode)

`pgdump` mode requires `DATABASE_URL`.

## Chart-backed automation

Bot charts support backup CronJobs and restore init containers with chart `backup.*` values.

Credential options:

- Secret refs (`backup.credentials.*SecretRef`) from synced ExternalSecrets
- Legacy credentials secret path (`credentialsSecret`) when raw keys are supplied

If no `latest.*` object exists, restore exits successfully for fresh startup behavior.
