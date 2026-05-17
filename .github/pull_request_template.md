## Summary

## Verification

- [ ] `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`
- [ ] `sh -n scripts/*.sh`
- [ ] `go run mvdan.cc/sh/v3/cmd/shfmt@v3.12.0 -d -i 2 -ci scripts/*.sh`
- [ ] `gofmt -l .` prints no files
- [ ] `go mod tidy` leaves `go.mod` and `go.sum` unchanged
- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`
- [ ] `go test -race ./...` for concurrency-sensitive changes
- [ ] `go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...`
- [ ] `RUN_INSTALL_SMOKE=1 ./scripts/smoke.sh`
- [ ] `go run github.com/goreleaser/goreleaser/v2@v2.15.4 check`
- [ ] `./scripts/verify.sh --release`
- [ ] Hosted GitHub Actions CI matrix passed on Linux, macOS, and Windows, or the PR explains why it has not run yet.
- [ ] For release candidates or portability-sensitive changes, `platform-smoke.yml` passed on the native OS/architecture matrix, or the PR explains why it has not run yet.

## Safety

- [ ] Does not print or inspect raw tokens.
- [ ] Preserves per-account `CODEX_HOME` isolation.
- [ ] Keeps public defaults safe.
- [ ] Cleans `dist/` after any local GoReleaser snapshot.
