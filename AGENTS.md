# ClawMachine Agent Guide

This file is the implementation source of truth for coding agents working in this repository.

If guidance conflicts:
1. Preserve simplicity.
2. Preserve maintainability.
3. Follow code-backed contracts in `control-plane/internal/routes.go` and `control-plane/cmd/clawmachine`.

## Core Values

- Simplicity over cleverness.
- Maintainability over short-term speed.
- Small, explicit changes over broad rewrites.

## Stack and Constraints

- Backend: Go (`net/http`, `html/template`), Cobra CLI.
- Frontend: server-rendered HTML + HTMX + small vanilla JS.
- Styling: Bootstrap + project CSS, with theme conventions defined in existing CSS and templates.
- Platform: Kubernetes + Helm.
- No JS framework.

## Architecture Ownership (Do Not Blur Boundaries)

- `control-plane/cmd/clawmachine`: CLI commands and wiring.
- `control-plane/internal/routes.go`: central HTTP route contract.
- `control-plane/internal/handler`: HTTP parsing/validation/response orchestration.
- `control-plane/internal/service`: Helm, Kubernetes, secrets, logs, backup, template, workspace logic.
- `control-plane/internal/onboarding`: canonical onboarding profile and compile engine.
- `control-plane/internal/botenv`: bot metadata registry and config mapping.
- `control-plane/templates`: page and partial templates.
- `control-plane/static`: minimal client-side behavior and styles.
- `control-plane/charts`: Helm chart sources.
- `docs/`: documentation site.

Rule of thumb: keep handlers thin; move reusable logic into services or onboarding/botenv packages.

## Non-Negotiable Invariants (MUST)

- Namespace behavior:
  - Bot operations must respect runtime namespace resolution (`POD_NAMESPACE` fallback behavior).
  - Do not hardcode namespace logic in new handlers.
- Route contract:
  - Register routes in `control-plane/internal/routes.go`.
  - Route changes require matching docs updates.
- CLI contract:
  - Command and flag changes require CLI docs updates.
- Rendering and security:
  - Do not silently ignore template render failures.
  - Keep security and body-limit middleware guarantees intact.
  - Escape untrusted HTML output.
- Docs build:
  - `mise run docs:check` (Hugo build) is the docs gate.

## Engineering Standards

- Prefer explicit code paths over hidden abstractions.
- Prefer standard library patterns already used in repo.
- Keep interfaces focused on behavior actually consumed by handlers.
- Use `any` instead of `interface{}`.
- Avoid introducing new dependencies unless a clear maintenance win is documented in code comments or PR notes.
- Keep feature toggles and defaults centralized in existing config surfaces.

## Change Playbooks

### 1) Add or change an HTTP route

Required updates:
- `control-plane/internal/routes.go`
- relevant handler(s) under `control-plane/internal/handler`
- route tests (`control-plane/internal/routes_test.go` and/or handler tests)
- docs route references (at minimum `docs/content/docs/cli.md`)

Validation:
- `mise run test`
- `mise run docs:check`

### 2) Add or change a CLI command or flags

Required updates:
- `control-plane/cmd/clawmachine/*`
- command tests in `control-plane/cmd/clawmachine/*_test.go`
- `docs/content/docs/cli.md`

Validation:
- `mise run test`
- `mise run docs:check`

### 3) Change install wizard, onboarding, or config compilation

Required updates (as needed by change):
- `control-plane/internal/handler/helm.go`
- `control-plane/internal/onboarding/*`
- `control-plane/internal/botenv/*`
- related templates under `control-plane/templates/pages/*`
- tests in handler/onboarding/botenv packages
- docs pages describing onboarding/config behavior

Validation:
- `mise run test`
- `mise run lint`
- `mise run vet`
- `mise run docs:check`

### 4) Add a new bot type

Required updates:
- bot metadata YAML in `control-plane/internal/botenv/bots/`
- registry/config builder behavior and tests
- chart under `control-plane/charts/<bot>/`
- chart packaging expectations (`mise run charts`)
- install/onboarding UX where relevant
- docs: bot types, operations, and any config specifics

Validation:
- `mise run charts`
- `mise run test`
- `mise run lint`
- `mise run vet`
- `mise run docs:check`

### 5) Change secrets/providers integration

Required updates:
- `control-plane/internal/handler/secrets*.go`
- `control-plane/internal/service/secrets.go` and/or `connect.go`
- relevant settings/secrets templates
- tests covering validation and failure paths
- docs for provider and secrets flows

Validation:
- `mise run test`
- `mise run lint`
- `mise run docs:check`

## Validation Matrix (Hard Gates)

Run the smallest set that fully covers your change. If in doubt, run all:

```bash
mise run charts
mise run test
mise run lint
mise run vet
mise run docs:check
```

If any expected check is skipped, record the reason in the PR.

## Development Workflow Notes

- Install tools: `mise trust && mise install`
- Fast local loop:
  - `cd control-plane`
  - `go run ./cmd/clawmachine serve --dev`
- Tilt workflow (optional infra toggles):
  - `mise run dev`
  - `tilt up -- --no-cilium`
  - `tilt up -- --no-external-secrets`
  - `tilt up -- --1password-connect`

## Keep Docs and Code in Sync

When behavior changes, update docs in the same change set.

Minimum sync targets:
- Route/API changes: `docs/content/docs/cli.md`.
- CLI command/flag changes: `docs/content/docs/cli.md`.
- User-visible workflow changes: relevant docs pages in `docs/content/docs/` or `docs/content/tutorials/`.

## Anti-Patterns to Avoid

- Fat handlers with business logic mixed into request parsing.
- Duplicated validation logic across handlers.
- Implicit behavior that is not reflected in docs or tests.
- Large refactors without test coverage expansion.
- New complexity that does not materially improve maintainability.

## Definition of Done

A change is done only when all are true:
1. Implementation follows architecture ownership boundaries.
2. Tests are added or updated for changed behavior.
3. Required docs are updated in the same change.
4. Required validation commands pass.
5. Simplicity and maintainability are improved or at least not regressed.
