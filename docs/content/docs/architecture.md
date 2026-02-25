---
title: Architecture
weight: 20
next: bots
prev: compatibility
---

How ClawMachine is built.

## Overview

ClawMachine is a Go control plane that serves HTML and runs Helm/Kubernetes operations.

Core traits:

- Single binary (`clawmachine`) for web and ops commands
- Embedded charts with remote OCI fallback support
- Server-rendered UI with HTMX partials
- Thin handlers and service-layer Kubernetes logic

## Stack

| Layer | Technology |
|-------|------------|
| Server | Go stdlib `net/http` |
| Templates | `html/template` |
| Interactivity | HTMX + small vanilla JS |
| Styling | Bootstrap + project CSS |
| Deployments | Helm v4 SDK + Kubernetes API |
| Secrets | ESO CRDs + Kubernetes Secrets |

## Runtime Subsystems

- `internal/routes.go`: central route contract
- `internal/handler`: HTTP handling for bots/secrets/network/onboarding
- `internal/service`: Helm, K8s, secrets, backup, logs, workspace
- `internal/onboarding`: canonical onboarding profiles + compile engine
- `internal/botenv`: bot env var registry for UI/API

## Route Surface

Serve mode includes:

- bot lifecycle APIs (`/bots/*`)
- onboarding/profile helpers (`/api/onboarding/*`, `/api/botenv`, `/api/models`)
- provider and ExternalSecret settings pages/API

## Security Model (Current)

- middleware chain: request logging, security headers, body limits
- static file serving without directory listing
- no built-in auth; expected access via port-forward or auth proxy
