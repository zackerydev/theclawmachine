#!/usr/bin/env sh
set -eu

REPO="zackerydev/theclawmachine"
BINARY="clawmachine"
VERSION="${VERSION:-}"

if [ -z "${INSTALL_DIR:-}" ]; then
  if [ -n "${HOME:-}" ]; then
    INSTALL_DIR="${HOME}/.local/bin"
  else
    INSTALL_DIR="/usr/local/bin"
  fi
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd tar
require_cmd uname

if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi

if [ -z "$VERSION" ]; then
  echo "error: failed to determine latest release tag" >&2
  exit 1
fi

RAW_VERSION="${VERSION#v}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *)
    echo "error: unsupported operating system: $OS" >&2
    exit 1
    ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

ASSET="${BINARY}_${RAW_VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."
curl -fsSL "$URL" -o "$TMP_DIR/$ASSET"
tar -xzf "$TMP_DIR/$ASSET" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/$BINARY" ]; then
  echo "error: downloaded archive does not contain ${BINARY}" >&2
  exit 1
fi

chmod +x "$TMP_DIR/$BINARY"

if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
  echo "error: could not create install directory: ${INSTALL_DIR}" >&2
  echo "hint: set INSTALL_DIR to a writable location, e.g. INSTALL_DIR=\$HOME/.local/bin" >&2
  exit 1
fi

if [ ! -w "$INSTALL_DIR" ]; then
  echo "error: install directory is not writable: ${INSTALL_DIR}" >&2
  echo "hint: use a user-writable path, e.g. INSTALL_DIR=\$HOME/.local/bin" >&2
  exit 1
fi

cp "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
"$INSTALL_DIR/$BINARY" version || true

case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) in_path="yes" ;;
  *) in_path="no" ;;
esac

if [ "$in_path" = "no" ]; then
  path_export="export PATH=\"$INSTALL_DIR:\$PATH\""
  shell_name="$(basename "${SHELL:-}")"

  echo
  echo "Your PATH does not include ${INSTALL_DIR}."
  echo "Add this line to your shell config:"
  case "$shell_name" in
    zsh)
      echo "  ${path_export}   # ~/.zshrc"
      ;;
    bash)
      echo "  ${path_export}   # ~/.bashrc (or ~/.bash_profile on macOS)"
      ;;
    *)
      echo "  ${path_export}"
      ;;
  esac
  echo
  echo "To use it in this terminal now, run:"
  echo "  ${path_export}"
fi
