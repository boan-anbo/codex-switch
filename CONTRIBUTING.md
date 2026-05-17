# Contributing

`codex-switch` is in pre-release iteration. Contributions should be small,
test-backed, and aligned with the account-isolation model.

## Development

```sh
./scripts/verify.sh
./scripts/verify.sh --release
pwsh ./scripts/verify.ps1
pwsh ./scripts/verify.ps1 -Release
```

`go test ./...` includes a binary-level CLI smoke test that builds
`codex-switch`, runs scriptable commands, uses a fake Codex executable, and
checks account-home initialization. GitHub Actions runs that test suite on
Linux, macOS, and Windows, plus Ubuntu race-detector, Staticcheck,
govulncheck, and CodeQL jobs for concurrent status/quota paths, static
analysis, dependency vulnerability scanning, and semantic code scanning.
Installer smoke also runs natively on all three operating systems. The
packaged-binary gate builds the GoReleaser snapshot once on Ubuntu, uploads
`dist/`, and then smokes the downloaded native archive on Linux, macOS, and
Windows. For public repositories, GitHub currently documents standard
GitHub-hosted runners as free and lists the native runner labels used here:
<https://docs.github.com/en/actions/reference/runners/github-hosted-runners>.
CI also runs `actionlint`, Go formatting, Go module tidiness checks, shell
script syntax checks, and `shfmt` so workflow expression mistakes,
dependency-file drift, installer script parse errors, and formatting drift are
caught before relying on the hosted OS matrix. CI keeps `GOTOOLCHAIN=auto`
explicit so pinned tools can request a newer patched Go toolchain without
changing the module's `go` directive. The CI workflow cancels stale runs on the
same branch and every job has a timeout, so the public-runner path stays cheap
and bounded.

When you need real cross-platform confirmation before merging or tagging, push
the branch and run:

```sh
gh workflow run ci.yml --ref <branch>
gh run watch --exit-status
```

For a fuller public-repo check, run the platform smoke workflow. The default
`core` tier is the cheap/common gate:
Linux amd64, macOS arm64, and Windows amd64. It also runs weekly so platform
drift is caught even when nobody is actively cutting a release.

```sh
gh workflow run platform-smoke.yml --ref <branch>
gh run watch --exit-status
```

Use the `full` tier before tagging release candidates or after
portability-sensitive changes:

```sh
gh workflow run platform-smoke.yml --ref <branch> -f tier=full
gh run watch --exit-status
```

Both tiers build one GoReleaser snapshot on Ubuntu, then run tests, vet,
source-install smoke, and native archive smoke on hosted runners. The full tier
adds Linux arm64, macOS Intel, and Windows arm64. Keep it as a deliberate gate
rather than the default every-push workflow.

Do not treat local `GOOS`/`GOARCH` cross-compilation as the final signal. It is
good build coverage, but it does not run Windows PowerShell, macOS/Linux shell
installers, native archive extraction, or executable alias checks.

For release packaging checks:

```sh
./scripts/verify.sh --release
pwsh ./scripts/verify.ps1 -Release
```

The tag-triggered release workflow waits for native Linux/macOS/Windows smoke
tests across amd64 and arm64 hosted runner labels, builds one GoReleaser
snapshot, smokes the downloaded archives on those same native platforms, then
repeats workflow linting, Go formatting, shell formatting, tests, vet, Go
module tidiness checks, Staticcheck, vulnerability checks, GoReleaser config
validation, and snapshot archive smoke before publishing a draft release.
Remove `dist/` after local release snapshots. Use the same GoReleaser,
Staticcheck, shfmt, and govulncheck versions pinned in
`.github/workflows/*.yml` when updating this checklist.

## Expectations

- Do not add tmux or HackSquad coupling to this repo.
- Do not hard-code personal account names or local machine paths.
- Keep public defaults safe; risky Codex flags belong in named presets.
- Do not print or log raw auth files or token values.
- Add tests for command parsing, account isolation, quota/error behavior, and
  release/install flows touched by a change.
- Keep docs honest about pre-release status until the first tag ships.
