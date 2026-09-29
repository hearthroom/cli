#!/bin/sh
# Install the hearthroom CLI on Linux or macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/hearthroom/cli/main/install.sh | sh
#
# Environment:
#   HEARTHROOM_VERSION      release tag to install (default: latest), e.g. v0.1.0
#   HEARTHROOM_INSTALL_DIR  target directory (default: ~/.local/bin, or /usr/local/bin when run as root)
#
# The script downloads the release archive for this platform from GitHub
# Releases, verifies it against checksums.txt from the same release, and places
# the `hearthroom` binary in the install directory. Files fetched with curl are
# not quarantined by macOS Gatekeeper, so no separate approval step is needed.
set -eu

REPO="hearthroom/cli"
BIN="hearthroom"

say() { printf '%s\n' "$*" >&2; }
die() { say "install: $*"; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"; }
need curl
need tar

os=$(uname -s)
case "$os" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported operating system: $os (use install.ps1 on Windows)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

version="${HEARTHROOM_VERSION:-}"
if [ -z "$version" ]; then
  # Resolve "latest" through the redirect on the releases page; this avoids
  # the rate-limited API.
  version=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's#.*/tag/##')
  case "$version" in v*) ;; *) die "could not determine the latest release; set HEARTHROOM_VERSION" ;; esac
fi
plain=${version#v}

asset="${BIN}_${plain}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "Downloading ${BIN} ${version} for ${os}/${arch}..."
curl -fsSL -o "$tmp/$asset" "$base/$asset" || die "download failed: $base/$asset"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "download failed: $base/checksums.txt"

expected=$(grep "  $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$expected" ] || die "no checksum for $asset in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$asset" | cut -d' ' -f1)
else
  actual=$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)
fi
[ "$expected" = "$actual" ] || die "checksum mismatch for $asset"

tar -xzf "$tmp/$asset" -C "$tmp" "$BIN"

if [ -n "${HEARTHROOM_INSTALL_DIR:-}" ]; then
  dir="$HEARTHROOM_INSTALL_DIR"
elif [ "$(id -u)" = "0" ]; then
  dir="/usr/local/bin"
else
  dir="$HOME/.local/bin"
fi
mkdir -p "$dir"
install -m 0755 "$tmp/$BIN" "$dir/$BIN"

say "Installed $dir/$BIN"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Add $dir to your PATH, for example:  export PATH=\"$dir:\$PATH\"" ;;
esac
"$dir/$BIN" version >&2 || true
