---
title: Advanced Setup
weight: 120
next: troubleshooting
prev: cli
---

Manual Helm installation for users who prefer full control over the deployment.

{{< callout type="info" >}}
For most users, the [Quick Start](../getting-started) interactive installer is the fastest path. This page covers the manual Helm workflow.
{{< /callout >}}

## Manual Helm Installation

### Add the Chart Repository

```bash
helm repo add clawmachine https://charts.clawmachine.dev
helm repo update
```

### Install with Defaults

```bash
helm install clawmachine clawmachine/clawmachine \
  --create-namespace --namespace claw-machine
```

### Install with External Secrets

```bash
helm install clawmachine clawmachine/clawmachine \
  --create-namespace --namespace claw-machine \
  --set external-secrets.enabled=true
```

### Custom Values File

Create a `values.yaml` to customize the deployment:

```yaml
replicaCount: 1

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: clawmachine.example.com
      paths:
        - path: /
          pathType: Prefix

external-secrets:
  enabled: true
```

Then install with your overrides:

```bash
helm install clawmachine clawmachine/clawmachine \
  --create-namespace --namespace claw-machine \
  -f values.yaml
```

## Upgrading

```bash
helm repo update
helm upgrade clawmachine clawmachine/clawmachine -n claw-machine
```

## Uninstalling

```bash
helm uninstall clawmachine -n claw-machine
```

## Configuration Reference

See [Helm Chart](../helm-chart) for the full values reference and dependency configuration.
