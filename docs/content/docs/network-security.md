---
title: Network Security
weight: 80
next: cli
prev: secrets
---

Each bot chart includes `NetworkPolicy` and optional `CiliumNetworkPolicy` resources.

## Defaults

Defaults are chart-specific:

- OpenClaw: `networkPolicy.egress=true`
- PicoClaw, IronClaw, BusyBox: `networkPolicy.egress=false`
- All: `networkPolicy.ingress=false`, `useCilium=false`, `allowedDomains=[]`

When Cilium CRDs are detected during install, ClawMachine auto-sets `networkPolicy.useCilium=true` unless explicitly set.

## DNS-Aware Egress with Cilium

When `useCilium=true` and `egress=false`, `allowedDomains` controls FQDN egress.

Example:

```yaml
networkPolicy:
  ingress: false
  egress: false
  useCilium: true
  allowedDomains:
    - "*.openai.com"
    - "*.anthropic.com"
    - "discord.com"
```

When `egress=true`, domain restrictions are not enforced.

## Dashboard and API

In install wizard step 1, configure:

- Allow ingress
- Allow all egress
- Allowed domains (one per line)

Per-bot settings can also be changed from bot detail (`Settings` tab) or via `PUT /bots/{name}`.

## Observability

`GET /bots/{name}/network` provides network flow summaries and powers the bot detail Network tab.

With Hubble available, ClawMachine shows:

- allowed and blocked external flows
- internal flows grouped separately
- destination, protocol/port, and DNS query context
