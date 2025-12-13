#!/usr/bin/env bash
set -euo pipefail

# install.sh
# Downloads a mcp-lens release asset for the current OS/arch, verifies it against checksums.txt,
# installs the binary into ~/.local/bin, and prints the installed absolute path.

REPO="golovatskygroup/mcp-lens"
VERSION="${MCP_LENS_VERSION:-v1.0.2}"
INSTALL_DIR="${MCP_LENS_INSTALL_DIR:-$HOME/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64) arch="amd64";;
  aarch64|arm64) arch="arm64";;
  *) echo "Unsupported arch: $arch" >&2; exit 1;;
esac

asset="mcp-lens_${VERSION#v}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${VERSION}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -fsSL -o "$work/checksums.txt" "${base_url}/checksums.txt"
curl -fsSL -o "$work/$asset" "${base_url}/$asset"

expected="$(grep " ${asset}\$" "$work/checksums.txt" | awk '{print $1}')"
if [[ -z "$expected" ]]; then
  echo "Checksum entry not found for $asset" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$work/$asset" | awk '{print $1}')"
else
  got="$(shasum -a 256 "$work/$asset" | awk '{print $1}')"
fi

if [[ "$expected" != "$got" ]]; then
  echo "Checksum mismatch for $asset" >&2
  echo "expected=$expected" >&2
  echo "got=$got" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
tar -xzf "$work/$asset" -C "$work" mcp-lens
mv -f "$work/mcp-lens" "$INSTALL_DIR/mcp-lens"
chmod +x "$INSTALL_DIR/mcp-lens"

python3 - <<PY
import os
print(os.path.realpath(os.path.expanduser("$INSTALL_DIR/mcp-lens")))
PY
