# Build IronClaw binary so toolbox can be built in CI/release without local image state.
FROM rust:1.92-slim-bookworm AS ironclaw-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

ARG IRONCLAW_VERSION=main
RUN git clone --depth 1 --branch ${IRONCLAW_VERSION} \
    https://github.com/nearai/ironclaw.git .

RUN cargo build --release --no-default-features --features postgres \
    && strip target/release/ironclaw \
    && test -f target/release/ironclaw

FROM debian:bookworm-slim

# System packages + PostgreSQL client (pg_dump, psql for backups)
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl git wget jq vim-tiny net-tools dnsutils iputils-ping \
    ca-certificates gnupg build-essential python3 python3-pip \
    postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Node.js 22.x
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# OpenClaw CLI
RUN npm install -g openclaw 2>/dev/null || echo "⚠ openclaw not on npm"

# PicoClaw binary
ARG PICOCLAW_VERSION=v0.1.1
ARG TARGETARCH
RUN wget -q -O /usr/local/bin/picoclaw \
    "https://github.com/sipeed/picoclaw/releases/download/${PICOCLAW_VERSION}/picoclaw-linux-${TARGETARCH}" \
    && chmod +x /usr/local/bin/picoclaw \
    || echo "⚠ picoclaw download failed"

# IronClaw binary (built in this Dockerfile for release reproducibility)
COPY --from=ironclaw-builder /src/target/release/ironclaw /usr/local/bin/ironclaw

# Create non-root user
RUN userdel debian 2>/dev/null || true && useradd -u 1000 -m -s /bin/bash claw

WORKDIR /workspace
RUN chown claw:claw /workspace

# Verify installs
RUN echo "=== Toolbox Contents ===" \
    && (openclaw --version 2>/dev/null || echo "openclaw: not found") \
    && (picoclaw --version 2>/dev/null || echo "picoclaw: not found") \
    && (ironclaw --version 2>/dev/null || echo "ironclaw: not found") \
    && echo "node $(node --version)" \
    && echo "npm $(npm --version)"

CMD ["sleep", "infinity"]
