# syntax=docker/dockerfile:1
# stratos-api (Go). The runtime is debian-slim (NOT distroless) on purpose: the
# image ships `psql` (postgresql-client) so the config-as-code seed (deploy/seed)
# can run from this pod against STRATOS_DB_URL, alongside the Go binary.

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# Copy ONLY the Go source the binary needs (not the whole repo): no values overlays, seeds,
# docs, or tests reach the build context/image.
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/stratos-api ./cmd/api

FROM debian:bookworm-slim
# ca-certificates for outbound TLS; postgresql-client so the config-as-code seed
# (deploy/seed) can `psql` from this pod against STRATOS_DB_URL; bash for exec/debug.
RUN apt-get update \
 && apt-get install -y --no-install-recommends bash ca-certificates postgresql-client \
 && rm -rf /var/lib/apt/lists/*

# api config mount point.
RUN mkdir -p /opt/stratos/api

COPY --from=build /out/stratos-api /usr/local/bin/stratos-api

# Non-root.
RUN useradd -r -u 10001 -g root stratos
USER 10001

EXPOSE 8080 8081
ENTRYPOINT ["/usr/local/bin/stratos-api"]
