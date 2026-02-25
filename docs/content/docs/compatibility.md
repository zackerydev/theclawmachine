---
title: Compatibility
weight: 15
next: architecture
prev: getting-started
---

Upstream requirements and Kubernetes environment compatibility for ClawMachine.

## Local Kubernetes

ClawMachine will optionally set up all upstream components (Cilium CNI, External Secrets Operator, 1Password Connect) during installation. Local clusters are the recommended way to get started.

| Environment | Supported | Notes |
|-------------|-----------|-------|
| [kind](https://kind.sigs.k8s.io) | ✅ Yes | Full support including network isolation via Cilium |
| [OrbStack](https://orbstack.dev) | ⚠️ Partial | See note below |

### OrbStack

OrbStack is supported with one caveat: it does not support the network isolation features provided by Cilium. The Cilium CNI requires Linux kernel capabilities that OrbStack's lightweight VM does not expose.

**What works:** bot installation, secrets management, backups, the control plane dashboard.

**What doesn't work:** per-bot network policies and egress allow lists. Bots will have unrestricted outbound network access regardless of your configuration.

If network isolation is a requirement, use kind instead.

## Remote Kubernetes

Remote cluster support is not yet tested but should work. If you are deploying ClawMachine to an existing remote cluster and do not want it to manage the CNI, ESO, or 1Password Connect, you must have the following upstream components already installed and configured:

### Required upstream components

**CNI with network policy support**

A CNI that supports `NetworkPolicy` enforcement is required for per-bot egress isolation. [Cilium](https://cilium.io) is recommended. Other CNIs that implement the Kubernetes `NetworkPolicy` spec (Calico, Weave, etc.) should also work.

**External Secrets Operator (ESO)**

[ESO](https://external-secrets.io) is used to sync secrets from your provider into Kubernetes. Install it before running ClawMachine if you want to manage secrets through the dashboard.

```bash
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets external-secrets/external-secrets -n external-secrets --create-namespace
```

**1Password Connect**

[1Password Connect](https://developer.1password.com/docs/connect) is the secrets backend ClawMachine integrates with. You can either point ClawMachine at an existing Connect server or let it install one in-cluster via the Settings → Providers page.

If you bring your own Connect instance, you will need the Connect host URL and an access token to configure the SecretStore through the dashboard.
