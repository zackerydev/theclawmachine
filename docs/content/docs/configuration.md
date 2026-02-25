---
title: Configuration
weight: 50
next: helm-chart
prev: bots
---

This page covers runtime and chart configuration surfaces.

## CLI Flags

### `serve`

| Flag | Default |
|------|---------|
| `--context` | current context |
| `--dev` | `false` |

### `install`

| Flag | Default |
|------|---------|
| `--namespace` | `claw-machine` |
| `--name` | `clawmachine` |
| `--context` | current context |
| `--external-secrets` | `false` |
| `--cilium` | `false` |
| `--interactive` | `true` |
| `--yes` | `false` |

### `upgrade`

| Flag | Default |
|------|---------|
| `--namespace` | `claw-machine` |
| `--name` | `clawmachine` |
| `--yes` | `false` |

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | server port |
| `LOG_LEVEL` | `info` | structured logging level |
| `POD_NAMESPACE` | `claw-machine` fallback | bot operation namespace at runtime |
| `CLAWMACHINE_CHART_REPO` | `oci://ghcr.io/zackerydev/theclawmachine/charts` | remote chart source override |

## Control Plane Chart

`control-plane/charts/clawmachine/values.yaml` currently includes:

- `replicaCount`
- `image.*`
- `service.*`
- `ingress.*`
- `resources`
- `devMode`
- `external-secrets.*`

Use `helm show values` or the local values file for authoritative defaults.

## Bot Charts

Bot chart defaults live in:

- `control-plane/charts/openclaw/values.yaml`
- `control-plane/charts/picoclaw/values.yaml`
- `control-plane/charts/ironclaw/values.yaml`
- `control-plane/charts/busybox/values.yaml`

Install wizard fields map into those values (network, persistence, workspace, backup, onboarding config, and secret refs).
