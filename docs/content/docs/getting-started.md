---
title: Getting Started
weight: 10
next: compatibility
prev: /docs
---

Install ClawMachine, open the dashboard, and deploy your first bot.

## Prerequisites

- Kubernetes cluster (v1.28+ recommended)
- `kubectl` configured for that cluster
- Helm installed

## Install CLI

```bash
curl -fsSL https://theclawmachine.dev/install.sh | bash
```

## Install ClawMachine

```bash
clawmachine install
```

Useful flags:

```bash
clawmachine install \
  --context kind-clawmachine \
  --namespace claw-machine \
  --external-secrets \
  --cilium \
  --yes
```

## Access Dashboard

```bash
kubectl port-forward -n claw-machine svc/clawmachine 8080:80
open http://localhost:8080
```

## Install First Bot

1. Open `/bots/new`.
2. Choose bot type.
3. Complete step 1 (infrastructure settings).
4. Complete step 2 (bot config questions).
5. Submit to install.

## Next

- [Bot Management](../bots)
- [Secrets Management](../secrets)
- [CLI Reference](../cli)
