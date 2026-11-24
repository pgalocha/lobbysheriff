# caveman

Modo de resposta conciso para o Claude Code: corta o texto de enchimento das respostas e
mantém intacto o conteúdo técnico (código, comandos, caminhos, erros). Responde na língua
do utilizador e não se aplica a commits, código, documentação nem PRs.

**Ativar:** `/caveman` numa sessão. **Desativar:** "stop caveman" ou "normal mode".

> Só reduz o texto das respostas (tokens de saída). Não reduz o que o agente lê
> (ficheiros, output de comandos, histórico), que é a maior parte do consumo numa sessão.

## Origem e licença

Cópia sem alterações de [`skills/caveman/SKILL.md`](https://github.com/JuliusBrussee/caveman/blob/47c61be2d10a354f0c5cbe2b90959d24c06b16c6/skills/caveman/SKILL.md)
do projeto [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman), commit
`47c61be`. Licença Apache-2.0: ver [`LICENSE`](LICENSE) e [`NOTICE`](NOTICE) nesta pasta.
Esta licença aplica-se só a esta pasta, não ao resto do repositório.

Só foi copiada a skill principal. Não estão incluídos o plugin, os hooks (precisam de Node),
nem as variantes `/caveman ultra` e `/caveman wenyan` referidas no `SKILL.md`.

## Atualizar

Não editar o `SKILL.md` à mão: copiar de novo a partir de um commit do original e atualizar
o commit indicado acima.

```bash
SHA=<commit>
curl -fsS https://raw.githubusercontent.com/JuliusBrussee/caveman/$SHA/skills/caveman/SKILL.md \
  -o .claude/skills/caveman/SKILL.md
```
