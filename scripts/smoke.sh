#!/usr/bin/env sh
set -eu

repo_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
tmp_dir="$(mktemp -d)"
cleanup() {
  chmod -R u+w "$tmp_dir" 2>/dev/null || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

home="$tmp_dir/home"
config="$tmp_dir/config.toml"
cache="$tmp_dir/cache"
bin="$tmp_dir/codex-switch"
mkdir -p "$home/.codex/sessions" "$cache"
touch "$home/.codex/config.toml"

cd "$repo_dir"
go build -o "$bin" .

HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" version >/dev/null
HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" help >/dev/null

accounts_json="$(HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" accounts --json)"
printf '%s\n' "$accounts_json" | grep '"name": "default"' >/dev/null

new_cmd="$(HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" new --account work --print -- --model gpt-test)"
printf '%s\n' "$new_cmd" | grep "CODEX_HOME=$home/.codex-work" >/dev/null
printf '%s\n' "$new_cmd" | grep -- "--model gpt-test" >/dev/null
test ! -e "$home/.codex-work/config.toml"
test ! -e "$home/.codex-work/sessions"

resume_cmd="$(HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" resume --account work --all --print)"
printf '%s\n' "$resume_cmd" | grep "CODEX_HOME=$home/.codex-work" >/dev/null
printf '%s\n' "$resume_cmd" | grep -- "resume --all --include-non-interactive" >/dev/null
test ! -e "$home/.codex-work/config.toml"
test ! -e "$home/.codex-work/sessions"

session_cmd="$(HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" resume 019d30aa-4798-7891-a56f-1f87a629e02c --print)"
printf '%s\n' "$session_cmd" | grep -- "resume --cd" >/dev/null
printf '%s\n' "$session_cmd" | grep -- "019d30aa-4798-7891-a56f-1f87a629e02c" >/dev/null

account_session_cmd="$(HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" resume --account work --session 019d30aa-4798-7891-a56f-1f87a629e02c --print)"
printf '%s\n' "$account_session_cmd" | grep "CODEX_HOME=$home/.codex-work" >/dev/null
printf '%s\n' "$account_session_cmd" | grep -- "resume --cd" >/dev/null
printf '%s\n' "$account_session_cmd" | grep -- "019d30aa-4798-7891-a56f-1f87a629e02c" >/dev/null
test ! -e "$home/.codex-work/config.toml"
test ! -e "$home/.codex-work/sessions"

login_cmd="$(HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" run work --print -- login)"
printf '%s\n' "$login_cmd" | grep "CODEX_HOME=$home/.codex-work" >/dev/null
printf '%s\n' "$login_cmd" | grep -- "codex login" >/dev/null
test ! -e "$home/.codex-work/config.toml"
test ! -e "$home/.codex-work/sessions"

HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" account add work2 >/dev/null
grep "name = 'work2'" "$config" >/dev/null
test ! -e "$home/.codex-work2/auth.json"

HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" init-account workinit >/dev/null
grep "name = 'workinit'" "$config" >/dev/null
test ! -e "$home/.codex-workinit/auth.json"

cat >"$tmp_dir/codex" <<'EOS'
#!/usr/bin/env sh
set -eu
printf '%s\n' "$CODEX_HOME" >"$HOME/launched-home.txt"
EOS
chmod +x "$tmp_dir/codex"
PATH="$tmp_dir:$PATH" HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" new --account work3 >/dev/null
grep "$home/.codex-work3" "$home/launched-home.txt" >/dev/null
test -e "$home/.codex-work3/config.toml"
test -e "$home/.codex-work3/sessions"
test ! -e "$home/.codex-work3/auth.json"

CODEX_HOME="$home/.codex" HOME="$home" CODEX_SWITCH_CONFIG="$config" CODEX_SWITCH_CACHE="$cache" "$bin" skill install >/dev/null
test -f "$home/.codex/skills/codex-switch/SKILL.md"
grep "codex-switch new --account NAME" "$home/.codex/skills/codex-switch/SKILL.md" >/dev/null
grep "codex-switch list --cwd ." "$home/.codex/skills/codex-switch/SKILL.md" >/dev/null
grep "codex-switch resume --account NAME --session SESSION_ID" "$home/.codex/skills/codex-switch/SKILL.md" >/dev/null

