---
title: Local Kubernetes Clusters
weight: 130
next: architecture
prev: getting-started
---

ClawMachine runs on any Kubernetes cluster. For local development, you'll need a local cluster. Here are the most common options.

## OrbStack

[OrbStack](https://orbstack.dev/) is a fast, lightweight alternative to Docker Desktop on macOS. It includes a built-in single-node Kubernetes cluster and provides automatic local DNS for services (`<service>.<namespace>.svc.cluster.local`).

```bash
# Enable Kubernetes in OrbStack settings, then:
kubectl config use-context orbstack
```

ClawMachine's Tiltfile is configured to use OrbStack's local DNS for accessing the dashboard.

{{< callout type="info" >}}
See the [OrbStack Kubernetes docs](https://docs.orbstack.dev/kubernetes/) for setup details.
{{< /callout >}}

## Kind

[Kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) runs clusters using Docker containers as nodes. It's lightweight and widely used for local development and CI.

```bash
# Install
brew install kind

# Create a cluster
kind create cluster --name clawmachine

# Delete when done
kind delete cluster --name clawmachine
```

To access services, you'll need to set up port forwarding or an ingress controller.

{{< callout type="info" >}}
See the [Kind quick start guide](https://kind.sigs.k8s.io/docs/user/quick-start/) for full setup instructions.
{{< /callout >}}

## k3d

[k3d](https://k3d.io/) runs [k3s](https://k3s.io/) (a lightweight Kubernetes distribution) inside Docker containers. It's fast to start and has built-in support for local registries and load balancers.

```bash
# Install
brew install k3d

# Create a cluster with port mapping
k3d cluster create clawmachine -p "8080:80@loadbalancer"

# Delete when done
k3d cluster delete clawmachine
```

{{< callout type="info" >}}
See the [k3d usage guide](https://k3d.io/stable/usage/commands/k3d/) for configuration options.
{{< /callout >}}

## Docker Desktop

[Docker Desktop](https://www.docker.com/products/docker-desktop/) includes an optional single-node Kubernetes cluster. Enable it in Settings > Kubernetes.

```bash
# After enabling Kubernetes in Docker Desktop:
kubectl config use-context docker-desktop
```

{{< callout type="info" >}}
See the [Docker Desktop Kubernetes docs](https://docs.docker.com/desktop/features/kubernetes/) for setup details.
{{< /callout >}}

## Verifying Your Cluster

Regardless of which option you choose, verify your cluster is running:

```bash
kubectl cluster-info
kubectl get nodes
```

Once your cluster is ready, proceed to [Getting Started](../getting-started) to install ClawMachine.
