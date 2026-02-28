---
title: Bot Management
weight: 40
next: configuration
prev: architecture
---

ClawMachine manages bot Helm releases in the runtime namespace (default `claw-machine`).

## Supported Bot Types

- OpenClaw
- PicoClaw
- IronClaw
- BusyBox (dev mode)

See [Bot Types](../bot-types) for defaults.

## Install Flow

Dashboard install is a three-step wizard:

1. **Infrastructure step** (`POST /bots/new/infra`)
: release name, persistence, ingress/egress, allowed domains, workspace import, backup settings.
2. **Config step** (`POST /bots/new/config`)
: onboarding-generated bot-specific config questions and model/secret mappings.
3. **Extra software step** (`POST /bots/new/software`)
: optional `.tool-versions` content for startup `mise install`.

Final install request is `POST /bots`.

## Bot Detail

![ClawMachine control plane — bot detail view showing status, tabs, and core runtime](/images/control-plane-bot-detail.png)

`/bots/{name}/page` includes:

- overview/status
- logs (`GET /bots/{name}/logs`)
- network flows (`GET /bots/{name}/network`)
- bot CLI execution (`POST /bots/{name}/cli`, OpenClaw only)
- core settings save path (`PUT /bots/{name}` and `PUT /bots/{name}/config`)
- restart/uninstall actions

## API Operations

```bash
# list
curl http://localhost:8080/bots

# install
curl -X POST http://localhost:8080/bots -H 'Content-Type: application/json' -d '{"releaseName":"my-bot","botType":"openclaw","values":{}}'

# status
curl http://localhost:8080/bots/my-bot

# upgrade values
curl -X PUT http://localhost:8080/bots/my-bot -H 'Content-Type: application/json' -d '{"botType":"openclaw","values":{"replicas":2}}'

# uninstall
curl -X DELETE http://localhost:8080/bots/my-bot
```

## Secret Handling in Bot Config

Install/upgrade supports:

- `envSecrets` bindings (env var -> secret/key)
- backup credential refs resolved from ExternalSecrets
- chart values with sensitive fields stripped/redacted where required
