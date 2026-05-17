#!/usr/bin/env sh
set -eu

repo_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
dist_dir="$repo_dir/dist"
tmp_dir="$(mktemp -d)"

case "$(uname -s)" in
  Darwin) os_name="darwin" ;;
  Linux) os_name="linux" ;;
  MINGW* | MSYS* | CYGWIN*) os_name="windows" ;;
  *)
    echo "unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch_name="amd64" ;;
  arm64 | aarch64) arch_name="arm64" ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

cleanup() {
  chmod -R u+w "$tmp_dir" 2>/dev/null || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

if [ "$os_name" = "windows" ]; then
  archive="$(find "$dist_dir" -maxdepth 1 -name "*_${os_name}_${arch_name}.zip" | head -n 1)"
else
  archive="$(find "$dist_dir" -maxdepth 1 -name "*_${os_name}_${arch_name}.tar.gz" | head -n 1)"
fi

if [ -z "$archive" ]; then
  echo "no archive found for $os_name/$arch_name in $dist_dir" >&2
  exit 1
fi

case "$archive" in
  *.zip)
    unzip -q "$archive" -d "$tmp_dir"
    ;;
  *.tar.gz)
    tar -xzf "$archive" -C "$tmp_dir"
    ;;
  *)
    echo "unsupported archive: $archive" >&2
    exit 1
    ;;
esac

test -f "$tmp_dir/README.md"
test -f "$tmp_dir/LICENSE"
test -f "$tmp_dir/CHANGELOG.md"
test -f "$tmp_dir/SECURITY.md"
test -f "$tmp_dir/skills/codex-switch/SKILL.md"
grep "codex-switch new --account NAME" "$tmp_dir/skills/codex-switch/SKILL.md" >/dev/null
grep "codex-switch list --cwd ." "$tmp_dir/skills/codex-switch/SKILL.md" >/dev/null
grep "codex-switch resume --account NAME --session SESSION_ID" "$tmp_dir/skills/codex-switch/SKILL.md" >/dev/null

binary="$tmp_dir/codex-switch"
if [ "$os_name" = "windows" ]; then
  binary="$tmp_dir/codex-switch.exe"
fi
test -f "$binary"

if [ "$os_name" != "windows" ]; then
  chmod +x "$binary"
  version_output="$("$binary" version)"
  printf '%s\n' "$version_output" | grep "codex-switch " >/dev/null
  if printf '%s\n' "$version_output" | grep "0.1.0-dev" >/dev/null; then
    echo "release archive still reports dev version: $version_output" >&2
    exit 1
  fi
  install_dir="$tmp_dir/install"
  skill_home="$tmp_dir/skill-home/.codex"
  (cd "$tmp_dir" && PATH="$install_dir/bin:/usr/bin:/bin:/usr/sbin:/sbin" CODEX_HOME="$skill_home" CODEX_SWITCH_RELEASE_DIR="$dist_dir" BIN_DIR="install/bin" "$repo_dir/scripts/install.sh" --from-release --alias --skill >/dev/null)
  test -x "$install_dir/bin/codex-switch"
  if find "$install_dir/bin" -maxdepth 1 -name '.codex-switch-install.*' | grep . >/dev/null; then
    echo "installer left a temporary binary staging file" >&2
    exit 1
  fi
  test -e "$install_dir/bin/cs"
  test -f "$skill_home/skills/codex-switch/SKILL.md"
  grep "codex-switch new --account NAME" "$skill_home/skills/codex-switch/SKILL.md" >/dev/null
  grep "codex-switch list --cwd ." "$skill_home/skills/codex-switch/SKILL.md" >/dev/null
  grep "codex-switch resume --account NAME --session SESSION_ID" "$skill_home/skills/codex-switch/SKILL.md" >/dev/null
  installed_version_output="$("$install_dir/bin/codex-switch" version)"
  if printf '%s\n' "$installed_version_output" | grep "0.1.0-dev" >/dev/null; then
    echo "installed release still reports dev version: $installed_version_output" >&2
    exit 1
  fi

  spaced_release_bin="$tmp_dir/release bin with spaces"
  CODEX_SWITCH_RELEASE_DIR="$dist_dir" BIN_DIR="$spaced_release_bin" "$repo_dir/scripts/install.sh" --from-release >/dev/null
  test -x "$spaced_release_bin/codex-switch"
  if find "$spaced_release_bin" -maxdepth 1 -name '.codex-switch-install.*' | grep . >/dev/null; then
    echo "release installer left a temporary binary staging file in a path with spaces" >&2
    exit 1
  fi

  alias_collision_dir="$tmp_dir/alias-collision"
  mkdir -p "$alias_collision_dir/bin"
  printf 'existing alias\n' >"$alias_collision_dir/bin/cs"
  (cd "$tmp_dir" && CODEX_SWITCH_RELEASE_DIR="$dist_dir" BIN_DIR="$alias_collision_dir/bin" "$repo_dir/scripts/install.sh" --from-release --alias >/dev/null 2>"$tmp_dir/alias-collision-stderr.txt")
  grep "cs already exists; leaving it untouched" "$tmp_dir/alias-collision-stderr.txt" >/dev/null
  test "$(cat "$alias_collision_dir/bin/cs")" = "existing alias"

  path_collision_dir="$tmp_dir/path-collision"
  mkdir -p "$path_collision_dir/existing" "$path_collision_dir/install"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$path_collision_dir/existing/cs"
  chmod +x "$path_collision_dir/existing/cs"
  (cd "$tmp_dir" && PATH="$path_collision_dir/existing:$PATH" CODEX_SWITCH_RELEASE_DIR="$dist_dir" BIN_DIR="$path_collision_dir/install" "$repo_dir/scripts/install.sh" --from-release --alias >/dev/null 2>"$tmp_dir/path-collision-stderr.txt")
  grep "cs already exists; leaving it untouched" "$tmp_dir/path-collision-stderr.txt" >/dev/null
  test ! -e "$path_collision_dir/install/cs"

  bad_release="$tmp_dir/bad-release"
  mkdir -p "$bad_release"
  cp "$archive" "$bad_release/$(basename "$archive")"
  awk -v asset="$(basename "$archive")" '
    $2 == asset {$1 = "0000000000000000000000000000000000000000000000000000000000000000"}
    {print}
  ' "$dist_dir/checksums.txt" >"$bad_release/checksums.txt"
  if CODEX_SWITCH_RELEASE_DIR="$bad_release" BIN_DIR="$tmp_dir/bad-install/bin" "$repo_dir/scripts/install.sh" --from-release >/dev/null 2>&1; then
    echo "installer accepted archive with bad checksum" >&2
    exit 1
  fi
fi

echo "archive smoke ok"
