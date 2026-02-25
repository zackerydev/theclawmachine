---
title: Secrets Management
weight: 70
next: network-security
prev: helm-chart
---

ClawMachine integrates with External Secrets Operator (ESO) and currently supports 1Password provider setup.

## Prerequisites

Install ESO first:

```bash
clawmachine install --external-secrets
```

You can then configure 1Password in the dashboard under `Settings` -> `Secret Providers`.

## Provider Setup Paths

ClawMachine supports two setup paths in **Settings > Secret Providers** (`/settings/providers`):

1. **Install 1Password Connect in-cluster** (`POST /settings/connect/install`)
2. **Use an existing Connect server** (`POST /settings/provider`)

Both flows configure a SecretStore in the bot namespace (default `claw-machine`) and store the Connect token in a Kubernetes Secret.

## External Secrets UI

Create an ExternalSecret from `/secrets` using the simplified form:

- `name` (Kubernetes resource name)
- `item` (1Password item key/name)
- `field` (defaults to `credential`)

This creates an `ExternalSecret` managed by ClawMachine and targets a same-name Kubernetes Secret.

Note: `GET /secrets/new` currently redirects to `/secrets`.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/settings` | Settings landing page |
| `GET` | `/settings/providers` | Provider management page |
| `POST` | `/settings/connect/install` | Install in-cluster Connect and configure store |
| `POST` | `/settings/provider` | Configure existing Connect host/token/vault |
| `DELETE` | `/settings/provider` | Remove SecretStore + token secret |
| `DELETE` | `/settings/connect` | Uninstall in-cluster Connect |
| `GET` | `/secrets` | ExternalSecret list/create page |
| `POST` | `/secrets` | Create/update ExternalSecret |
| `DELETE` | `/secrets/{name}` | Delete ExternalSecret |

## Out-of-Band Bot Secrets

Bot install/update flows also support out-of-band bot secrets. Sensitive values are created as Kubernetes Secrets and referenced by chart values, rather than being stored directly in Helm release values.

Use cases:

- Provider API keys
- Channel tokens
- Backup credential secret refs
- `envSecrets` bindings for per-variable `secretKeyRef`

## Troubleshooting

- **ESO not installed**: install with `clawmachine install --external-secrets`.
- **Provider not ready**: verify Connect host/token/vault and check SecretStore conditions.
- **ExternalSecret pending**: verify item/field names and SecretStore readiness.
