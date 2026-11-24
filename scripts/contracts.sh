#!/usr/bin/env bash
# Tarefas dos contratos OpenAPI (contracts/openapi/*.yaml).
# Corre dentro do Dev Container (tem Node e Go) e na CI.
#
#   scripts/contracts.sh lint              valida os contratos (Redocly)
#   scripts/contracts.sh postman           gera as coleções Postman (Portman)
#   scripts/contracts.sh check-postman     falha se as coleções estiverem desatualizadas
#   scripts/contracts.sh breaking <base>   falha se houver breaking changes face a <base> (oasdiff)
#   scripts/contracts.sh newman [nome]     corre as coleções contra a API local (Newman)
set -euo pipefail

# Versões fixas: atualizar aqui (e no devcontainer, se aplicável).
REDOCLY_VERSION=2.77.0
PORTMAN_VERSION=1.35.2
NEWMAN_VERSION=6.2.3
OASDIFF_VERSION=v1.39.0

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTRACTS_DIR="contracts/openapi"
POSTMAN_DIR="postman"
cd "$ROOT"

contracts() {
  ls "$CONTRACTS_DIR"/*.yaml
}

lint() {
  npx --yes "@redocly/cli@${REDOCLY_VERSION}" lint $(contracts)
}

# Gera postman/<serviço>.postman_collection.json com testes de contrato
# (status, content-type, schema) e remove os ids aleatórios, para o ficheiro
# só mudar quando o contrato muda.
postman() {
  local spec name out
  for spec in $(contracts); do
    name="$(basename "$spec" .yaml)"
    out="$POSTMAN_DIR/$name.postman_collection.json"
    npx --yes "@apideck/portman@${PORTMAN_VERSION}" \
      --local "$spec" \
      --output "$out" \
      --portmanConfigFile "$POSTMAN_DIR/portman-config.json" \
      --syncPostman false \
      --includeTests true >/dev/null
    node -e '
      const fs = require("fs");
      const file = process.argv[1];
      const strip = (v) => Array.isArray(v) ? v.map(strip)
        : v && typeof v === "object"
          ? Object.fromEntries(Object.entries(v)
              .filter(([k]) => !["id", "_postman_id", "_exporter_id"].includes(k))
              .map(([k, x]) => [k, strip(x)]))
          : v;
      fs.writeFileSync(file, JSON.stringify(strip(JSON.parse(fs.readFileSync(file, "utf8"))), null, 2) + "\n");
    ' "$out"
    echo "✔ $out"
  done
}

check_postman() {
  postman
  if ! git diff --exit-code --stat -- "$POSTMAN_DIR"; then
    echo "::error::As coleções Postman não correspondem aos contratos. Corre 'scripts/contracts.sh postman' e faz commit."
    exit 1
  fi
}

breaking() {
  local base="${1:?uso: contracts.sh breaking <commit-base>}"
  local spec tmp status=0
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  for spec in $(contracts); do
    if ! git cat-file -e "${base}:${spec}" 2>/dev/null; then
      echo "• $spec é novo (não existe em $base): sem comparação."
      continue
    fi
    git show "${base}:${spec}" >"$tmp/base.yaml"
    echo "• $spec"
    if ! go run "github.com/oasdiff/oasdiff@${OASDIFF_VERSION}" breaking "$tmp/base.yaml" "$spec" --fail-on ERR; then
      echo "::error file=$spec::Breaking change no contrato. Se for intencional, cria uma nova versão da API em vez de alterar a existente."
      status=1
    fi
  done
  return "$status"
}

newman() {
  local only="${1:-}" collection
  for collection in "$POSTMAN_DIR"/*.postman_collection.json; do
    [ -z "$only" ] || [ "$(basename "$collection" .postman_collection.json)" = "$only" ] || continue
    npx --yes "newman@${NEWMAN_VERSION}" run "$collection" \
      --environment "$POSTMAN_DIR/local.postman_environment.json" \
      --env-var "baseUrl=${BASE_URL:-http://localhost:8080}"
  done
}

case "${1:-}" in
  lint) lint ;;
  postman) postman ;;
  check-postman) check_postman ;;
  breaking) shift; breaking "$@" ;;
  newman) shift; newman "$@" ;;
  *)
    sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
