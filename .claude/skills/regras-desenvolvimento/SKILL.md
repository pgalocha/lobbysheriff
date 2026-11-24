---
name: regras-desenvolvimento
description: Regras obrigatórias para agentes que trabalham no LobbySheriff. Usar SEMPRE
  antes de alterar código, criar ou apagar ficheiros, mexer no Git (branches, commits, push,
  PRs), instalar ferramentas, correr Docker, alterar contratos de API, migrations, CI,
  dependências ou secrets.
---

# Regras de desenvolvimento

Regras do projeto para qualquer agente de IA. Em caso de dúvida, pergunta ao utilizador
em vez de assumir.

## 1. Git: o utilizador decide

- **Nunca** criar, mudar ou apagar branches por iniciativa própria.
- **Nunca** fazer `git add`, `commit`, `push`, `merge`, `rebase` nem abrir PRs sem pedido
  expresso para essa ação. Um pedido para uma ação não autoriza as seguintes.
- Trabalhar no branch atual e deixar as alterações no working tree. No fim, dizer que
  ficheiros mudaram e deixar a decisão ao utilizador.
- Nunca reescrever histórico (`--amend`, `reset --hard`, force push) sem pedido expresso.
- Quando pedido um commit: mensagem em inglês no formato Conventional Commits
  (`feat(go-stats-api): …`, `chore: …`, `ci: …`).

## 2. Ambiente: nada instalado no host

- O host (Ubuntu no WSL) só tem Git e Docker. **Não instalar** Go, PHP, Node nem outras
  ferramentas de linguagem no host.
- Ferramentas de desenvolvimento vivem no Dev Container (`.devcontainer/`). Fora dele, usar
  containers descartáveis (`docker run --rm …`) com versões fixas.
- `docker compose` corre no terminal do host, na raiz do repositório. Dentro do Dev Container,
  os bind mounts apontam para caminhos que não existem no host.
- A infraestrutura local é a do `docker-compose.yml` (MySQL 8.4, Redis 8, Swagger UI).
  Hosts: `mysql`, `redis`, `go-stats-api`.

## 3. Secrets

- Nunca commitar `.env`, `k8s/**/secrets.env`, chaves nem tokens. Verificar o diff antes de
  sugerir um commit.
- A `FACEIT_API_KEY` só existe no backend. Nunca no frontend, em logs, em respostas da API,
  em skills ou em testes.
- Testes e CI fazem mock da FACEIT API; nunca usam a key real.
- Variáveis novas: acrescentar ao `.env.example` (sem valores reais) e, se não forem
  secretas, aos `k8s/overlays/*/config.env`.

## 4. Contratos de API (contract-first)

O contrato em `contracts/openapi/<serviço>.yaml` é a fonte de verdade. Ordem obrigatória:

1. Alterar o contrato.
2. `scripts/contracts.sh lint`
3. `go generate ./...` no serviço (regenera `internal/api/api.gen.go`; nunca editar à mão).
4. Implementar os handlers (`api.ServerInterface`).
5. `scripts/contracts.sh postman` (regenera a coleção; nunca editar à mão).
6. Contrato, código gerado e coleção mudam juntos.

- Sem breaking changes (remover rotas, campos, valores aceites; tornar obrigatório o que era
  opcional). Se forem necessárias: nova operação, e a antiga fica `deprecated: true`.
- Erros da API seguem o schema `Error` do contrato.
- Exceções ao lint vão para `.redocly.lint-ignore.yaml`, sempre com o motivo.

## 5. Migrations

- Migration já existente no branch de destino **nunca** se edita, apaga ou renomeia: cria-se
  uma nova.
- Criar com `migrate create -ext sql -dir migrations -seq <nome>`. Cada `.up.sql` tem um
  `.down.sql` que o desfaz por completo.
- Numeração acima da última do branch de destino (a CI verifica).
- Cada serviço só toca no seu schema; referências a dados de outro serviço não têm FK.

## 6. Go (`services/go-stats-api`)

- Antes de dar uma tarefa por concluída: `gofmt`, `go vet`, `go test -race ./...` e
  `golangci-lint run ./...` sem erros.
- Testes em tabela (`tests := []struct{…}`), com `t.Context()` e `httptest`.
- Respostas HTTP novas ficam cobertas pelos testes de contrato (`assertMatchesContract`).
- Configuração só por variáveis de ambiente (`internal/config`); falhar no arranque se faltar
  algo obrigatório.
- Logs com `slog` em JSON. Detalhes de erros internos vão para o log, não para a resposta.
- `/livez` não depende de MySQL/Redis; `/readyz` e `/health` sim.

## 7. Dependências, imagens e CI

- Versões sempre fixas: imagens (`mysql:8.4`, nunca `latest`), pacotes npm (`@x.y.z`),
  módulos Go, golangci-lint.
- Escolher versões publicadas há pelo menos duas semanas.
- GitHub Actions fixadas por SHA, com a versão em comentário (`@<sha> # v7.0.1`).
- Workflows sem filtro `paths` no trigger; jobs que não se aplicam ficam *skipped* através
  do job `changes`, para poderem ser checks obrigatórios.
- Novos ecossistemas (gomod, npm, docker) entram no `.github/dependabot.yml`.

## 8. Língua e estilo

- Documentação, comentários no código e mensagens de log/erro: português de Portugal.
- Commits, nomes de código e identificadores: inglês.
- Seguir o `.editorconfig` (LF, UTF-8; tabs em Go).

## 9. Antes de dizer que está feito

- Correr o que valida a alteração (testes, lint, `docker compose up --wait`, Newman) e
  reportar o resultado real. Se algo não foi testado, dizer qual.
- Não alterar ficheiros fora do âmbito pedido; se for necessário, explicar porquê.
- Nunca usar o branch `master` diretamente: tem ruleset (PR obrigatório).
