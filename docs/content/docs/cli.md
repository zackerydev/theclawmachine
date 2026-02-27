---
title: CLI Reference
weight: 110
next: advanced-setup
prev: network-security
---

ClawMachine is a single binary. `clawmachine` without a subcommand runs `serve`.

## Command Summary

```bash
clawmachine serve
clawmachine install
clawmachine upgrade
clawmachine uninstall
clawmachine doctor
clawmachine status
clawmachine backup
clawmachine restore
clawmachine completion [bash|zsh|fish|powershell]
clawmachine version
clawmachine version --all
```

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--context` | current kube context | Kubernetes context to use |
| `--dev` | `false` | Enable dev mode for `serve` |
| `--web` | `false` | Alias-style flag for web mode |

## `serve`

```bash
clawmachine
clawmachine serve
clawmachine serve --dev
```

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Listen port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## `install`

```bash
clawmachine install
clawmachine install --namespace claw-machine --external-secrets --cilium
clawmachine install --context my-cluster --name clawmachine --yes
```

| Flag | Default | Description |
|------|---------|-------------|
| `--namespace` | `claw-machine` | Target namespace |
| `--name` | `clawmachine` | Helm release name |
| `--external-secrets` | `false` | Install External Secrets Operator |
| `--cilium` | `false` | Install Cilium CNI |
| `--interactive` | `true` | Run interactive installer |
| `--yes`, `-y` | `false` | Skip confirmation prompts |

Image tag contract:
- `clawmachine install` always deploys `ghcr.io/zackerydev/theclawmachine:<tag>` with an explicit `image.tag` override.
- For release/prerelease binaries, `<tag>` is the CLI version (without a leading `v`).
- For `dev` or empty CLI versions, install falls back to the embedded chart `appVersion`.
- Invalid non-dev CLI versions fail fast instead of silently falling back.

## `upgrade`

```bash
clawmachine upgrade
clawmachine upgrade --namespace claw-machine --name clawmachine --yes
```

| Flag | Default | Description |
|------|---------|-------------|
| `--namespace` | `claw-machine` | Target namespace |
| `--name` | `clawmachine` | Helm release name |
| `--yes`, `-y` | `false` | Skip confirmation prompt |

`clawmachine upgrade` uses the same image tag contract as `install`.

## `uninstall`

```bash
clawmachine uninstall
clawmachine uninstall --namespace claw-machine --yes
```

## `doctor`

```bash
clawmachine doctor
```

## `status`

```bash
clawmachine status
```

## `backup` and `restore`

```bash
clawmachine backup --bucket my-backups
clawmachine restore --bucket my-backups
```

See full flags with:

```bash
clawmachine backup --help
clawmachine restore --help
```

## `completion`

```bash
clawmachine completion zsh
clawmachine completion bash
```

## `version`

```bash
clawmachine version
clawmachine version --all
```

`clawmachine version` prints the CLI version.

`clawmachine version --all` also prints:
- Canonical bot image refs (`repo:tag`) from embedded bot charts
- SHA256 checksums for vendored embedded charts (External Secrets, Cilium, Connect)

Example:

```text
clawmachine v0.1.0

bot images (canonical repo:tag):
  - openclaw: ghcr.io/zackerydev/openclaw:0.1.0
  - picoclaw: ghcr.io/zackerydev/picoclaw:0.1.0
  - ironclaw: ghcr.io/zackerydev/ironclaw:0.1.0
  - busybox: ghcr.io/zackerydev/theclawmachine-toolbox:0.1.0

vendored charts (sha256):
  - external-secrets@2.0.0: sha256:<digest>
  - cilium@1.17.1: sha256:<digest>
  - connect@2.3.0: sha256:<digest>
```

## HTTP API Endpoints (Serve Mode)

### Core

| Method | Path |
|--------|------|
| `GET` | `/health` |
| `GET` | `/static/*` |

### Pages

| Method | Path |
|--------|------|
| `GET` | `/` |
| `GET` | `/bots/new` |
| `POST` | `/bots/new/infra` |
| `POST` | `/bots/new/config` |
| `GET` | `/bots/{name}/page` |
| `GET` | `/settings` |
| `GET` | `/settings/providers` |
| `GET` | `/secrets` |
| `GET` | `/secrets/new` (redirects to `/secrets`) |

### Bot Operations

| Method | Path |
|--------|------|
| `GET` | `/bots` |
| `POST` | `/bots` |
| `GET` | `/bots/{name}` |
| `PUT` | `/bots/{name}` |
| `DELETE` | `/bots/{name}` |
| `GET` | `/bots/{name}/logs` |
| `POST` | `/bots/{name}/cli` |
| `POST` | `/bots/{name}/restart` |
| `PUT` | `/bots/{name}/config` |
| `GET` | `/bots/{name}/network` |
| `GET` | `/bots/{name}/backup` |
| `POST` | `/bots/{name}/backup` |

### Settings and Secrets

| Method | Path |
|--------|------|
| `GET` | `/settings/status` |
| `POST` | `/settings/provider` |
| `DELETE` | `/settings/provider` |
| `POST` | `/settings/connect/install` |
| `DELETE` | `/settings/connect` |
| `GET` | `/secrets/available` |
| `GET` | `/secrets/status` |
| `POST` | `/secrets` |
| `DELETE` | `/secrets/{name}` |

### API Helpers

| Method | Path |
|--------|------|
| `GET` | `/api/botenv` |
| `GET` | `/api/models` |
| `GET` | `/api/onboarding/profile` |
| `POST` | `/api/onboarding/preview` |

HTMX requests are supported for page partials (`HX-Request: true`).
