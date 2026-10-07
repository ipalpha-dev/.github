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
    pt-BR:setup_keys) echo "↑/↓ escolher   Enter confirmar   Esc sair" ;;
    en-US:setup_keys) echo "↑/↓ choose   Enter confirm   Esc quit" ;;
    pt-BR:setup_input_keys) echo "Enter confirmar   Ctrl+C sair" ;;
    en-US:setup_input_keys) echo "Enter confirm   Ctrl+C quit" ;;
    pt-BR:setup_configuring) echo "Configurando o workspace" ;;
    en-US:setup_configuring) echo "Configuring the workspace" ;;
    pt-BR:invalid_choice) echo "Opção inválida." ;;
    en-US:invalid_choice) echo "Invalid choice." ;;
    pt-BR:target_folder) echo "Pasta raiz do workspace" ;;
    en-US:target_folder) echo "Workspace root folder" ;;
    pt-BR:target_prompt) echo "Caminho (Enter aceita o padrão)" ;;
    en-US:target_prompt) echo "Path (Enter accepts the default)" ;;
    pt-BR:seed_account) echo "Primeiro acesso" ;;
    en-US:seed_account) echo "First sign-in" ;;
    es:seed_account) echo "Primer acceso" ;;
    fr:seed_account) echo "Premier accès" ;;
    de:seed_account) echo "Erste Anmeldung" ;;
    pt-BR:seed_name) echo "Seu nome" ;;
    en-US:seed_name) echo "Your name" ;;
    es:seed_name) echo "Tu nombre" ;;
    fr:seed_name) echo "Votre nom" ;;
    de:seed_name) echo "Ihr Name" ;;
    pt-BR:seed_name_hint) echo "Edite o nome ou pressione Enter para manter." ;;
    en-US:seed_name_hint) echo "Edit the name or press Enter to keep it." ;;
    es:seed_name_hint) echo "Edita el nombre o pulsa Enter para conservarlo." ;;
    fr:seed_name_hint) echo "Modifiez le nom ou appuyez sur Entrée pour le conserver." ;;
    de:seed_name_hint) echo "Namen bearbeiten oder mit Enter beibehalten." ;;
    pt-BR:seed_phone) echo "Seu celular com DDD" ;;
    en-US:seed_phone) echo "Your mobile number with area code" ;;
    es:seed_phone) echo "Tu móvil con código de área" ;;
    fr:seed_phone) echo "Votre portable avec l’indicatif régional" ;;
    de:seed_phone) echo "Ihre Mobilnummer mit Vorwahl" ;;
    pt-BR:seed_phone_hint) echo "DDD + número: 11 dígitos. Não precisa incluir +55." ;;
    en-US:seed_phone_hint) echo "Area code + number: 11 digits. No need to include +55." ;;
    es:seed_phone_hint) echo "Código de área + número: 11 dígitos. No hace falta +55." ;;
    fr:seed_phone_hint) echo "Indicatif régional + numéro : 11 chiffres. +55 est facultatif." ;;
    de:seed_phone_hint) echo "Vorwahl + Nummer: 11 Ziffern. +55 ist nicht erforderlich." ;;
    pt-BR:seed_name_invalid) echo "Informe um nome de até 200 caracteres, em uma linha." ;;
    en-US:seed_name_invalid) echo "Enter a name up to 200 characters on one line." ;;
    es:seed_name_invalid) echo "Introduce un nombre de hasta 200 caracteres en una línea." ;;
    fr:seed_name_invalid) echo "Saisissez un nom de 200 caractères maximum sur une ligne." ;;
    de:seed_name_invalid) echo "Geben Sie einen Namen mit höchstens 200 Zeichen in einer Zeile ein." ;;
    pt-BR:seed_phone_invalid) echo "Confira o DDD e o celular: devem ter 11 dígitos." ;;
    en-US:seed_phone_invalid) echo "Check the area code and mobile number: 11 digits required." ;;
    es:seed_phone_invalid) echo "Revisa el código de área y el móvil: deben tener 11 dígitos." ;;
    fr:seed_phone_invalid) echo "Vérifiez l’indicatif régional et le portable : 11 chiffres requis." ;;
    de:seed_phone_invalid) echo "Prüfen Sie Vorwahl und Mobilnummer: 11 Ziffern erforderlich." ;;
    pt-BR:seed_save_failed) echo "Não foi possível salvar a configuração do primeiro acesso." ;;
    en-US:seed_save_failed) echo "Could not save the first sign-in configuration." ;;
    es:seed_save_failed) echo "No se pudo guardar la configuración del primer acceso." ;;
    fr:seed_save_failed) echo "Impossible d’enregistrer la configuration du premier accès." ;;
    de:seed_save_failed) echo "Die Konfiguration der ersten Anmeldung konnte nicht gespeichert werden." ;;
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
    pt-BR:help_run) echo "Uso: ./run [stop [--purge]]