if [ "${RUN_INSTALL_SMOKE:-}" = "1" ]; then
  install_gopath="$tmp_dir/gopath"
  install_gomodcache="$tmp_dir/gomodcache"

  GOPATH="$install_gopath" GOMODCACHE="$install_gomodcache" CODEX_HOME="$home/.codex" "$repo_dir/scripts/install.sh" --bin-dir "$tmp_dir/source-bin" --skill >/dev/null
  test -x "$tmp_dir/source-bin/codex-switch"
  if find "$tmp_dir/source-bin" -maxdepth 1 -name '.codex-switch-install.*' | grep . >/dev/null; then
    echo "source installer left a temporary binary staging file" >&2
    exit 1
  fi
  test -f "$home/.codex/skills/codex-switch/SKILL.md"
  grep "codex-switch new --account NAME" "$home/.codex/skills/codex-switch/SKILL.md" >/dev/null
  grep "codex-switch list --cwd ." "$home/.codex/skills/codex-switch/SKILL.md" >/dev/null
  grep "codex-switch resume --account NAME --session SESSION_ID" "$home/.codex/skills/codex-switch/SKILL.md" >/dev/null

  alias_bin="$tmp_dir/existing-alias-bin"
  mkdir -p "$alias_bin"
  printf 'existing alias\n' >"$alias_bin/cs"
  GOPATH="$install_gopath" GOMODCACHE="$install_gomodcache" "$repo_dir/scripts/install.sh" --from-source --bin-dir "$alias_bin" --alias >/dev/null 2>"$tmp_dir/alias-stderr.txt"
  grep "cs already exists; leaving it untouched" "$tmp_dir/alias-stderr.txt" >/dev/null
  test "$(cat "$alias_bin/cs")" = "existing alias"

  path_collision_bin="$tmp_dir/path-collision-bin"
  path_collision_target="$tmp_dir/path-collision-target"
  mkdir -p "$path_collision_bin" "$path_collision_target"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$path_collision_bin/cs"
  chmod +x "$path_collision_bin/cs"
  PATH="$path_collision_bin:$PATH" GOPATH="$install_gopath" GOMODCACHE="$install_gomodcache" "$repo_dir/scripts/install.sh" --from-source --bin-dir "$path_collision_target" --alias >/dev/null 2>"$tmp_dir/path-alias-stderr.txt"
  grep "cs already exists; leaving it untouched" "$tmp_dir/path-alias-stderr.txt" >/dev/null
  test ! -e "$path_collision_target/cs"

  (cd "$tmp_dir" && GOPATH="$install_gopath" GOMODCACHE="$install_gomodcache" "$repo_dir/scripts/install.sh" --from-source --bin-dir "$tmp_dir/path-source-bin" >/dev/null)
  test -x "$tmp_dir/path-source-bin/codex-switch"

  (cd "$tmp_dir" && GOPATH="$install_gopath" GOMODCACHE="$install_gomodcache" "$repo_dir/scripts/install.sh" --from-source --bin-dir relative-bin >/dev/null)
  test -x "$tmp_dir/relative-bin/codex-switch"

  spaced_bin="$tmp_dir/source bin with spaces"
  GOPATH="$install_gopath" GOMODCACHE="$install_gomodcache" "$repo_dir/scripts/install.sh" --from-source --bin-dir "$spaced_bin" >/dev/null
  test -x "$spaced_bin/codex-switch"

  download_probe="$tmp_dir/download-probe"
  mkdir -p "$download_probe/bin"
  cat >"$download_probe/bin/curl" <<'EOS'
#!/usr/bin/env sh
set -eu
url=""
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      shift
      out="$1"
      ;;
    -*)
      ;;
    *)
      url="$1"
      ;;
  esac
  shift
done
printf '%s\n' "$url" >>"$CODEX_SWITCH_DOWNLOAD_LOG"
printf 'not a release artifact\n' >"$out"
EOS
  chmod +x "$download_probe/bin/curl"
  download_log="$download_probe/urls.txt"
  if PATH="$download_probe/bin:/usr/bin:/bin" CODEX_SWITCH_DOWNLOAD_LOG="$download_log" BIN_DIR="$download_probe/install" "$repo_dir/scripts/install.sh" --from-release --version v1.2.3 >/dev/null 2>&1; then
    echo "release installer unexpectedly accepted probe downloads" >&2
    exit 1
  fi
  grep "/releases/download/v1.2.3/codex-switch_" "$download_log" >/dev/null
  grep "/releases/download/v1.2.3/checksums.txt" "$download_log" >/dev/null
fi

echo "smoke ok"
