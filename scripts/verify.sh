#!/usr/bin/env sh
set -eu

repo_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
release=0
staticcheck_version="v0.7.0"
shfmt_version="v3.12.0"
govulncheck_version="v1.3.0"

usage() {
  cat <<'EOF'
usage: ./scripts/verify.sh [--release]

Runs the local preflight gates. Add --release to also build a GoReleaser
snapshot, smoke the native archive, and remove dist/ before exiting.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --release)
      release=1
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
done

run() {
  printf '\n==> %s\n' "$*"
  "$@"
}

check_gofmt() {
  printf '\n==> gofmt -l .\n'
  files="$(gofmt -l .)"
  if [ -n "$files" ]; then
    printf '%s\n' "$files"
    echo "Run gofmt before committing." >&2
    exit 1
  fi
}

check_go_mod_tidy() {
  tmp_dir="$(mktemp -d)"
  tidy_done=0
  cleanup_tidy() {
    rm -rf "$tmp_dir"
  }
  restore_go_mod() {
    cp "$tmp_dir/go.mod" go.mod
    cp "$tmp_dir/go.sum" go.sum
  }
  restore_go_mod_on_exit() {
    if [ "$tidy_done" -eq 0 ]; then
      restore_go_mod
    fi
    cleanup_tidy
  }
  cp go.mod "$tmp_dir/go.mod"
  cp go.sum "$tmp_dir/go.sum"
  trap restore_go_mod_on_exit EXIT INT TERM
  printf '\n==> go mod tidy\n'
  if ! go mod tidy; then
    restore_go_mod
    cleanup_tidy
    trap - EXIT INT TERM
    exit 1
  fi
  if ! cmp -s go.mod "$tmp_dir/go.mod" || ! cmp -s go.sum "$tmp_dir/go.sum"; then
    echo "go.mod or go.sum is not tidy; run go mod tidy" >&2
    diff -u "$tmp_dir/go.mod" go.mod || true
    diff -u "$tmp_dir/go.sum" go.sum || true
    restore_go_mod
    cleanup_tidy
    trap - EXIT INT TERM
    exit 1
  fi
  tidy_done=1
  cleanup_tidy
  trap - EXIT INT TERM
}

cd "$repo_dir"

run go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
for script in ./scripts/*.sh; do
  run sh -n "$script"
done
run go run "mvdan.cc/sh/v3/cmd/shfmt@$shfmt_version" -d -i 2 -ci ./scripts/*.sh
check_gofmt
check_go_mod_tidy
run go test ./...
run go vet ./...
run go run "honnef.co/go/tools/cmd/staticcheck@$staticcheck_version" ./...
run go run "golang.org/x/vuln/cmd/govulncheck@$govulncheck_version" ./...

printf '\n==> RUN_INSTALL_SMOKE=1 ./scripts/smoke.sh\n'
RUN_INSTALL_SMOKE=1 ./scripts/smoke.sh

run go run github.com/goreleaser/goreleaser/v2@v2.15.4 check

if [ "$release" -eq 1 ]; then
  cleanup_dist() {
    rm -rf "$repo_dir/dist"
  }
  trap cleanup_dist EXIT INT TERM

  run go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean
  run ./scripts/archive-smoke.sh
fi

echo
echo "verify ok"
