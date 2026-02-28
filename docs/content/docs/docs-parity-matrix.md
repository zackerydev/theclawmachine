---
title: Docs Parity Matrix
weight: 160
prev: cli
next: advanced-setup
---

Repo-verified matrix used to keep docs aligned with code.

## CLI Commands (cmd/clawmachine)

- serve
- install
- upgrade
- uninstall
- doctor
- status
- backup
- restore
- completion
- version

## HTTP Route Groups (internal/routes.go)

### Pages

- `GET /`
- `GET /bots/new`
- `POST /bots/new/infra`
- `POST /bots/new/config`
- `POST /bots/new/software`
- `GET /bots/{name}/page`
- `GET /settings`
- `GET /settings/providers`
- `GET /secrets`

### Bot APIs

- `GET /bots`
- `POST /bots`
- `GET /bots/{name}`
- `PUT /bots/{name}`
- `DELETE /bots/{name}`
- `GET /bots/{name}/logs`
- `POST /bots/{name}/cli`
- `POST /bots/{name}/restart`
- `PUT /bots/{name}/config`
- `GET /bots/{name}/network`
- `GET /bots/{name}/backup`
- `POST /bots/{name}/backup`

### Settings/Secrets APIs

- `GET /settings/status`
- `POST /settings/provider`
- `DELETE /settings/provider`
- `POST /settings/connect/install`
- `DELETE /settings/connect`
- `GET /secrets/available`
- `GET /secrets/status`
- `POST /secrets`
- `DELETE /secrets/{name}`

### Helper APIs

- `GET /api/botenv`
- `GET /api/models`
- `GET /api/onboarding/profile`
- `POST /api/onboarding/preview`

## Known Notes

- `GET /secrets/new` currently redirects to `/secrets`.
- Runtime default namespace for bot operations is `claw-machine` unless overridden by `POD_NAMESPACE`.
