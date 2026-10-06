# IPAlpha — ferramentas para desenvolvedores

Este repositório é o `.github` da organização **ipalpha-dev**: guarda a página da
organização (`profile/`) e as ferramentas de desenvolvimento (`setup`, `lib/`,
`templates/`). Não é clonado no seu ambiente — o setup roda a partir de uma cópia
temporária e a apaga no fim, como no Cross.

## Setup (copie e cole no terminal)

```sh
bash <(curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/bootstrap.sh)
```

Pré-requisito: [chave SSH no GitHub](https://docs.github.com/pt/authentication/connecting-to-github-with-ssh)
(`ssh -T git@github.com`) e ser membro da organização `ipalpha-dev`.

O setup pergunta o idioma e a pasta (padrão `./IpAlpha`), instala as ferramentas,
clona os repositórios, cria os `.env`, e gera os comandos abaixo na pasta escolhida.

Sem clonar à mão? O comando acima já faz isso. Alternativa com clone manual:

```sh
git clone git@github.com:ipalpha-dev/.github.git /tmp/ipalpha-setup && /tmp/ipalpha-setup/setup
```

Layout gerado:

```text
IpAlpha/
├── core/           # shared-js, shared-ui, auth-api, auth-webapp, persons-api, organizations-api, projects-api, notifications-api,
│                   # dispatch-api, ai-api, developers-api, mordomia-webapp, developers-webapp
├── apps/           # apps fora do core (namespace próprio em produção): apps/forms/{forms-api,forms-webapp}
├── deployment/     # manifestos k8s (namespace ipalpha-core, imagens ghcr.io/ipalpha-dev/<ms>)
├── features/     # ./feature new <slug> (worktrees, um por feature)
├── run  publish  pull  feature  set-keys
└── .ipalpha/       # settings, compose, portas, mprocs (opt-in), scripts auxiliares
```

## Dia a dia

| Comando | Faz |
| --- | --- |
| `./run` | Infra (MongoDB, Redis, RabbitMQ) → instala dependências npm que faltam → os 5 serviços em background (runner embutido; use `IPALPHA_RUNNER=mprocs ./run` se preferir o painel) |
| `./pull` | Atualiza todos os repositórios, clona os novos, adiciona chaves novas nos `.env` e atualiza `.ipalpha/` a partir deste repositório |
| `./publish` | Repositórios alterados → IA escolhe versão + mensagem → commit/push → imagem `ghcr.io/ipalpha-dev/<ms>` (ou npm, para o shared-js) → atualiza `deployment/` |
| `./feature new <slug>` | Ambiente de feature isolado: `features/<slug>/` com worktrees em `feat/<slug>` a partir do último Core Deploy verde. Dentro dela, `./publish` publica um preview em `https://ipalpha-<slug>.kevyn.com.br` (+ `forms-`/`auth-ipalpha-<slug>`, caixa de códigos em `/mailbox`), válido por 72 h. `./feature list\|extend\|rebase\|reset\|destroy`. Guia: [docs/local-development.md](docs/local-development.md#feature-environments-feature) |
| `./set-keys` | Pergunta as chaves (SMS Barato, Comtele, superusuário) e grava nos `.env` locais |

`./run --help`, `./publish --help`, `./pull --help` mostram o uso completo.

## Manter estas ferramentas

Edite aqui, rode `tests/validate.sh`, faça push. Os desenvolvedores recebem as
mudanças no próximo `./pull`. Referência completa: [docs/local-development.md](docs/local-development.md).
