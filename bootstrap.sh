#!/usr/bin/env bash
# Kept so old links keep working: the setup now lives in install.sh (macOS/Linux/WSL) and
# install.ps1 (Windows).
set -euo pipefail
url="https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.sh"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$url" | sh -s -- "$@"
else
  wget -qO- "$url" | sh -s -- "$@"
fi
