#!/usr/bin/env bash
# Verifica as regras das migrations entre um commit base e o HEAD:
#   1. Migrations que já existem no base não podem ser editadas, apagadas nem renomeadas.
#   2. Migrations novas têm de ter versão superior à última do base
#      (apanha dois branches que criaram a mesma versão em paralelo).
#   3. No HEAD, cada versão tem exatamente um .up.sql e um .down.sql.
#
# Uso: check-migrations.sh <base-sha> <diretório-das-migrations>
# Sem base (primeiro push de um branch), só corre a regra 3.
set -euo pipefail

base="${1:-}"
dir="${2:?uso: check-migrations.sh <base-sha> <diretório>}"
dir="${dir%/}"
errors=0

fail() {
  echo "::error::$*"
  errors=$((errors + 1))
}

# Versão (sem zeros à esquerda) de cada ficheiro NNNNNN_nome.up.sql em stdin.
up_versions() {
  sed -nE 's#^(.*/)?([0-9]+)_[^/]*\.up\.sql$#\2#p' | sed -E 's/^0+([0-9])/\1/' | sort -n
}

# ─── Regra 3: pares up/down e versões únicas no HEAD ─────────────
shopt -s nullglob
files=("$dir"/*.sql)
if [ ${#files[@]} -eq 0 ]; then
  fail "nenhuma migration encontrada em $dir"
fi
for f in "${files[@]}"; do
  name="$(basename "$f")"
  if [[ ! "$name" =~ ^[0-9]{6}_[a-z0-9_]+\.(up|down)\.sql$ ]]; then
    fail "nome inválido: $name (esperado NNNNNN_nome.up.sql / .down.sql)"
  fi
done
dups="$(printf '%s\n' "${files[@]}" | sed -nE 's#.*/([0-9]+)_[^/]*\.(up|down)\.sql$#\1 \2#p' | sort | uniq -d)"
if [ -n "$dups" ]; then
  fail "versões repetidas (dois ficheiros com o mesmo número): $(echo "$dups" | tr '\n' ' ')"
fi
for up in "$dir"/*.up.sql; do
  [ -f "${up%.up.sql}.down.sql" ] || fail "$(basename "$up") não tem o .down.sql correspondente"
done
for down in "$dir"/*.down.sql; do
  [ -f "${down%.down.sql}.up.sql" ] || fail "$(basename "$down") não tem o .up.sql correspondente"
done

# ─── Regras 1 e 2: comparação com o base ─────────────────────────
if [ -z "$base" ] || [[ "$base" =~ ^0+$ ]]; then
  echo "Sem commit base: regras de imutabilidade e ordem não verificadas."
elif ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
  fail "commit base $base não encontrado (o checkout precisa de fetch-depth: 0)"
else
  # --no-renames: um rename aparece como D + A, e o D é apanhado pela regra 1.
  while IFS=$'\t' read -r status path; do
    [ -n "$status" ] || continue
    case "$status" in
      M*|D*|T*) fail "migration já existente foi alterada ou apagada: $path — cria uma nova migration em vez de a editar" ;;
    esac
  done < <(git diff --name-status --no-renames "$base" HEAD -- "$dir")

  base_max="$(git ls-tree --name-only "$base" -- "$dir/" | up_versions | tail -n1)"
  base_max="${base_max:-0}"
  while read -r v; do
    [ -n "$v" ] || continue
    if [ "$v" -le "$base_max" ]; then
      fail "migration nova com versão $v, mas o destino já vai na $base_max — renumera-a para uma versão superior"
    fi
  done < <(git diff --name-only --diff-filter=A --no-renames "$base" HEAD -- "$dir" | up_versions)

  echo "Base: $base (última versão $base_max)"
fi

if [ "$errors" -gt 0 ]; then
  echo "$errors problema(s) nas migrations."
  exit 1
fi
echo "Migrations OK: $(printf '%s\n' "${files[@]}" | up_versions | tail -n1) é a última versão."
