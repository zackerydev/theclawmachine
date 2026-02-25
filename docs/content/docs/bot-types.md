---
title: Bot Types
weight: 30
next: secrets
prev: architecture
---

Bot types are defined by embedded charts and `internal/botenv` metadata.

## OpenClaw

Defaults (`charts/openclaw/values.yaml`):

- image: `ghcr.io/zackerydev/openclaw:2026.2.21`
- service port: `18789`
- persistence: enabled (`5Gi`)
- network: ingress `false`, egress `true`

## PicoClaw

Defaults (`charts/picoclaw/values.yaml`):

- image: `ghcr.io/zackerydev/picoclaw:0.1.2`
- service port: `18790`
- persistence: disabled
- network: ingress `false`, egress `false`

## IronClaw

Defaults (`charts/ironclaw/values.yaml`):

- image: `ghcr.io/zackerydev/ironclaw:0.11.1`
- service port: `3000`
- built-in PostgreSQL with pgvector enabled by default
- backup mode default: `pgdump`

## BusyBox (Dev)

Defaults (`charts/busybox/values.yaml`):

- image: `ghcr.io/zackerydev/theclawmachine-toolbox:0.1.0`
- service port: `80`

BusyBox appears only when ClawMachine is running in dev mode.

## Env Registry API

`GET /api/botenv` returns bot env var metadata used by the install UI and integrations.