Sobe o ambiente local do IPAlpha:
  1. Infra no contêiner (MongoDB, Redis, RabbitMQ + UI de management) — reaproveita contêineres existentes
  2. Instala dependências npm que faltam (shared-js vem do npm)
  3. Abre o painel de processos (infra + MSs, logs ao vivo, iniciar/parar/reiniciar por MS)

Teclas do painel: ↑/↓ seleciona · Tab foco no log · s inicia/para · r reinicia · o abre /frontend · c limpa · f segue · q sai

Runner (settings runner= ou IPALPHA_RUNNER=):
  auto        padrão — painel embutido; sem terminal interativo usa background
  background  runner simples, logs em \$TMPDIR/ipalpha-run-logs
  mprocs      opt-in — usa mprocs se estiver instalado

./run stop           para a infra (mantém os contêineres para subir rápido)
./run stop --purge   remove os contêineres (os volumes/dados ficam)

Portas vêm de .ipalpha/settings / .ipalpha/ports.env." ;;
    en-US:help_run) echo "Usage: ./run [stop [--purge]]

Start the local IPAlpha stack:
  1. Infra in containers (MongoDB, Redis, RabbitMQ + management UI) — reuses existing containers
  2. Install missing npm dependencies (shared-js comes from npm)
  3. Open the process panel (infra + MSs, live logs, start/stop/restart per MS)

Panel keys: ↑/↓ select · Tab log focus · s start/stop · r restart · o open /frontend · c clear · f follow · q quit

Runner (settings runner= or IPALPHA_RUNNER=):
  auto        default — built-in panel; without an interactive terminal falls back to background
  background  plain runner, logs under \$TMPDIR/ipalpha-run-logs
  mprocs      opt-in — use mprocs if installed

./run stop           stop infra (keeps containers for a fast next start)
./run stop --purge   remove containers (volumes/data are kept)

Ports come from .ipalpha/settings / .ipalpha/ports.env." ;;
    pt-BR:run_infra) echo "Subindo a infra…" ;;
    en-US:run_infra) echo "Starting infrastructure…" ;;
    pt-BR:procs_missing) echo "Painel de processos indisponível (sem go e sem download) — ./run usará o modo background" ;;
    en-US:procs_missing) echo "Process panel unavailable (no go and download failed) — ./run will use background mode" ;;
    pt-BR:auth_clients) echo "Gerando credenciais locais dos MSs (AUTH_CLIENT_ID/SECRET + SEED_CLIENTS_JSON)" ;;
    en-US:auth_clients) echo "Generating local MS credentials (AUTH_CLIENT_ID/SECRET + SEED_CLIENTS_JSON)" ;;
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
  • Detecta repos sujos em core/ e apps/ (multi-seleção se vários; todos marcados por padrão)
  • IA escolhe bump semver + mensagem de commit (a versão vai no package.json)
  • Commit/push de cada repo escolhido
  • Tag Git v<versão>: aqui só para shared-js/shared-ui; nos demais o TeamCity cria a tag
    quando essa versão passa a rodar em produção (a imagem usa a versão como tag)

Flags:
  -d, --dry-run     preview e opção de aplicar exatamente esse plano
  -f, --folder NOME só um repo
  --engine NOME     CLI de IA só nesta execução
  clean             limpa o cache de decisões
  -h, --help        esta ajuda" ;;
    en-US:help_publish) echo "Usage: ./publish [flags]

Dev release:
  • Detect dirty repos under core/ and apps/ (multi-select when several; all selected by default)
  • AI picks semver bump + commit message (the version goes into package.json)
  • Commit/push each selected repo
  • Git tag v<version>: here only for shared-js/shared-ui; other repos are tagged by TeamCity
    once that version runs in production (the image tag is the version)

Flags:
  -d, --dry-run     preview, then optionally apply that exact plan
  -f, --folder NAME only one repo
  --engine NAME     AI CLI for this run only
  clean             wipe the decision cache
  -h, --help        this help" ;;
    pt-BR:publish_no_dirty) echo "Nada a publicar em core/ e apps/ (sem mudanças locais; versões já marcadas aguardam o deploy)." ;;
    en-US:publish_no_dirty) echo "Nothing to publish under core/ and apps/ (no local changes; bumped versions wait for the deploy)." ;;
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
    pt-BR:help_feature) echo "Uso: ./feature <comando> [slug] [flags]
  new <slug>       worktrees em feat/<slug> a partir do último Core Deploy verde (features/<slug>/)
  new <slug> --baseline <commit>  base registrada antes (commit do deployment master)
  list | ls        previews publicados + features locais, com URLs e expiração
  rebase [slug]    move a base para o último Core Deploy verde (rebase dos branches)
  extend [slug]    +72 h sem build
  reset [slug]     confirma → restaura o seed sintético (mantém imagens)
  destroy [slug]   confirma → apaga namespace/DNS/registro; mantém branches, remove worktrees
  ./publish --feature <slug>   commit + push de feat/<slug> e deploy do preview
