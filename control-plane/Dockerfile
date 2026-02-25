FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o clawmachine ./cmd/clawmachine

FROM alpine:3.23
WORKDIR /app
RUN addgroup -S clawmachine \
    && adduser -S -G clawmachine -H -s /sbin/nologin clawmachine
COPY --from=builder /app/clawmachine /app/clawmachine
COPY templates /app/templates
COPY static /app/static
USER clawmachine
ENTRYPOINT ["/app/clawmachine"]
