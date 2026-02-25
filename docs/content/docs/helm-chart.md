---
title: Helm Chart
weight: 60
next: secrets
prev: configuration
---

Configuration reference for the ClawMachine Helm chart.

## Installation

```bash
helm repo add clawmachine https://charts.clawmachine.dev
helm install clawmachine clawmachine/clawmachine
```

## Values

Chart values documentation will be added as the Helm chart is developed. Key configuration areas will include:

- **Image** — Container image and tag
- **Resources** — CPU and memory limits
- **Ingress** — External access configuration
- **Bot defaults** — Default settings for each bot type

## Dependencies

### External Secrets Operator

The chart includes the [External Secrets Operator](https://external-secrets.io/) as an optional dependency for managing secrets from external providers (AWS Secrets Manager, Vault, etc.).

It is **disabled by default**. To enable it:

```yaml
# values.yaml
external-secrets:
  enabled: true
```

Or via Helm install:

```bash
helm install clawmachine clawmachine/clawmachine \
  --set external-secrets.enabled=true
```

When enabled, you can create `ExternalSecret` and `SecretStore` resources to sync secrets into your cluster. See the [External Secrets getting started guide](https://external-secrets.io/main/introduction/getting-started/) for configuration details.
