# IPAlpha — ferramentas para desenvolvedores

Este repositório é o `.github` da organização **ipalpha-dev**: a página da organização
(`profile/`) e a ferramenta `ipalpha` (Go, um único binário para Windows, macOS e Linux).

## Setup (copie e cole no terminal)

**macOS, Linux, WSL**

```sh
curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.ps1 | iex
```

O setup verifica as ferramentas (Git, Node.js 20+, Docker; instala com winget/brew/apt se você
aceitar), entra no GitHub (chave SSH **ou** login pelo navegador com o GitHub CLI), clona os
repositórios, cria os `.env`, pergunta o primeiro acesso e qual IA usar, e gera os comandos abaixo.
Pode rodar de novo a qualquer momento: nada que você configurou é sobrescrito.

Pré-requisito: ser membro da organização `ipalpha-dev` no GitHub.

## Dia a dia

No Windows use `.\run`, `.\pull`… (PowerShell/cmd) ou `./run.cmd` (Git Bash).

| Comando | Faz |
| --- | --- |
| `./run` | Sobe MongoDB, Redis, RabbitMQ e Mailpit, instala dependências e abre o painel com cada API e web app. Porta ocupada? O serviço muda para uma livre nesta execução e tudo continua funcionando. `?` no painel mostra as teclas. |
| `./status` · `./logs <serviço> -f` · `./stop` | O que está rodando e em que porta · logs · para a infra |
| `./pull` | Atualiza a ferramenta, os repositórios e adiciona chaves novas aos `.env` |
| `./publish` | Repositórios alterados → a IA escolhida propõe versão + mensagem (ou você digita) → commit/push → npm/imagem → `deployment` |
| `./feature new <slug>` | Ambiente de feature isolado com preview público. Guia: [docs/local-development.md](docs/local-development.md#feature-environments-feature) |
| `./doctor` | Diz o que falta ou está quebrado e como resolver. `--copy` copia o relatório |
| `./ipalpha config` · `./ipalpha ai` | Muda idioma, IA/modelo, web apps, portas, runtime |

Qualquer erro mostra a causa, as últimas linhas, como resolver e o caminho do log completo
(`.ipalpha/logs/`).

## Manter esta ferramenta

```sh
go test ./...          # unitários + ponta a ponta (sem rede, sem Docker)
go run ./cmd/ipalpha   # rodar sem instalar
```

Código em `internal/` (um pacote por assunto), catálogo de repositórios e portas em
`internal/catalog`, textos nos 5 idiomas em `internal/i18n` (um teste falha se faltar algum),
arquivos gerados no workspace em `internal/assets/files`. O TeamCity
(`Tooling — Test and release`, em `deployment/.teamcity`) testa no Linux, compila para Windows e macOS e publica
os binários a cada push em `master`; os desenvolvedores recebem no próximo `./pull`. Sem GitHub Actions.
Referência completa: [docs/local-development.md](docs/local-development.md).
