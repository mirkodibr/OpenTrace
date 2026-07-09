# syntax=docker/dockerfile:1.7

# Stage 1 — Build
FROM golang:1.22-alpine AS builder

ARG SERVICE
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Cache module downloads separately from source compilation.
# sdk/go/go.mod is required because the root module replaces the SDK
# dependency with the in-repo path (see root go.mod).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=sdk/go/go.mod,target=sdk/go/go.mod \
    go mod download

# Compile the requested service binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    go build \
        -trimpath \
        -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
        -o /bin/service \
        ./cmd/${SERVICE}

# Stage 2 — Runtime (distroless static — no shell, no libc, ~2MB base)
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=builder /bin/service /service

# Run as non-root (distroless nonroot = UID 65532)
USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/service"]
