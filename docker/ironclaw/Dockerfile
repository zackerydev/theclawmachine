# --- Build stage ---
FROM rust:1.92-slim-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Clone IronClaw source
ARG IRONCLAW_VERSION=main
RUN git clone --depth 1 --branch ${IRONCLAW_VERSION} \
    https://github.com/nearai/ironclaw.git .

# Build release binary with only the postgres feature (no libsql needed in k8s)
RUN cargo build --release --no-default-features --features postgres \
    && strip target/release/ironclaw

# Verify the binary exists
RUN test -f target/release/ironclaw

# --- Runtime stage ---
FROM debian:bookworm-slim

# ca-certificates: TLS for outbound API calls (LLM providers, NEAR AI)
# tzdata: timezone support for heartbeat/cron scheduling
# postgresql-client: pg_dump/psql for database backup/restore
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata wget postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Non-root user with no login shell
RUN groupadd --system --gid 1000 ironclaw \
    && useradd --system --uid 1000 --gid ironclaw \
       --no-create-home --shell /usr/sbin/nologin ironclaw \
    && mkdir -p /home/ironclaw/.ironclaw/workspace \
    && chown -R ironclaw:ironclaw /home/ironclaw

COPY --from=builder --chown=ironclaw:ironclaw /src/target/release/ironclaw /usr/local/bin/ironclaw

USER ironclaw
ENV HOME=/home/ironclaw

# Gateway listens on port 3000
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q --spider http://localhost:3000/api/health || exit 1

# Workspace volume for persistent state (memory, documents, embeddings)
VOLUME ["/home/ironclaw/.ironclaw/workspace"]

ENTRYPOINT ["ironclaw"]
