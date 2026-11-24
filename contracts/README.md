# Contratos de API

Cada serviço tem um contrato OpenAPI 3.0 neste diretório. O contrato é a **fonte de verdade**:
o código, a validação dos pedidos, o Swagger UI e as coleções Postman derivam dele.

| Contrato | Serviço |
|---|---|
| [`openapi/go-stats-api.yaml`](openapi/go-stats-api.yaml) | `services/go-stats-api` |

## Alterar uma API

Dentro do Dev Container:

```bash
# 1. Editar o contrato
code contracts/openapi/go-stats-api.yaml

# 2. Validar o contrato
scripts/contracts.sh lint

# 3. Gerar o código Go (tipos + interface dos handlers)
cd services/go-stats-api && go generate ./... && cd -

# 4. Implementar os handlers novos: até lá, o código não compila
#    (internal/server/handlers.go tem de implementar api.ServerInterface)

# 5. Gerar a coleção Postman e testar contra a API local
scripts/contracts.sh postman
scripts/contracts.sh newman
```

Faz commit do contrato, do código gerado e da coleção juntos.

## O que garante o cumprimento do contrato

| Onde | Verificação |
|---|---|
| Compilação | Os handlers implementam a interface gerada do contrato |
| Runtime | Middleware valida cada pedido contra o contrato e responde 400 com os detalhes |
| `go test` | Cada resposta é validada contra o contrato (status, content-type, schema); as rotas da app e as do contrato têm de coincidir |
| CI · `contracts` | Lint (Redocly), *breaking changes* face ao branch de destino (oasdiff), coleções Postman atualizadas |
| CI · `go-stats-api` | Código gerado atualizado; coleção Postman corrida com o Newman contra a stack real |

## Breaking changes

Remover rotas, campos de resposta ou valores aceites, ou tornar obrigatório algo que era
opcional, parte os clientes (o `php-players-api` e o frontend). A CI bloqueia estas alterações.
Se forem mesmo necessárias, cria-se uma nova versão da operação e a antiga fica marcada como
`deprecated: true` até os clientes migrarem.

## Ferramentas

- **Swagger UI**: http://localhost:8090 (serviço `swagger-ui` do Compose). O *Try it out* chama a
  API em `localhost:8080`, que tem de ter `CORS_ALLOWED_ORIGINS=http://localhost:8090` no `.env`.
- **Postman**: importa `postman/<serviço>.postman_collection.json` e
  `postman/local.postman_environment.json`. A coleção é gerada pelo Portman com testes de
  contrato em cada pedido; não a edites à mão (a CI falha se não corresponder ao contrato).
- **Lint**: regras em [`redocly.yaml`](../redocly.yaml); exceções pontuais, com o motivo, em
  [`.redocly.lint-ignore.yaml`](../.redocly.lint-ignore.yaml).
