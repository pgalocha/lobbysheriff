# Kubernetes

Manifests do LobbySheriff organizados com [Kustomize](https://kustomize.io/)
(incluído no `kubectl`).

```
k8s/
├── base/                     # Recursos comuns a todos os ambientes
│   └── kustomization.yaml
└── overlays/
    ├── staging/
    │   ├── kustomization.yaml
    │   ├── namespace.yaml    # lobbysheriff-staging
    │   ├── config.env        # config não sensível   → ConfigMap (versionado)
    │   └── secrets.env.example  # modelo dos secrets → Secret (NÃO versionado)
    └── production/
        └── (mesma estrutura)  # lobbysheriff-production
```

## Configuração vs. secrets

| Ficheiro | Conteúdo | Git | Gera |
|---|---|---|---|
| `config.env` | Hosts, portas, nomes de BD, intervalos | ✅ | ConfigMap `lobbysheriff-config` |
| `secrets.env` | API keys, passwords, tokens | ❌ (`.gitignore`) | Secret `lobbysheriff-secrets` |
| `secrets.env.example` | Só os nomes das variáveis | ✅ | — |

Os nomes das variáveis são os mesmos do `.env.example` da raiz. Ao adicionar uma
variável nova, atualizar o `.env.example`, os `config.env` e/ou os
`secrets.env.example`.

## Secrets no CI/CD

Os valores reais ficam nos **GitHub Environments** (`staging` e `production`).
O workflow de deploy escreve o `secrets.env` antes do build:

```yaml
- name: Gerar secrets.env
  run: |
    cat > k8s/overlays/${{ env.ENVIRONMENT }}/secrets.env <<EOF
    FACEIT_API_KEY=${{ secrets.FACEIT_API_KEY }}
    MYSQL_ROOT_PASSWORD=${{ secrets.MYSQL_ROOT_PASSWORD }}
    ...
    EOF
```

> Alternativa futura: Sealed Secrets ou External Secrets Operator, para não
> depender do CI para gerar o Secret.

## Comandos

```bash
# Pré-visualizar o que será aplicado
kubectl kustomize k8s/overlays/staging

# Aplicar
kubectl apply -k k8s/overlays/staging
```

Nos Deployments, carregar as variáveis com:

```yaml
envFrom:
  - configMapRef:
      name: lobbysheriff-config
  - secretRef:
      name: lobbysheriff-secrets
```