Flags: -y/--yes (sem confirmação), --no-wait (não espera o TeamCity), --force (destroy com mudanças locais)" ;;
    en-US:help_feature) echo "Usage: ./feature <command> [slug] [flags]
  new <slug>       worktrees on feat/<slug> from the last green Core Deploy (features/<slug>/)
  new <slug> --baseline <commit>  an earlier recorded baseline (deployment master commit)
  list | ls        published previews + local features, with URLs and expiry
  rebase [slug]    move the baseline to the last green Core Deploy (rebases branches)
  extend [slug]    +72 h without a build
  reset [slug]     confirm → restore the synthetic seed (keeps images)
  destroy [slug]   confirm → delete namespace/DNS/record; keeps branches, removes worktrees
  ./publish --feature <slug>   commit + push feat/<slug> and deploy its preview
Flags: -y/--yes (no confirmation), --no-wait (do not wait for TeamCity), --force (destroy with local changes)" ;;
    pt-BR:feature_bad_slug) echo "slug inválido (use ^[a-z0-9-]{3,30}$, sem hífen nas pontas)" ;;
    en-US:feature_bad_slug) echo "invalid slug (use ^[a-z0-9-]{3,30}$, no leading/trailing hyphen)" ;;
    pt-BR:feature_no_baseline) echo "deployment master ainda não tem releases/core-latest.json (nenhum Core Deploy verde registrado)" ;;
    en-US:feature_no_baseline) echo "deployment master has no releases/core-latest.json yet (no green Core Deploy recorded)" ;;
    pt-BR:feature_list_empty) echo "nenhum preview publicado nem feature local" ;;
    en-US:feature_list_empty) echo "no published preview and no local feature" ;;
    pt-BR:feature_exists) echo "feature já existe" ;;
    en-US:feature_exists) echo "feature already exists" ;;
    pt-BR:feature_missing) echo "feature não encontrada (rode ./feature new)" ;;
    en-US:feature_missing) echo "feature not found (run ./feature new)" ;;
    pt-BR:feature_creating) echo "Criando a feature" ;;
    en-US:feature_creating) echo "Creating feature" ;;
    pt-BR:feature_created) echo "Feature criada" ;;
    en-US:feature_created) echo "Feature created" ;;
    pt-BR:feature_changed) echo "Serviços alterados" ;;
    en-US:feature_changed) echo "Changed services" ;;
    pt-BR:feature_waiting) echo "Aguardando o pipeline Preview" ;;
    en-US:feature_waiting) echo "Waiting for the Preview pipeline" ;;
    pt-BR:feature_status_unavailable) echo "sem resultado ainda — o registro já foi enviado; veja o build Preview em https://devops.kevyn.com.br (Ip Alpha / Core / Previews) — falhas aparecem só lá" ;;
    en-US:feature_status_unavailable) echo "no result yet — the record was already pushed; see the Preview build at https://devops.kevyn.com.br (Ip Alpha / Core / Previews) — failures are shown only there" ;;
    pt-BR:feature_failed) echo "Preview falhou" ;;
    en-US:feature_failed) echo "Preview failed" ;;
    pt-BR:feature_confirm) echo "Digite o slug para confirmar" ;;
    en-US:feature_confirm) echo "Type the slug to confirm" ;;
    pt-BR:feature_destroyed) echo "Feature removida (branches mantidos)" ;;
    en-US:feature_destroyed) echo "Feature removed (branches kept)" ;;
    pt-BR:feature_rebased) echo "Base atualizada" ;;
    en-US:feature_rebased) echo "Baseline refreshed" ;;
    *) echo "$key" ;;
  esac
}

ipalpha_prompt_language() {
  local picked
  if [[ "${ipalpha_ui_active:-false}" == true ]]; then
    ipalpha_ui_select "Idioma / Language" "Idioma / Language" "Português" "English" || exit 130
    if [[ "$ipalpha_ui_answer" == 2 ]]; then ipalpha_i18n_init en-US; else ipalpha_i18n_init pt-BR; fi
    return 0
  fi
  printf '\033[?25h' >/dev/tty 2>/dev/null || true
  echo "$(ipalpha_msg choose_lang)"
  read -r picked </dev/tty 2>/dev/null || picked=""
  case "$picked" in
    2|en|en-US) ipalpha_i18n_init en-US ;;
    *) ipalpha_i18n_init pt-BR ;;
  esac
  printf '\033[?25h' >/dev/tty 2>/dev/null || true
}
