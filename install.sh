#!/bin/sh
# Install the latest suluctl release. Usage:
#   curl -fsSL https://raw.githubusercontent.com/ellyZz/suluctl/main/install.sh | sh
set -eu

REPO="ellyZz/suluctl"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported OS: $OS (download manually from https://github.com/$REPO/releases)" >&2; exit 1 ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | head -1 | cut -d'"' -f4)
[ -n "$TAG" ] || { echo "could not resolve latest release" >&2; exit 1; }
VERSION=${TAG#v}

URL="https://github.com/$REPO/releases/download/$TAG/suluctl_${VERSION}_${OS}_${ARCH}.tar.gz"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading suluctl $TAG ($OS/$ARCH)..."
curl -fsSL "$URL" | tar -xz -C "$TMP"

DEST="/usr/local/bin"
if [ ! -w "$DEST" ]; then
  DEST="$HOME/.local/bin"
  mkdir -p "$DEST"
fi
install -m 0755 "$TMP/suluctl" "$DEST/suluctl"
echo "Installed: $DEST/suluctl"
"$DEST/suluctl" version
