---
title: Deploy Your First Bot
weight: 1
---

This tutorial walks through deploying your first bot via the three-step install wizard.

## Prerequisites

- ClawMachine installed
- `kubectl` access to your cluster

## Step 1: Open the dashboard

```bash
kubectl port-forward -n claw-machine svc/clawmachine 8080:80
```

Open `http://localhost:8080`.

## Step 2: Start install

1. Click **Install Bot**.
2. Choose a bot type (OpenClaw is recommended for first deploy).

## Step 3: Infrastructure (step 1)

Set:

- release name
- persistence
- ingress/egress and allowed domains
- optional workspace import
- optional backup settings

Click **Next**.

## Step 4: Bot config (step 2)

Fill bot-specific config questions. For secret fields, choose synced ExternalSecrets where available.

Click **Next**.

## Step 5: Extra software (step 3)

Optionally provide `.tool-versions` content for command-line tools you want installed by `mise` on bot startup.

Click **Install Bot**.

## Step 6: Verify

- Bot appears on `/`
- Detail page loads at `/bots/{name}/page`
- Logs tab shows runtime logs

CLI check:

```bash
kubectl get pods -n claw-machine -l app.kubernetes.io/instance=my-first-bot
```
