#!/usr/bin/env sh
set -eu

repo="boan-anbo/codex-switch"
alias_name=false
install_skill=false
mode=auto
version=latest
bin_dir="${BIN_DIR:-$HOME/.local/bin}"
script_dir=""

case "$0" in
  */*)
    script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true)"
    ;;
esac

usage() {
  cat <<'EOF' >&2
usage: install.sh [--alias] [--skill] [--from-source|--from-release] [--version VERSION] [--bin-dir DIR]
EOF
}

set_mode() {
  next_mode="$1"
  if [ "$mode" != auto ] && [ "$mode" != "$next_mode" ]; then
    echo "choose only one of --from-source or --from-release" >&2
    exit 2
  fi
  mode="$next_mode"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --alias) alias_name=true ;;
    --skill) install_skill=true ;;
    --from-source) set_mode source ;;
    --from-release) set_mode release ;;
    --version=*) version="${1#--version=}" ;;
    --version)
      shift
      if [ "$#" -eq 0 ]; then
        echo "missing value for --version" >&2
        exit 2
      fi
      version="$1"
      ;;
    --bin-dir=*) bin_dir="${1#--bin-dir=}" ;;
    --bin-dir)
      shift
      if [ "$#" -eq 0 ]; then
        echo "missing value for --bin-dir" >&2
        exit 2
      fi
      bin_dir="$1"
      ;;
    --help | -h)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
  shift
done

if [ "$bin_dir" = "" ]; then
  bin_dir="$HOME/.local/bin"
fi
case "$bin_dir" in
  "~") bin_dir="$HOME" ;;
  "~/"*) bin_dir="$HOME/${bin_dir#~/}" ;;
esac
case "$bin_dir" in
  /*) ;;
  *) bin_dir="$(pwd)/$bin_dir" ;;
esac

install_alias() {
  target="$1"
  alias_path="$(dirname "$target")/cs"
  if command -v cs >/dev/null 2>&1 || [ -e "$alias_path" ]; then
    echo "cs already exists; leaving it untouched" >&2
  else
    ln -s "$target" "$alias_path"
    echo "installed cs -> $target"
  fi
}

install_bundled_skill() {
  target="$1"
  "$target" skill install
}

install_binary() {
  src="$1"
  dst="$2"
  dst_dir="$(dirname "$dst")"
  tmp_dst="$(mktemp "$dst_dir/.codex-switch-install.XXXXXX")"
  if cp "$src" "$tmp_dst" && chmod 755 "$tmp_dst" && mv "$tmp_dst" "$dst"; then
    return 0
  fi
  rm -f "$tmp_dst"
  return 1
}

checkout_dir() {
  if [ -f go.mod ] && grep -q "^module github.com/$repo\$" go.mod; then
    pwd
    return 0
  fi
  if [ "$script_dir" != "" ] && [ -f "$script_dir/../go.mod" ] && grep -q "^module github.com/$repo\$" "$script_dir/../go.mod"; then
    (cd "$script_dir/.." && pwd)
    return 0
  fi
  return 1
}

is_checkout() {
  checkout_dir >/dev/null 2>&1
}

install_from_source() {
  if ! command -v go >/dev/null 2>&1; then
    echo "go is required for --from-source" >&2
    exit 1
  fi
  mkdir -p "$bin_dir"
  source_dir="$(checkout_dir || true)"
  if [ "$source_dir" != "" ]; then
    (cd "$source_dir" && GOBIN="$bin_dir" go install .)
  else
    GOBIN="$bin_dir" go install "github.com/$repo@$version"
  fi
  target="$bin_dir/codex-switch"
  if [ "$alias_name" = true ]; then
    install_alias "$target"
  fi
  if [ "$install_skill" = true ]; then
    install_bundled_skill "$target"
  fi
  echo "installed codex-switch -> $target"
}

platform_asset() {
  os_name="$(uname -s)"
  arch_name="$(uname -m)"
  case "$os_name" in
    Darwin) os_name=darwin ;;
    Linux) os_name=linux ;;
    *)
      echo "unsupported OS: $os_name" >&2
      exit 1
      ;;
  esac
  case "$arch_name" in
    x86_64 | amd64) arch_name=amd64 ;;
    arm64 | aarch64) arch_name=arm64 ;;
    *)
      echo "unsupported architecture: $arch_name" >&2
      exit 1
      ;;
  esac
  printf 'codex-switch_%s_%s.tar.gz\n' "$os_name" "$arch_name"
}

download() {
  url="$1"
  dst="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dst"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$dst"
  else
    echo "curl or wget is required to download releases" >&2
    exit 1
  fi
}

release_url() {
  asset="$1"
  if [ "$version" = latest ]; then
    printf 'https://github.com/%s/releases/latest/download/%s\n' "$repo" "$asset"
  else
    printf 'https://github.com/%s/releases/download/%s/%s\n' "$repo" "$version" "$asset"
  fi
}

verify_checksum() {
  asset="$1"
  archive="$2"
  checksums="$3"
  expected="$(awk -v asset="$asset" '$2 == asset {print $1}' "$checksums")"
  if [ "$expected" = "" ]; then
    echo "checksum entry missing for $asset" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
  else
    echo "sha256sum or shasum is required to verify release checksums" >&2
    exit 1
  fi
  if [ "$actual" != "$expected" ]; then
    echo "checksum mismatch for $asset" >&2
    echo "expected: $expected" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi
}

install_from_release() {
  asset="$(platform_asset)"
  tmp_dir="$(mktemp -d)"
  cleanup() {
    chmod -R u+w "$tmp_dir" 2>/dev/null || true
    rm -rf "$tmp_dir"
  }
  trap cleanup EXIT INT TERM
  archive="$tmp_dir/$asset"
  checksums="$tmp_dir/checksums.txt"
  if [ "${CODEX_SWITCH_RELEASE_DIR:-}" != "" ]; then
    cp "${CODEX_SWITCH_RELEASE_DIR%/}/$asset" "$archive"
    cp "${CODEX_SWITCH_RELEASE_DIR%/}/checksums.txt" "$checksums"
  else
    download "$(release_url "$asset")" "$archive"
    download "$(release_url checksums.txt)" "$checksums"
  fi
  verify_checksum "$asset" "$archive" "$checksums"
  tar -xzf "$archive" -C "$tmp_dir"
  mkdir -p "$bin_dir"
  install_binary "$tmp_dir/codex-switch" "$bin_dir/codex-switch"
  if [ "$alias_name" = true ]; then
    install_alias "$bin_dir/codex-switch"
  fi
  if [ "$install_skill" = true ]; then
    install_bundled_skill "$bin_dir/codex-switch"
  fi
  echo "installed codex-switch -> $bin_dir/codex-switch"
}

if [ "$mode" = auto ]; then
  if is_checkout; then
    mode=source
  else
    mode=release
  fi
fi

case "$mode" in
  source) install_from_source ;;
  release) install_from_release ;;
  *)
    echo "unknown install mode: $mode" >&2
    exit 2
    ;;
esac
