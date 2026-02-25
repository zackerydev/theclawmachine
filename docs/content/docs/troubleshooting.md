---
title: Troubleshooting
weight: 140
next: development
prev: advanced-setup
---

Things will go wrong. Here's how to fix them.

## Installation Issues

### "No Kubernetes cluster detected"

The installer can't reach a cluster. Either you don't have one, or `kubectl` isn't configured.

```bash
# Check if kubectl can talk to anything
kubectl cluster-info

# List available contexts
kubectl config get-contexts
```

If you need a local cluster, see [Local Clusters](../local-clusters).

### Install hangs or times out

The `clawmachine install` command waits up to 5 minutes for the deployment to roll out. If it times out:

```bash
# Check what's happening
kubectl -n claw-machine get pods
kubectl -n claw-machine describe pod <pod-name>
kubectl -n claw-machine logs <pod-name>
```

Common causes:
- **ImagePullBackOff** — can't pull the container image. Check your registry access.
- **Pending** — no node can schedule the pod. Check resource availability.
- **CrashLoopBackOff** — the container starts and dies. Check logs.

### Helm release already exists

```
Error: cannot re-use a name that is still in use
```

ClawMachine is already installed. Either uninstall first or use a different release name:

```bash
helm uninstall clawmachine -n claw-machine
# or
clawmachine install --name clawmachine-2
```

## Dashboard Issues

### Can't access the dashboard

```bash
# Is the pod running?
kubectl -n claw-machine get pods

# Port-forward
kubectl port-forward -n claw-machine svc/clawmachine 8080:80

# Then open http://localhost:8080
```

If using OrbStack, the service is available at `http://clawmachine.claw-machine.svc.cluster.local:80` automatically.

### Dashboard shows no bots

That's not a bug — you haven't installed any yet. Click "Install Bot."

If you *have* installed bots and they're not showing, check the namespace. Bots are installed in the `default` namespace. If you're looking in a different namespace, that's why.

## Bot Issues

### Bot install fails

Check the Helm error message in the dashboard. Common causes:

- **Chart loading error** — the embedded chart is corrupted. Rebuild the binary.
- **Resource quota exceeded** — the namespace has resource limits. Reduce the bot's resource requests.
- **PVC can't bind** — no StorageClass available. Disable persistence or configure a StorageClass.

### Bot pod is CrashLoopBackOff

```bash
kubectl logs <pod-name> -n claw-machine
kubectl describe pod <pod-name> -n claw-machine
```

For IronClaw specifically:
- **Missing database** — IronClaw needs PostgreSQL. Check `database.url` is set correctly.
- **Invalid auth token** — the gateway auth token is empty or wrong.
- **LLM provider error** — API key is missing or invalid.

### Bot health check failing

Each bot has a health endpoint. Test it directly:

```bash
# PicoClaw
kubectl port-forward svc/<release-name>-picoclaw 18790:18790
curl http://localhost:18790/health

# IronClaw
kubectl port-forward svc/<release-name>-ironclaw 3000:3000
curl http://localhost:3000/api/health
```

## Secrets Issues

### "External Secrets Operator is not installed"

ESO CRDs aren't in the cluster. Install ClawMachine with the `--external-secrets` flag:

```bash
clawmachine install --external-secrets
```

Or enable it in an existing installation:

```bash
helm upgrade clawmachine clawmachine/clawmachine \
  -n claw-machine \
  --set external-secrets.enabled=true
```

### SecretStore shows "Not Ready"

The 1Password Connect Server is unreachable or the token is invalid.

```bash
# Check the SecretStore status
kubectl describe secretstore onepassword-store -n claw-machine
```

Verify:
- The Connect Server URL is correct and reachable from inside the cluster
- The access token is valid
- The vault name matches a vault the token has access to

### ExternalSecret stuck in "Pending"

```bash
kubectl describe externalsecret <name> -n claw-machine
```

Common causes:
- SecretStore isn't ready (fix that first)
- The remote key doesn't match any 1Password item
- The property name doesn't match a field in the 1Password item

## Network Policy Issues

### Can't reach a bot from another pod

Network policies block traffic by default. Enable ingress:

```bash
curl -X PUT http://localhost:8080/bots/my-bot \
  -H "Content-Type: application/json" \
  -d '{"botType": "picoclaw", "values": {"networkPolicy": {"ingress": true}}}'
```

### Network policies aren't working at all

Your CNI doesn't support NetworkPolicy enforcement. Kind's default CNI (kindnet) doesn't enforce them. Install Cilium:

```bash
helm install cilium cilium/cilium --namespace kube-system --set operator.replicas=1 --set hubble.relay.enabled=true
```

See [Network Security](../network-security) for full Cilium setup.

## Still Stuck?

```bash
# The nuclear option: full cluster state dump
kubectl -n claw-machine get all
kubectl -n claw-machine get all
kubectl get networkpolicies -A
kubectl get externalsecrets -A
kubectl get secretstores -A
helm list -A
```

If none of this helps, [open an issue](https://github.com/zackerydev/theclawmachine/issues).
