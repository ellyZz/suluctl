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

TAG=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's#.*/tag/##')
[ -n "$TAG" ] || { echo "could not resolve latest release" >&2; exit 1; }
VERSION=${TAG#v}

ARCHIVE="suluctl_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$ARCHIVE"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading suluctl $TAG ($OS/$ARCH)..."
curl -fsSL -o "$TMP/$ARCHIVE" "$URL"
curl -fsSL -o "$TMP/checksums.txt" "https://github.com/$REPO/releases/download/$TAG/checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | sha256sum -c - >/dev/null)
elif command -v shasum >/dev/null 2>&1; then
  (cd "$TMP" && grep " $ARCHIVE\$" checksums.txt | shasum -a 256 -c - >/dev/null)
else
  echo "warning: no sha256 tool found, skipping checksum verification" >&2
fi
tar -xz -C "$TMP" -f "$TMP/$ARCHIVE"

DEST="/usr/local/bin"
if [ ! -w "$DEST" ]; then
  DEST="$HOME/.local/bin"
  mkdir -p "$DEST"
fi
install -m 0755 "$TMP/suluctl" "$DEST/suluctl"
echo "Installed: $DEST/suluctl"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "warning: $DEST is not on your PATH" >&2 ;;
esac
"$DEST/suluctl" version
