#!/usr/bin/env bash

ipalpha_lang="pt-BR"

ipalpha_i18n_init() {
  ipalpha_lang="${1:-pt-BR}"
  case "$ipalpha_lang" in
    pt|pt-BR|ptbr) ipalpha_lang="pt-BR" ;;
    en|en-US|enus) ipalpha_lang="en-US" ;;
    *) ipalpha_lang="pt-BR" ;;
  esac
}

ipalpha_msg() {
  local key="$1"
  case "$ipalpha_lang:$key" in
    pt-BR:choose_lang) echo "Idioma / Language: [1] Português (padrão)  [2] English" ;;
    en-US:choose_lang) echo "Language / Idioma: [1] Português (default)  [2] English" ;;
    pt-BR:invalid_choice) echo "Opção inválida." ;;
    en-US:invalid_choice) echo "Invalid choice." ;;
    pt-BR:target_folder) echo "Pasta raiz do workspace" ;;
    en-US:target_folder) echo "Workspace root folder" ;;
    pt-BR:target_prompt) echo "Caminho (Enter aceita o padrão)" ;;
    en-US:target_prompt) echo "Path (Enter accepts the default)" ;;
    pt-BR:checking_tools) echo "Verificando ferramentas…" ;;
    en-US:checking_tools) echo "Checking tools…" ;;
    pt-BR:tool_ok) echo "OK" ;;
    en-US:tool_ok) echo "OK" ;;
    pt-BR:tool_missing) echo "Faltando" ;;
    en-US:tool_missing) echo "Missing" ;;
    pt-BR:tool_how) echo "Como proceder?" ;;
    en-US:tool_how) echo "How do you want to proceed?" ;;
    pt-BR:tool_opt_auto) echo "Instalar para mim" ;;
    en-US:tool_opt_auto) echo "Install for me" ;;
    pt-BR:tool_opt_manual) echo "Já instalei — verificar de novo" ;;
    en-US:tool_opt_manual) echo "I installed it — check again" ;;
    pt-BR:tool_opt_quit) echo "Sair do setup" ;;
    en-US:tool_opt_quit) echo "Quit setup" ;;
    pt-BR:tool_install_failed) echo "Instalação falhou ou a ferramenta ainda não está no PATH" ;;
    en-US:tool_install_failed) echo "Install failed or tool still missing from PATH" ;;
    pt-BR:tool_hint_node) echo "Node.js LTS ≥ 20 + npm — https://nodejs.org  (ou: brew install node@22)" ;;
    en-US:tool_hint_node) echo "Node.js LTS ≥ 20 + npm — https://nodejs.org  (or: brew install node@22)" ;;
    pt-BR:tool_hint_git) echo "Git é obrigatório — https://git-scm.com  (ou: brew install git)" ;;
    en-US:tool_hint_git) echo "Git is required — https://git-scm.com  (or: brew install git)" ;;
    pt-BR:tool_hint_gh) echo "GitHub CLI — https://cli.github.com  (ou: brew install gh) e depois: gh auth login" ;;
    en-US:tool_hint_gh) echo "GitHub CLI — https://cli.github.com  (or: brew install gh) then: gh auth login" ;;
    pt-BR:tool_hint_kubectl) echo "kubectl — https://kubernetes.io/docs/tasks/tools/  (ou: brew install kubectl)" ;;
    en-US:tool_hint_kubectl) echo "kubectl — https://kubernetes.io/docs/tasks/tools/  (or: brew install kubectl)" ;;
    pt-BR:tool_hint_container) echo "Apple container — macOS 26+: brew install container  (fallback: Docker Desktop)" ;;
    en-US:tool_hint_container) echo "Apple container — macOS 26+: brew install container  (fallback: Docker Desktop)" ;;
    pt-BR:tool_hint_docker) echo "Docker Desktop (ou Docker Engine + Compose v2) — https://www.docker.com/products/docker-desktop/" ;;
    en-US:tool_hint_docker) echo "Docker Desktop (or Docker Engine + Compose v2) — https://www.docker.com/products/docker-desktop/" ;;
    pt-BR:tool_hint_mprocs) echo "mprocs (opcional): brew install mprocs  — ou use o runner simples embutido" ;;
    en-US:tool_hint_mprocs) echo "mprocs (optional): brew install mprocs  — or use the built-in simple runner" ;;
    pt-BR:need_runtime) echo "Nenhum runtime de contêiner (container ou docker). Instale um dos dois." ;;
    en-US:need_runtime) echo "No container runtime (container or docker). Install one of them." ;;
    pt-BR:runtime_selected) echo "Runtime de contêiner" ;;
    en-US:runtime_selected) echo "Container runtime" ;;
    pt-BR:cloning) echo "Clonando" ;;
    en-US:cloning) echo "Cloning" ;;
    pt-BR:keeping_repo) echo "Mantendo repositório existente" ;;
    en-US:keeping_repo) echo "Keeping existing repository" ;;
    pt-BR:clone_fail) echo "Falha ao clonar" ;;
    en-US:clone_fail) echo "Failed to clone" ;;
    pt-BR:env_install) echo "Instalando arquivo de ambiente" ;;
    en-US:env_install) echo "Installing environment file" ;;
    pt-BR:env_keep) echo "Mantendo .env existente" ;;
    en-US:env_keep) echo "Keeping existing .env" ;;
    pt-BR:env_merge) echo "Adicionando chaves novas ao .env" ;;
    en-US:env_merge) echo "Adding new keys to .env" ;;
    pt-BR:env_fallback) echo "Usando template de env de fallback (adicione .env.example no repo)" ;;
    en-US:env_fallback) echo "Using fallback env template (add .env.example to the repo)" ;;
    pt-BR:env_skip) echo "Sem .env.example; pulando env" ;;
    en-US:env_skip) echo "No .env.example; skipping env" ;;
    pt-BR:ports_check) echo "Verificando portas (infra + microserviços)…" ;;
    en-US:ports_check) echo "Checking ports (infra + microservices)…" ;;
    pt-BR:ports_remap) echo "Remapeando porta ocupada" ;;
    en-US:ports_remap) echo "Remapping busy port" ;;
    pt-BR:app_deps) echo "Instalando dependências npm" ;;
    en-US:app_deps) echo "Installing npm dependencies" ;;
    pt-BR:writing_workspace) echo "Escrevendo run / publish / pull e .ipalpha/…" ;;
    en-US:writing_workspace) echo "Writing run / publish / pull and .ipalpha/…" ;;
    pt-BR:writing_settings) echo "Gravando .ipalpha/settings" ;;
    en-US:writing_settings) echo "Writing .ipalpha/settings" ;;
    pt-BR:done) echo "Setup concluído." ;;
    en-US:done) echo "Setup complete." ;;
    pt-BR:done_in) echo "Workspace em" ;;
    en-US:done_in) echo "Workspace at" ;;
    pt-BR:done_run_now) echo "Execute:" ;;
    en-US:done_run_now) echo "Run:" ;;
    pt-BR:need_gh) echo "gh é necessário para clonar os repos da org. Instale: https://cli.github.com e rode: gh auth login" ;;
    en-US:need_gh) echo "gh is required to clone the org repos. Install: https://cli.github.com then: gh auth login" ;;
    pt-BR:choose_runner) echo "Painel de processos: instalar mprocs ou usar o runner simples embutido?" ;;
    en-US:choose_runner) echo "Process panel: install mprocs or use the built-in simple runner?" ;;
    pt-BR:choose_ai) echo "CLI de IA para ./publish (mensagens de commit + semver):" ;;
    en-US:choose_ai) echo "AI CLI for ./publish (commit messages + semver):" ;;
    pt-BR:ai_selected) echo "IA do publish" ;;
    en-US:ai_selected) echo "Publish AI" ;;
    pt-BR:update_pulling) echo "Atualizando repositórios existentes…" ;;
    en-US:update_pulling) echo "Pulling existing repositories…" ;;
    pt-BR:update_pulled) echo "atualizado" ;;
    en-US:update_pulled) echo "pulled" ;;
    pt-BR:update_conflict) echo "conflito/pulado (sem alterações)" ;;
    en-US:update_conflict) echo "conflict/skipped (left unchanged)" ;;
    pt-BR:update_clone_fail) echo "clone falhou" ;;
    en-US:update_clone_fail) echo "clone failed" ;;
    pt-BR:update_done) echo "Pull concluído." ;;
    en-US:update_done) echo "Pull complete." ;;
    pt-BR:update_tooling_ok) echo "ferramentas atualizadas (.ipalpha)" ;;
    en-US:update_tooling_ok) echo "tooling refreshed (.ipalpha)" ;;
    pt-BR:update_tooling_fail) echo "não foi possível atualizar as ferramentas (sem acesso ao .github)" ;;
    en-US:update_tooling_fail) echo "could not refresh tooling (no access to .github)" ;;
    pt-BR:cleanup) echo "Removendo o clone do setup" ;;
    en-US:cleanup) echo "Removing the setup clone" ;;
    pt-BR:update_no_settings) echo "Falta .ipalpha/settings — rode o setup primeiro." ;;
    en-US:update_no_settings) echo "Missing .ipalpha/settings — run setup first." ;;
    pt-BR:help_run) echo "Uso: ./run

