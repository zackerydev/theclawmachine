FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src
RUN git clone --depth 1 --branch main \
    https://github.com/sipeed/picoclaw.git .

RUN make deps && make build \
    && cp build/picoclaw /picoclaw

FROM alpine:3.23

ARG MISE_VERSION=v2026.2.21
RUN apk add --no-cache ca-certificates tzdata wget

RUN MISE_VERSION=${MISE_VERSION} MISE_INSTALL_PATH=/usr/local/bin/mise MISE_INSTALL_HELP=0 \
    sh -c "$(wget -qO- https://mise.run)" \
    && /usr/local/bin/mise --version

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
