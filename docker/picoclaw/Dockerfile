FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src
RUN git clone --depth 1 --branch main \
    https://github.com/sipeed/picoclaw.git .

RUN make deps && make build \
    && cp build/picoclaw /picoclaw

FROM alpine:3.23

ARG MISE_VERSION=v2026.2.21
ARG MISE_SHA256_X64=97ad70c76034cd7a5e6ea2fb09b022da8267d105c9ac095192d3cd63aee9cc2f
ARG MISE_SHA256_ARM64=4587074544adf44cc817cfa43bd31ee66424d0bd1c7bf6e4632b73f611125b02
RUN apk add --no-cache ca-certificates curl tzdata

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

COPY --from=builder /picoclaw /usr/local/bin/picoclaw

RUN addgroup -S picoclaw \
    && adduser -S -G picoclaw -H -s /sbin/nologin picoclaw \
    && mkdir -p /home/picoclaw/.picoclaw/workspace \
    && chown -R picoclaw:picoclaw /home/picoclaw

USER picoclaw
ENV HOME=/home/picoclaw
RUN picoclaw onboard

EXPOSE 18790

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:18790/health || exit 1

VOLUME ["/home/picoclaw/.picoclaw/workspace"]

ENTRYPOINT ["picoclaw"]
CMD ["gateway"]
