---
title: Development
weight: 150
prev: troubleshooting
next: roadmap
---

Set up local development for ClawMachine.

## Prerequisites

```bash
mise trust && mise install
```

## Common tasks

| Command | Purpose |
|---------|---------|
| `mise run charts` | Package charts for embed |
| `mise run build` | Build binary |
| `mise run test` | Unit tests |
| `mise run lint` | `golangci-lint` |
| `mise run vet` | `go vet` |
| `mise run docs:check` | Build docs site |

## Local server

```bash
cd control-plane
go run ./cmd/clawmachine serve --dev
```

## Key directories

- `control-plane/cmd/clawmachine`: cobra commands
- `control-plane/internal/routes.go`: route registration
- `control-plane/internal/handler`: handlers
- `control-plane/internal/service`: business logic
- `control-plane/internal/onboarding`: onboarding contracts/compiler
- `control-plane/internal/botenv`: bot env metadata
- `control-plane/templates`: HTML templates
- `control-plane/charts`: source charts
- `docs/`: Hugo docs site

## Docs workflow

```bash
cd docs
hugo --minify
```
