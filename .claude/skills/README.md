# Skills dos agentes de IA

Skills do projeto para o Claude Code. Cada subpasta com um `SKILL.md` é
descoberta automaticamente quando se trabalha neste repositório.

## Estrutura

```
.claude/skills/
└── <nome-da-skill>/          # kebab-case, igual ao campo `name`
    ├── SKILL.md              # obrigatório: instruções + frontmatter
    ├── scripts/              # opcional: scripts que a skill executa
    ├── references/           # opcional: documentação lida só quando precisa
    └── assets/               # opcional: templates, ficheiros de exemplo
```

## Modelo de `SKILL.md`

```markdown
---
name: nome-da-skill
description: O que faz e QUANDO usar. É isto que o agente lê para decidir
  se carrega a skill, por isso inclui as palavras e situações que a devem ativar.
---

# Nome da Skill

## Quando usar
- ...

## Passos
1. ...

## Regras
- ...
```

## Convenções

- **`description` é a parte mais importante**: deve dizer claramente quando a
  skill se aplica (ex.: "Usar ao criar uma migration no go-stats-api").
- Manter o `SKILL.md` curto; detalhes longos vão para `references/` e são
  referenciados a partir do `SKILL.md`.
- Scripts em `scripts/` devem ser determinísticos e não pedir input interativo.
- **Nunca** colocar secrets (ex.: `FACEIT_API_KEY`) numa skill — usar variáveis
  do `.env`.
- Uma skill = uma tarefa. Se a `description` precisa de "e também…", divide.

## Índice

| Skill | Descrição |
|---|---|
| [`regras-desenvolvimento`](regras-desenvolvimento/) | Regras obrigatórias para agentes: Git decidido pelo utilizador, nada instalado no host, secrets, contratos, migrations, Go, CI. |
| [`caveman`](caveman/) | Respostas concisas para gastar menos tokens de saída; ativa com `/caveman`. Cópia da skill de [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman) (Apache-2.0). |
