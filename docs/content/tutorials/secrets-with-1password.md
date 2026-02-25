---
title: Secrets with 1Password
weight: 2
---

This tutorial covers ClawMachine's current 1Password + ESO flow.

## Prerequisites

- External Secrets Operator installed
- 1Password Connect credentials/token available

## Step 1: Configure provider

Open `/settings/providers`.

Choose one path:

1. **Install 1Password Connect** (in-cluster)
2. **Use Existing Connect Server**

Required fields:

- Connect host (existing path)
- access token
- vault name

## Step 2: Create ExternalSecret

Open `/secrets` and submit:

- secret name
- 1Password item
- field (default `credential`)

ClawMachine creates/updates an `ExternalSecret` and ESO syncs target Kubernetes Secret data.

## Step 3: Use secrets in bot install

During bot install config step, secret-backed questions let you select ExternalSecrets.

For backup credentials, install step 1 provides Secret selectors that map to chart credential refs.

## Step 4: Validate

```bash
kubectl get externalsecrets -n claw-machine
kubectl get secretstore -n claw-machine
```
