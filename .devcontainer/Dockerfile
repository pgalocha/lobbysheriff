# Imagem base do Dev Container. Go, golangci-lint, kubectl e gh vêm das
# features em devcontainer.json; aqui fica só o que não tem feature oficial.
FROM mcr.microsoft.com/devcontainers/base:bookworm

ARG MIGRATE_VERSION=4.20.1

# Clientes MySQL e Redis para inspecionar a infraestrutura local.
RUN apt-get update \
    && apt-get install -y --no-install-recommends default-mysql-client redis-tools \
    && rm -rf /var/lib/apt/lists/*

# golang-migrate (binário oficial, inclui o driver mysql).
RUN arch="$(dpkg --print-architecture)" \
    && curl -fsSL "https://github.com/golang-migrate/migrate/releases/download/v${MIGRATE_VERSION}/migrate.linux-${arch}.tar.gz" \
       | tar -xz -C /usr/local/bin migrate \
    && migrate -version
