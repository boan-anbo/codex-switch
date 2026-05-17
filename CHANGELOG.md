# Changelog

All notable changes to `codex-switch` will be documented here.

This project has not shipped a public release yet.

## Unreleased

- Initial standalone Go CLI extraction.
- Native account/action picker.
- Picker opens from local account/auth state first and loads usage in the
  background so stale quota checks do not block startup.
- Isolated Codex account homes with shared sessions/config by default.
- Non-destructive account creation: `account add` refuses existing account names,
  while `init-account` prepares existing homes without rewriting mappings.
- Live best-effort Codex usage adapter with graceful login-refresh errors.
- Safe launch presets, scriptable `new`, `resume`, `run`, `login`, `quota`,
  `doctor`, and `skill install` commands.
- Binary release packaging for macOS, Linux, and Windows on amd64 and arm64.
- Cross-platform CI with workflow linting, Go tests, vet, vulnerability checks,
  Go module tidiness checks, Staticcheck, an Ubuntu race-detector job, native
  smoke tests, installer smoke tests, and GoReleaser archive smoke tests.
- Tiered platform smoke workflow with weekly cheap/common native coverage and
  an on-demand full OS/architecture matrix for release candidates.
- Release installers with GoReleaser checksum verification and configurable
  install directories, including paths with spaces.
- Test harness isolation so automated tests do not read the developer's real
  Codex homes, plus hosted Windows checks that PowerShell smoke scripts restore
  caller environment variables.
- Bundled Codex agent skill for safe account/status/quota/login workflows.
- Local `verify` scripts for repeatable preflight and release-shaped smoke
  checks, including Go formatting plus shell script syntax and formatting
  checks.
- Runtime duplicate-home guards, account-home override diagnostics, and
  automatic discovery of existing `~/.codex-*` account homes.
- `--print` command mode that emits shell-specific launch commands without
  initializing account homes.
