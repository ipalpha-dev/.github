#!/bin/sh
# IPAlpha setup for macOS, Linux and WSL. Downloads the ipalpha tool for this computer and runs
# its setup (tools, repositories, .env files, AI). Safe to run again.
#   curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.sh | sh
set -eu

base="${IPALPHA_RELEASE_URL:-https://github.com/ipalpha-dev/.github/releases/latest/download}"

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
fail() { printf '\n\033[31m%s\033[0m\n' "$*" >&2; exit 1; }

os="$(uname -s)"
case "$os" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  MINGW*|MSYS*|CYGWIN*) fail "No Windows, use o PowerShell: irm https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.ps1 | iex
On Windows use PowerShell: irm https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.ps1 | iex" ;;
  *) fail "Sistema não suportado / unsupported system: $os" ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "Processador não suportado / unsupported CPU: $arch" ;;
esac
asset="ipalpha-$os-$arch"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-install.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "IPAlpha — baixando a ferramenta / downloading the tool ($asset)"
download() {
  if command -v curl >/dev/null 2>&1; then curl -fsSL --retry 3 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then wget -q -O "$2" "$1"
  else fail "Instale curl ou wget / install curl or wget"; fi
}
download "$base/$asset" "$tmp/ipalpha" || fail "Falha no download / download failed: $base/$asset
Confira a internet e tente de novo / check the connection and try again."
download "$base/checksums.txt" "$tmp/checksums.txt" || fail "Falha no download / download failed: checksums.txt"

want="$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
if command -v sha256sum >/dev/null 2>&1; then got="$(sha256sum "$tmp/ipalpha" | awk '{print $1}')"
else got="$(shasum -a 256 "$tmp/ipalpha" | awk '{print $1}')"; fi
[ -n "$want" ] && [ "$want" = "$got" ] || fail "Arquivo corrompido (checksum) / corrupted download (checksum)"
chmod +x "$tmp/ipalpha"

# The setup asks questions: give it the terminal even when this script came through a pipe.
if [ -t 1 ] && [ -r /dev/tty ]; then
  "$tmp/ipalpha" setup "$@" </dev/tty
else
  "$tmp/ipalpha" setup "$@"
fi
