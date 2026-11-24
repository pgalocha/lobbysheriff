# go-stats-api

Sincronização com a FACEIT, snapshots, partidas, rankings e LobbySheriff da Semana.

## Rotas

Definidas no contrato [`contracts/openapi/go-stats-api.yaml`](../../contracts/openapi/go-stats-api.yaml)
(ver [contracts/README.md](../../contracts/README.md)). Documentação interativa em http://localhost:8090.

| Rota | Descrição |
|---|---|
| `GET /livez` | Liveness: o processo está vivo (não verifica dependências) |
| `GET /readyz` | Readiness: 200 se MySQL e Redis estiverem disponíveis, 503 se não |
| `GET /health` | Estado de cada dependência em JSON |

## Desenvolvimento (dentro do Dev Container)

```bash
cd services/go-stats-api
go run ./cmd/server                # arranca a API em :8080
go generate ./...                  # regenera internal/api a partir do contrato
go test ./...
golangci-lint run ./...
```

## Migrations

Ficheiros SQL em [`migrations/`](migrations/), embebidos no binário.

```bash
go run ./cmd/server migrate up         # aplica as pendentes
go run ./cmd/server migrate down [N]   # reverte as últimas N (omissão: 1)
go run ./cmd/server migrate version    # versão atual do schema

migrate create -ext sql -dir migrations -seq <nome>   # nova migration
```

Com `docker compose up`, o serviço `go-stats-migrate` aplica as migrations antes de a API
arrancar. Em Kubernetes, correm num Job antes do deploy (`go-stats-api migrate up`).

Regras:

- Uma migration já aplicada em staging ou produção **nunca** se edita: cria-se outra.
- Cada `.up.sql` tem o seu `.down.sql`, que a desfaz por completo.
- Se uma migration falhar a meio, o schema fica `dirty`: corrigir à mão e usar
  `migrate force <versão>`.