Sobe o ambiente local do IPAlpha:
  1. Infra no contêiner (MongoDB, Redis, RabbitMQ + UI de management)
  2. Instala dependências npm que faltam (shared-js vem do npm)
  3. Painel de processos com projects-api, person-api, organization-api, notification-api, auth-api (nesta ordem)

Runner (settings runner= ou IPALPHA_RUNNER=):
  auto        mprocs se instalado, senão runner simples em background
  mprocs      prefere mprocs (fallback se faltar)
  background  sem UI — logs em \$TMPDIR/ipalpha-run-logs

Portas vêm de .ipalpha/settings / .ipalpha/ports.env. Pare com .ipalpha/bin/infra-down." ;;
    en-US:help_run) echo "Usage: ./run

Start the local IPAlpha stack:
  1. Infra in containers (MongoDB, Redis, RabbitMQ + management UI)
  2. Install missing npm dependencies (shared-js comes from npm)
  3. Process panel with projects-api, person-api, organization-api, notification-api, auth-api (in that order)

Runner (settings runner= or IPALPHA_RUNNER=):
  auto        mprocs if installed, else simple background runner
  mprocs      prefer mprocs (fallback if missing)
  background  no UI — logs under \$TMPDIR/ipalpha-run-logs

Ports come from .ipalpha/settings / .ipalpha/ports.env. Stop infra with .ipalpha/bin/infra-down." ;;
    pt-BR:help_pull) echo "Uso: ./pull

