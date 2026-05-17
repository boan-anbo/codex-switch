# codex-switch

[![CI](https://github.com/boan-anbo/codex-switch/actions/workflows/ci.yml/badge.svg)](https://github.com/boan-anbo/codex-switch/actions/workflows/ci.yml)

`codex-switch` is a small Go CLI for running Codex with multiple local account
homes without logging accounts in and out of the same `CODEX_HOME`.

It gives each account isolated auth/state files while sharing sessions and
configuration by default, so one account can resume the same conversation
without forcing another account to refresh.

Status: pre-release. The CLI is usable from this checkout, but no official tag,
release artifact, Homebrew formula, Scoop manifest, or package-manager
distribution should be treated as ready yet.

## Install

From this checkout:

```sh
go install .
```

After the first tagged release, install the binary archive without a Go
toolchain. The install scripts verify the downloaded archive against
GoReleaser's `checksums.txt` before installing:

```sh
curl -fsSL https://raw.githubusercontent.com/boan-anbo/codex-switch/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/boan-anbo/codex-switch/main/scripts/install.ps1 | iex
```

Source install will also work:

```sh
go install github.com/boan-anbo/codex-switch@latest
```

The full command is `codex-switch`. You can also install a short `cs` shim if
the name is free:

```sh
./scripts/install.sh --alias
```

Both source and release installer modes honor `--bin-dir DIR` / `-BinDir DIR`;
`~` and relative paths are normalized before installation. Add `--skill` /
`-Skill` when you also want the bundled Codex agent skill installed into the
current `CODEX_HOME`.

When installing from the published script, pass POSIX installer options after
`sh -s --`:

```sh
curl -fsSL https://raw.githubusercontent.com/boan-anbo/codex-switch/main/scripts/install.sh | sh -s -- --alias --skill
```

For PowerShell options, download the installer first so parameters are explicit:

```powershell
$installer = "$env:TEMP\install-codex-switch.ps1"
iwr https://raw.githubusercontent.com/boan-anbo/codex-switch/main/scripts/install.ps1 -OutFile $installer
& $installer -Alias -Skill
```

## Quick Start

```sh
codex-switch
codex-switch account add work --login
codex-switch accounts --refresh
codex-switch quota --all --refresh
codex-switch new work --print
codex-switch resume work --last --print
codex-switch list --cwd .
codex-switch run work -- login
```

The default picker asks for an account first, shows compact weekly usage and
reset timing, then asks whether to start a new Codex session, resume in the
current directory, resume across all directories, or login/refresh that account.
It opens from local account/auth state first, then loads usage in the background
so stale quota checks do not block the picker from appearing.

For scripts, `--print` shows a shell-appropriate command with `CODEX_HOME` set
without initializing account homes or launching Codex. On macOS/Linux this uses
POSIX syntax; on Windows it uses PowerShell syntax. Non-print launches initialize
the selected account home first. For `new` and `resume`, positional account
names are only recognized for configured accounts; use `--account NAME` when
targeting a new or unlisted account so prompts and resume session IDs pass
through cleanly. `codex-switch list` prints account-aware resume hints such as
`codex-switch resume --session SESSION_ID`; use `--sessions DIR` when Codex
sessions live outside the selected/default Codex home. `login NAME` and
`run NAME` accept any valid account name.

## Account Model

By default:

- `default` uses `~/.codex`
- named accounts use `~/.codex-NAME`
- existing `~/.codex-*` directories are discovered automatically

`account add` creates a new configured account, initializes its home, and links
shared assets from `~/.codex`. It refuses to overwrite an existing account name;
use `init-account` when you only want to prepare an existing account home.

- `config.toml`
- `skills`
- `memories`
- `AGENTS.md`
- `sessions`

`auth.json` and Codex state DB files are not linked. That is the isolation
boundary. On Windows, directory shares use junctions when normal symlinks are
not available. Custom shared assets must be single basenames; `auth.json`,
history, and SQLite state files are rejected.

`codex-switch init-account NAME` initializes and links an existing account home
without starting a login flow. If the name is not configured yet, it creates the
standard `~/.codex-NAME` account first.

## Config

The config file is written to the platform config directory:

- macOS: `~/Library/Application Support/codex-switch/config.toml`
- Linux: `~/.config/codex-switch/config.toml`
- Windows: `%AppData%\codex-switch\config.toml`

Example:

```toml
[codex]
bin = "codex"

[ui]
quota_ttl_seconds = 30
session_limit = 80

[defaults]
account = "default"
preset = "safe"
share_from_home = "~/.codex"
share = ["config.toml", "skills", "memories", "AGENTS.md", "sessions"]

[[accounts]]
name = "default"
home = "~/.codex"

[[accounts]]
name = "work"
home = "~/.codex-work"

[[presets]]
name = "safe"
args = []

[[presets]]
name = "full-access"
args = ["--dangerously-bypass-approvals-and-sandbox", "--sandbox", "danger-full-access"]
```

Use presets when you want local defaults such as full workspace access. The
public default remains safe.

For temporary account-home overrides, set
`CODEX_SWITCH_ACCOUNT_<NAME>_HOME=/path/to/home`; account names are uppercased
and `.`/`-` become `_` in the variable name, so `work.prod` uses
`CODEX_SWITCH_ACCOUNT_WORK_PROD_HOME`. Runtime duplicate homes are rejected.
`account add` refuses to create a configured account while an override for that
name is active; use `init-account NAME` to prepare the override home without
writing it into the config.

## Quota

Quota is fetched by reading the selected account home's Codex access token and
calling the same ChatGPT usage endpoint Codex uses. This is live account data
when available. If the endpoint changes or a login is stale, the picker still
works and shows a login/refresh action.

`codex-switch` never prints access tokens or refresh tokens.

Use `codex-switch doctor` to inspect config paths, Codex binary detection,
account homes, and shared asset links without printing auth tokens.

## Agent Skill

Install the bundled skill into the current Codex home:

```sh
codex-switch skill install
```

The skill teaches Codex agents to use this CLI for account/status/quota/login
flows without inspecting tokens.

## Cross-Platform Testing

The main portability gate is GitHub Actions with hosted Linux, macOS, and
Windows runners. For public repositories, GitHub currently documents standard
GitHub-hosted runners as free and lists the native runner labels used here:
<https://docs.github.com/en/actions/reference/runners/github-hosted-runners>.
That is the intended cheap OSS path; no local Windows or macOS VM is required
to prove native execution.

CI runs `go test ./...` and `go vet ./...` on all three operating systems, plus
Ubuntu Staticcheck, govulncheck, CodeQL, and race-detector jobs for dependency
vulnerability scanning, semantic code scanning, static analysis, and concurrent
code paths. The test suite includes a binary-level smoke test that builds
`codex-switch`, runs scriptable commands, and launches a fake Codex executable
to verify `CODEX_HOME` isolation.
Installer smoke also runs natively on all three operating systems. For packaged
binaries, CI uses a cheaper release-shaped gate: GoReleaser builds the full
snapshot artifact set once on Ubuntu, uploads `dist/`, and then
Linux/macOS/Windows runners download it and smoke their native archive. Archive
smoke verifies release version injection, checksum rejection, the release
installer, the `cs` alias, and bundled skill installation from the packaged
binary. The workflow can be started manually from GitHub Actions with
`workflow_dispatch`.

For a fuller public-repo portability check, `platform-smoke.yml` builds one
snapshot and then runs tests, vet, source-install smoke, and native
release-archive smoke on hosted runners. Its default `core` tier covers the
most common native targets cheaply: Linux amd64, macOS arm64, and Windows
amd64. It also runs on a weekly schedule so drift in hosted images, shell
behavior, or release packaging is caught without asking contributors to own
local VMs. The optional `full` tier adds Linux arm64, macOS Intel, and Windows
arm64. Keep the full tier as a deliberate gate for release candidates or
portability-sensitive changes rather than the default PR path.

After pushing a branch, the manual hosted check is:

```sh
gh workflow run ci.yml --ref <branch>
gh run watch --exit-status
```

The cheap/default native OS check is:

```sh
gh workflow run platform-smoke.yml --ref <branch>
gh run watch --exit-status
```

The full native OS/architecture check is:

```sh
gh workflow run platform-smoke.yml --ref <branch> -f tier=full
gh run watch --exit-status
```

Use the hosted matrix as the final cross-platform signal. `GOOS`/`GOARCH`
cross-compilation is useful for build coverage, but it does not prove Windows
PowerShell behavior, path normalization, executable shims, or native archive
installation.

The cheaper local preflight is:

```sh
./scripts/verify.sh
./scripts/verify.sh --release
```

Windows PowerShell:

```powershell
./scripts/verify.ps1
./scripts/verify.ps1 -Release
```

`verify` runs workflow linting, Go formatting, Go module tidiness checks, tests,
vet, Staticcheck, vulnerability checks, source-install smoke, shell script
formatting checks, and GoReleaser config validation. POSIX verification and
hosted Ubuntu CI also run shell script syntax checks; PowerShell verification
runs the same shell check when `sh` is available. `--release` / `-Release` also builds a
snapshot release, smokes the native archive, and removes `dist/` before
exiting. That catches most regressions quickly, but actual cross-platform
confirmation is the GitHub Actions OS matrix. CI uses the patched Go toolchain
version declared in
`go.mod`, allows Go's normal `GOTOOLCHAIN=auto` behavior for pinned tooling, and
pins GoReleaser, Staticcheck, shfmt, and govulncheck instead of using moving
`latest` values.
Release builds inject the GoReleaser version into `codex-switch version`;
archive smoke fails if an artifact still reports the development fallback. The
tag-triggered release workflow waits for native Linux/macOS/Windows smoke tests
across amd64 and arm64 hosted runner labels, builds a single GoReleaser
snapshot, smokes the downloaded archives on those same native platforms, then
repeats the core Ubuntu preflight checks before creating a draft release.
