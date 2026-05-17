# Security Policy

`codex-switch` manages local Codex account homes and reads Codex auth metadata
to report login status and usage. Treat auth files as sensitive.

## Supported Versions

No public release is supported yet. Security fixes will be handled on the main
pre-release branch until the first tagged version exists.

## Reporting a Vulnerability

Do not open a public issue that includes tokens, raw `auth.json` contents, or
other secrets.

Use GitHub private vulnerability reporting if it is enabled for the repository.
If it is not enabled, open a public issue that only asks for a private security
contact path; do not include exploit details, tokens, raw auth files, account
home contents, or sensitive logs in that public issue.

Include:

- affected OS and `codex-switch` version or commit
- command that triggered the issue
- redacted output
- whether token, auth, account home, or session data may have been exposed
- whether `codex-switch doctor` shows unexpected account-home or shared-asset
  state after redaction

## Security Invariants

- Never print access tokens, refresh tokens, bearer strings, or raw auth files.
- Keep `auth.json` and Codex state DB files isolated per account home.
- Shared sessions/config/skills/memories must be explicit and inspectable.
- Quota failure must not trigger destructive login/logout behavior.
- Release installers must verify archive hashes against GoReleaser
  `checksums.txt` before installing downloaded binaries.

## Dependency Checks

CI runs Go tests, `go vet`, Staticcheck, `govulncheck`, Go module tidiness
checks, shell script syntax checks, and an Ubuntu race-detector job. Dependabot
is configured for Go modules and GitHub Actions so dependency updates are
surfaced before release.