Sincroniza esta máquina com a org ipalpha:
  1. Pull fast-forward de cada repo (conflito pula a pasta)
  2. Clona repos que faltam
  3. Cria .env nos novos e adiciona chaves novas aos existentes
  4. Reescreve portas a partir de .ipalpha/settings

Lê idioma e portas de .ipalpha/settings." ;;
    en-US:help_pull) echo "Usage: ./pull

Sync this machine with the ipalpha org:
  1. Fast-forward pull of every repo (conflicts skip that folder)
  2. Clone missing repos
  3. Create .env for new repos and add new keys to existing ones
  4. Rewrite ports from .ipalpha/settings

Reads language and ports from .ipalpha/settings." ;;
    pt-BR:help_publish) echo "Uso: ./publish [flags]

Release de dev:
  • Detecta repos sujos em core/ (multi-seleção se vários; todos marcados por padrão)
  • IA escolhe bump semver + mensagem de commit
  • Commit/push de cada repo escolhido
  • Build/push da imagem ghcr.io/ipalpha-dev/<ms>
  • Atualiza a tag da imagem em deployment/core/<ms>/ e push

Flags:
  -d, --dry-run     preview e opção de aplicar exatamente esse plano
  -f, --folder NOME só um repo
  --engine NOME     CLI de IA só nesta execução
  clean             limpa o cache de decisões
  -h, --help        esta ajuda" ;;
    en-US:help_publish) echo "Usage: ./publish [flags]

Dev release:
  • Detect dirty repos under core/ (multi-select when several; all selected by default)
  • AI picks semver bump + commit message
  • Commit/push each selected repo
  • Build/push image ghcr.io/ipalpha-dev/<ms>
  • Bump image tag in deployment/core/<ms>/ and push it

Flags:
  -d, --dry-run     preview, then optionally apply that exact plan
  -f, --folder NAME only one repo
  --engine NAME     AI CLI for this run only
  clean             wipe the decision cache
  -h, --help        this help" ;;
    pt-BR:publish_no_dirty) echo "Nenhum repo sujo em core/ — nada a publicar." ;;
    en-US:publish_no_dirty) echo "No dirty repos under core/ — nothing to publish." ;;
    pt-BR:publish_select) echo "Repos sujos — toggle pelo número, Enter confirma" ;;
    en-US:publish_select) echo "Dirty repos — toggle by number, Enter confirms" ;;
    pt-BR:publish_plan) echo "Plano de publicação" ;;
    en-US:publish_plan) echo "Publish plan" ;;
    pt-BR:publish_apply) echo "Aplicar este plano? [s/N]" ;;
    en-US:publish_apply) echo "Apply this plan? [y/N]" ;;
    pt-BR:publish_ai_down) echo "IA indisponível — usando padrão (patch)" ;;
    en-US:publish_ai_down) echo "AI unavailable — defaulting to patch" ;;
    pt-BR:publish_done) echo "Publish concluído." ;;
    en-US:publish_done) echo "Publish complete." ;;
    pt-BR:publish_cache_clean) echo "Cache de decisões limpo." ;;
    en-US:publish_cache_clean) echo "Decision cache wiped." ;;
    pt-BR:publish_no_dockerfile) echo "sem Dockerfile — pulando imagem" ;;
    en-US:publish_no_dockerfile) echo "no Dockerfile — skipping image" ;;
    pt-BR:publish_deployment_missing) echo "pasta deployment ausente — pulando bump de tag" ;;
    en-US:publish_deployment_missing) echo "deployment folder missing — skipping tag bump" ;;
    pt-BR:publish_aborted) echo "Abortado." ;;
    en-US:publish_aborted) echo "Aborted." ;;
    pt-BR:publish_folder_unknown) echo "Repo desconhecido:" ;;
    en-US:publish_folder_unknown) echo "Unknown repo:" ;;
    *) echo "$key" ;;
  esac
}

ipalpha_prompt_language() {
  local picked
  printf '\033[?25h' >/dev/tty 2>/dev/null || true
  echo "$(ipalpha_msg choose_lang)"
  read -r picked </dev/tty 2>/dev/null || picked=""
  case "$picked" in
    2|en|en-US) ipalpha_i18n_init en-US ;;
    *) ipalpha_i18n_init pt-BR ;;
  esac
  printf '\033[?25h' >/dev/tty 2>/dev/null || true
}
