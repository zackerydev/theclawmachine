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

# Create OpenClaw directories
RUN mkdir -p /root/.openclaw/workspace /root/.openclaw/credentials

# Working directory
WORKDIR /root/.openclaw

# Gateway port
EXPOSE 18789

# Startup handled by Helm chart's startup-configmap
CMD ["openclaw", "gateway", "--port", "18789", "--allow-unconfigured"]
