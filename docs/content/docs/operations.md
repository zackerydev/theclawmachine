---
title: Operations
weight: 90
next: troubleshooting
prev: secrets
---

You installed it. Now keep it running.

## Health Checks

ClawMachine exposes `GET /health` which returns `200 OK` when the server is up. Use it for Kubernetes liveness and readiness probes (the Helm chart sets these up automatically).

Each bot has its own health endpoint:

| Bot | Health Endpoint |
|-----|----------------|
| PicoClaw | `GET /health` on port 18790 |
| IronClaw | `GET /api/health` on port 3000 |

The dashboard shows bot status based on the Helm release state. If a bot's pod is crashing, the status will reflect it.

## Monitoring

ClawMachine doesn't ship its own metrics endpoint (yet). For now, monitor at the Kubernetes level:

```bash
# Pod status
kubectl get pods -n claw-machine

# Resource usage (requires metrics-server)
kubectl top pods -n claw-machine

# Events — the first place to look when something's wrong
kubectl get events -n claw-machine --sort-by=.lastTimestamp
```

For bot namespaces, same commands with the appropriate namespace.

## Upgrading ClawMachine

```bash
clawmachine upgrade
```

This upgrades the control plane only. Your bots are independent Helm releases — they keep running untouched. If a new ClawMachine version ships updated bot charts, existing bots don't auto-upgrade. You'd upgrade them individually through the dashboard or `helm upgrade`.

Manual Helm fallback:

```bash
helm repo update
helm upgrade clawmachine clawmachine/clawmachine -n claw-machine
```

{{< callout type="info" >}}
Check the [changelog](https://github.com/zackerydev/theclawmachine/releases) before upgrading. Breaking changes happen. We try to document them.
{{< /callout >}}

## Upgrading Bots

From the dashboard, navigate to a bot's detail page and use the upgrade option. Or via Helm:

```bash
helm upgrade <bot-release-name> -n <namespace>
```

Since bot charts are embedded in the ClawMachine binary, upgrading a bot to a newer chart version requires upgrading ClawMachine first.

## Backups

ClawMachine itself is stateless — it reads everything from the Kubernetes API. No database to back up. If you lose the cluster, reinstall ClawMachine and re-create your bots.

What *you* should back up:

- **1Password vault** — already handled by 1Password
- **Bot data** — if bots use PVCs (persistent volumes), back those up with your cluster's backup solution (Velero, etc.)
- **Custom Helm values** — keep your `values.yaml` files in version control

## Scaling

The ClawMachine control plane is a single deployment. You can bump `replicaCount` in the Helm values if you need redundancy, but honestly — it's a dashboard. One replica is fine unless you have an SRE team that won't let you sleep otherwise.

Bots scale independently through their own Helm values.

## Security Notes

- ClawMachine uses the pod's service account to talk to the Kubernetes API. It needs permissions to manage Helm releases and (optionally) ESO CRDs.
- The middleware stack adds `Content-Security-Policy`, `X-Frame-Options`, `X-Content-Type-Options`, and body size limits.
- Connect tokens are stored as Kubernetes Secrets, never echoed back to the UI.
- Static file serving has directory listing disabled.

{{< callout type="warning" >}}
ClawMachine has broad Kubernetes permissions by design — it manages Helm releases across the cluster. Don't expose the dashboard to the internet without authentication in front of it (Ingress + auth proxy, VPN, etc.).
{{< /callout >}}

## Uninstalling

### Remove a bot

From the dashboard detail page, or:

```bash
helm uninstall <bot-name> -n <namespace>
```

### Remove ClawMachine

```bash
helm uninstall clawmachine -n claw-machine
```

Bots survive this. They're separate releases. Remove them first if you want a clean slate.

### Remove everything

```bash
# Uninstall all bots first, then:
helm uninstall clawmachine -n claw-machine
kubectl delete namespace claw-machine
```
