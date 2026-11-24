# syntax=docker/dockerfile:1

# ─── Build ───────────────────────────────────────────────────────
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/go-stats-api ./cmd/server

# ─── Runtime ─────────────────────────────────────────────────────
# Distroless: sem shell nem gestor de pacotes; corre como utilizador não-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/go-stats-api /go-stats-api

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/go-stats-api", "healthcheck"]

ENTRYPOINT ["/go-stats-api"]
