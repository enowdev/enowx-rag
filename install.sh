#!/bin/sh
# enowx-rag installer — downloads a prebuilt binary from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/enowdev/enowx-rag/main/install.sh | sh
#
# Options (env vars):
#   ENOWX_VERSION   version to install, e.g. v0.1.0 (default: latest)
#   ENOWX_INSTALL_DIR   install directory (default: /usr/local/bin, or
#                       ~/.local/bin if that isn't writable)
set -eu

REPO="enowdev/enowx-rag"
BINARY="enowx-rag"

info() { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
err()  { printf '\033[1;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || \
  err "need curl or wget installed"
command -v tar >/dev/null 2>&1 || err "need tar installed"

# Fetch a URL to stdout with whichever downloader is present.
fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  else
    wget -qO- "$1"
  fi
}
# Download a URL to a file.
download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  else
    wget -qO "$2" "$1"
  fi
}

# --- Detect OS/arch (must match .goreleaser.yaml name_template) ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS: $os (Windows: download the .zip from the Releases page)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- Resolve version ---
version="${ENOWX_VERSION:-}"
if [ -z "$version" ]; then
  info "Resolving latest release…"
  version=$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name":' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$version" ] || err "could not determine latest version; set ENOWX_VERSION"
fi
# Strip a leading v for the archive name (which uses the bare version).
ver_no_v=${version#v}

archive="${BINARY}_${ver_no_v}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

info "Downloading ${archive} (${version})…"
download "${base_url}/${archive}" "${tmp}/${archive}" \
  || err "download failed — check that ${version} has a ${os}/${arch} build"

# --- Verify checksum when available ---
if download "${base_url}/checksums.txt" "${tmp}/checksums.txt" 2>/dev/null; then
  expected=$(grep " ${archive}\$" "${tmp}/checksums.txt" | awk '{print $1}')
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual=$(sha256sum "${tmp}/${archive}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
      actual=$(shasum -a 256 "${tmp}/${archive}" | awk '{print $1}')
    fi
    if [ -n "${actual:-}" ] && [ "$actual" != "$expected" ]; then
      err "checksum mismatch for ${archive}"
    fi
    info "Checksum verified."
  fi
fi

tar -xzf "${tmp}/${archive}" -C "$tmp"
[ -f "${tmp}/${BINARY}" ] || err "archive did not contain ${BINARY}"
chmod +x "${tmp}/${BINARY}"

# --- Choose install dir ---
dir="${ENOWX_INSTALL_DIR:-/usr/local/bin}"
if [ ! -d "$dir" ] || [ ! -w "$dir" ]; then
  if [ "$dir" = "/usr/local/bin" ]; then
    dir="${HOME}/.local/bin"
    mkdir -p "$dir"
    info "/usr/local/bin not writable; installing to ${dir}"
  else
    err "install dir not writable: $dir"
  fi
fi

mv "${tmp}/${BINARY}" "${dir}/${BINARY}"
info "Installed ${BINARY} ${version} to ${dir}/${BINARY}"

case ":${PATH}:" in
  *":${dir}:"*) ;;
  *) printf '\033[1;33mnote:\033[0m %s is not on your PATH. Add it:\n  export PATH="%s:$PATH"\n' "$dir" "$dir" ;;
esac

info "Run '${BINARY} version' to verify, or '${BINARY} --serve' to start the dashboard."
