# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Projeto

LobbySheriff: cockpit de estatísticas FACEIT (CS2) para grupos de amigos ("lobbies"), com
rankings e o "LobbySheriff da Semana". Monorepo de microsserviços, *Database per Service*.
Fase atual: **Fase 1, `services/go-stats-api`** (Go 1.27 + Fiber v3). `php-players-api`,
`frontend` (Vue 3) e `gateway` (Nginx) ainda não existem.

**Regras obrigatórias:** [`.claude/skills/regras-desenvolvimento/SKILL.md`](.claude/skills/regras-desenvolvimento/SKILL.md).
As mais importantes:
- **Git é decidido pelo utilizador.** Nunca criar branches, `git add`, commit, push ou PR sem
  pedido expresso para essa ação. Deixar as alterações no working tree.
- **Nada instalado no host** (Ubuntu/WSL só tem Git e Docker). Go, Node e afins só no Dev
  Container ou em `docker run --rm` com versão fixa.
- Documentação, comentários e mensagens em português de Portugal; commits e identificadores
  em inglês (Conventional Commits).

## Comandos

Dentro do Dev Container (`.devcontainer/`, tem Go, golangci-lint, migrate, Node, gh):

```bash
cd services/go-stats-api
go run ./cmd/server                                   # API em :8080
go test -race ./...                                   # todos os testes
go test ./internal/server -run TestResponsesMatchContract   # um teste
golangci-lint run ./...
go generate ./...                                     # regenera internal/api a partir do contrato
go run ./cmd/server migrate up | down [N] | version   # migrations

scripts/contracts.sh lint | postman | newman | breaking <base-sha>   # na raiz
```

No host (terminal Ubuntu, raiz do repo). `docker compose` corre **aqui**, não dentro do Dev
Container (os bind mounts usam caminhos do host):

```bash
cp .env.example .env
docker compose up -d --build --wait     # MySQL, Redis, migrations, API, Swagger UI
curl localhost:8080/health              # Swagger UI em http://localhost:8090
```

Sem Dev Container, o toolchain corre em containers descartáveis, ex.:
`docker run --rm -v "$PWD":/repo -w /repo/services/go-stats-api golangci/golangci-lint:v2.15.0 go test ./...`
(monta a raiz do repo: `go generate` lê o contrato em `contracts/`).

## Arquitetura

**Contract-first.** `contracts/openapi/go-stats-api.yaml` é a fonte de verdade da API:
- `internal/api/api.gen.go` é gerado pelo oapi-codegen (`go tool`, fixado no `go.mod`):
  tipos, `ServerInterface` e o contrato embebido (`GetSpec`). Nunca editar à mão.
- `internal/server/handlers.go` implementa `api.ServerInterface` (falha a compilar se faltar
  uma operação); as rotas são registadas por `api.RegisterHandlers`.
- `internal/server/validation.go`: middleware kin-openapi valida cada pedido contra o contrato
  (400 com detalhes); todos os erros seguem o schema `Error`.
- `internal/server/server_test.go` valida cada resposta contra o contrato e exige que as rotas
  do Fiber e as do contrato coincidam.
- `postman/<serviço>.postman_collection.json` é gerado pelo Portman (`scripts/contracts.sh
  postman`) com testes de contrato; o Newman corre-o contra a stack real.
- Ordem ao mudar a API: contrato, lint, `go generate`, handlers, coleção Postman. Os quatro
  mudam no mesmo commit. Breaking changes são bloqueadas pela CI (oasdiff).

**go-stats-api** (`cmd/server/main.go`): o binário tem subcomandos: sem argumentos arranca o
servidor; `healthcheck` (usado pelo `HEALTHCHECK` da imagem distroless, sem curl); `migrate`.
Config só por variáveis de ambiente (`internal/config`, falha no arranque se faltar algo).
`internal/health` corre verificações em paralelo com timeout; `/livez` não depende de
dependências, `/readyz` e `/health` verificam MySQL e Redis.

**Migrations**: `services/go-stats-api/migrations/*.sql` (golang-migrate), embebidas no
binário. Uma migration existente no branch de destino nunca se edita; a CI verifica isto, a
ordem das versões e os pares up/down (`.github/scripts/check-migrations.sh`), e corre
up/down/up contra MySQL 8.4 com a imagem real. No Compose, o serviço `go-stats-migrate`
aplica-as antes de a API arrancar; em Kubernetes será um Job.

**Infra local**: `docker-compose.yml` tem `name: lobbysheriff` fixo, para o Dev Container
(override em `.devcontainer/compose.devcontainer.yml`, serviço `dev`) e o `docker compose`
do host partilharem a mesma rede e volumes. Hosts: `mysql`, `redis`, `go-stats-api`.
`infra/mysql/initdb.d/` cria um schema e um utilizador por serviço (só na primeira
inicialização do volume; `docker compose down -v` para recriar). Dados de outros serviços
(ex.: `lobby_id` do PHP) não têm FK.

**CI** (`.github/workflows/`): `go-stats-api.yml` (testes, lint, código gerado, Newman,
migrations) e `contracts.yml` (lint Redocly, breaking changes, coleções Postman). Correm em
todos os PRs para `master`/`staging`, sem filtro `paths`: o job `changes` faz os restantes
ficarem *skipped* quando o serviço não muda, para poderem ser checks obrigatórios. Actions
fixadas por SHA; versões de ferramentas sempre fixas (e com pelo menos duas semanas).

**Deploy**: `k8s/` com Kustomize (`base/` + `overlays/{staging,production}`). `config.env`
versionado (ConfigMap); `secrets.env` ignorado pelo Git e gerado no CI a partir dos GitHub
Environments. A `FACEIT_API_KEY` só existe no backend; testes e CI fazem mock da FACEIT.
