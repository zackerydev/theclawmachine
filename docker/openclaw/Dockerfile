# OpenClaw container image for ClawMachine
FROM node:22-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
  bash \
  ca-certificates \
  curl \
  git \
  && rm -rf /var/lib/apt/lists/*

# Install OpenClaw
ARG OPENCLAW_VERSION=2026.2.22
RUN npm install -g openclaw@${OPENCLAW_VERSION} \
  && openclaw --version

ARG MISE_VERSION=v2026.2.21
ARG MISE_SHA256_X64=97ad70c76034cd7a5e6ea2fb09b022da8267d105c9ac095192d3cd63aee9cc2f
ARG MISE_SHA256_ARM64=4587074544adf44cc817cfa43bd31ee66424d0bd1c7bf6e4632b73f611125b02
RUN set -eux; \
  arch="$(uname -m)"; \
  case "$arch" in \
    x86_64) mise_arch="x64"; mise_sha="${MISE_SHA256_X64}" ;; \
    aarch64|arm64) mise_arch="arm64"; mise_sha="${MISE_SHA256_ARM64}" ;; \
    *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
  esac; \
  mise_tar="mise-${MISE_VERSION}-linux-${mise_arch}.tar.gz"; \
  curl -fsSL "https://github.com/jdx/mise/releases/download/${MISE_VERSION}/${mise_tar}" -o /tmp/mise.tar.gz; \
  echo "${mise_sha}  /tmp/mise.tar.gz" | sha256sum -c -; \
  tar -xzf /tmp/mise.tar.gz -C /tmp; \
  install -m 0755 /tmp/mise/bin/mise /usr/local/bin/mise; \
  rm -rf /tmp/mise /tmp/mise.tar.gz; \
  /usr/local/bin/mise --version

# Create OpenClaw directories
RUN mkdir -p /root/.openclaw/workspace /root/.openclaw/credentials

# Working directory
WORKDIR /root/.openclaw

# Gateway port
EXPOSE 18789

# Startup handled by Helm chart's startup-configmap
CMD ["openclaw", "gateway", "--port", "18789", "--allow-unconfigured"]
