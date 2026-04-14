#!/usr/bin/env sh
# Supermodel CLI installer
# Usage: curl -fsSL https://supermodeltools.com/install.sh | sh

set -e

REPO="supermodeltools/cli"
BINARY="supermodel"
INSTALL_DIR="${SUPERMODEL_INSTALL_DIR:-/usr/local/bin}"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Resolve latest version if not pinned
if [ -z "$SUPERMODEL_VERSION" ]; then
  SUPERMODEL_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
fi

if [ -z "$SUPERMODEL_VERSION" ]; then
  echo "Could not determine latest version. Set SUPERMODEL_VERSION to pin a release."
  exit 1
fi

VERSION_BARE="${SUPERMODEL_VERSION#v}"
ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${SUPERMODEL_VERSION}/${ARCHIVE}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${SUPERMODEL_VERSION}/checksums.txt"

echo "Installing supermodel ${SUPERMODEL_VERSION} (${OS}/${ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "$TMP/$ARCHIVE"
curl -fsSL "$CHECKSUM_URL" -o "$TMP/checksums.txt"

# Verify checksum
(cd "$TMP" && grep "$ARCHIVE" checksums.txt | sha256sum --check --status 2>/dev/null \
  || shasum -a 256 --check --ignore-missing checksums.txt 2>/dev/null) \
  || { echo "Checksum verification failed"; exit 1; }

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" "$BINARY"

# Install — fall back to user-local dir if /usr/local/bin isn't writable
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  echo "No write access to /usr/local/bin — installing to $INSTALL_DIR"
  echo "Add $INSTALL_DIR to your PATH if it isn't already."
fi

install -m755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"

echo "Installed: $INSTALL_DIR/$BINARY"
"$INSTALL_DIR/$BINARY" version

# Run the setup wizard when a controlling terminal is available.
# Use /dev/tty as stdin so interactive prompts work even in piped installs
# (e.g. curl … | sh), where stdin is the pipe rather than the terminal.
if { </dev/tty; } 2>/dev/null; then
  echo ""
  "$INSTALL_DIR/$BINARY" setup </dev/tty
fi
