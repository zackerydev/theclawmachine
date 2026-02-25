---
title: Network Isolation
weight: 3
---

This tutorial covers current network controls for bot charts.

## Step 1: Configure network settings during install

In step 1 of install wizard:

- toggle ingress
- toggle allow-all egress
- if egress is disabled, add allowed domains (one per line)

## Step 2: Verify policy resources

```bash
kubectl get networkpolicy -n claw-machine
kubectl get ciliumnetworkpolicy -n claw-machine
```

`CiliumNetworkPolicy` appears when `networkPolicy.useCilium=true`.

## Step 3: Check network flow UI

Open bot detail -> **Network** tab.

The panel calls `GET /bots/{name}/network` and shows allowed/blocked requests.
