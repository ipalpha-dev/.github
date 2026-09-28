#!/usr/bin/env bash
set -euo pipefail

org="ipalpha-dev"
repo=".github"

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
fail() { printf '\n\033[31m%s\033[0m\n' "$*" >&2; exit 1; }

say "IPAlpha — preparando o ambiente de desenvolvimento"

if ! command -v git >/dev/null 2>&1; then
  if [[ "$(uname -s)" == "Darwin" ]]; then
    xcode-select --install >/dev/null 2>&1 || true
    fail "Instale as Xcode Command Line Tools (janela aberta) e rode este comando de novo."
  fi
  fail "Instale o git e rode este comando de novo."
fi

ssh_out="$(ssh -T -o StrictHostKeyChecking=accept-new -o BatchMode=yes git@github.com 2>&1 || true)"
if ! grep -q "successfully authenticated" <<<"$ssh_out"; then
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    clone() { gh repo clone "$org/$repo" "$1" -- --quiet; }
  else
    fail "Sem acesso SSH ao GitHub. Configure uma chave: https://docs.github.com/pt/authentication/connecting-to-github-with-ssh
Depois confirme com: ssh -T git@github.com"
  fi
else
  clone() { git clone --quiet "git@github.com:$org/$repo.git" "$1"; }
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-setup.XXXXXX")"
say "Baixando as ferramentas ($org/$repo)…"
clone "$tmp/$repo" || fail "Não foi possível clonar $org/$repo. Você é membro da organização $org?"

export IPALPHA_DEFAULT_ROOT="${IPALPHA_DEFAULT_ROOT:-$PWD/IpAlpha}"
exec "$tmp/$repo/setup" "$@" </dev/tty
