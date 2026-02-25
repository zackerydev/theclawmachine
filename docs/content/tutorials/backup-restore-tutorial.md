---
title: Backup and Restore Tutorial
weight: 6
---

This tutorial verifies backup/restore using S3-compatible storage.

## Step 1: Configure backup in install wizard

In install step 1:

- enable backups
- set schedule
- set provider and S3 fields (`endpoint`, `bucket`, `region`, `prefix`)
- select access key and secret key secrets if using ExternalSecrets

## Step 2: Confirm CronJob

```bash
kubectl get cronjobs -n claw-machine
```

## Step 3: Trigger a manual run

```bash
kubectl create job --from=cronjob/<bot-name>-backup <bot-name>-backup-manual -n claw-machine
```

## Step 4: Validate restore path

Recreate the bot with same backup config. Restore init container attempts to read `latest.tar.gz` or `latest.sql.gz`.

If no latest object exists, restore exits cleanly and bot starts fresh.
