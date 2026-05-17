---
name: codex-switch
description: "Use codex-switch for local Codex multi-account work: account selection, isolated CODEX_HOME login, quota/deadline checks, and new/resume launches."
---

# Codex Switch

Use `codex-switch` when the user asks about Codex account switching, usage,
quota reset time, login refresh, or starting/resuming Codex under a different
account.

Core commands:

```sh
codex-switch accounts
codex-switch quota --all --refresh
codex-switch current
codex-switch doctor
codex-switch account add NAME --login
codex-switch init-account NAME
codex-switch login NAME
codex-switch new --account NAME -- [codex args...]
codex-switch list --cwd .
codex-switch resume --account NAME --last
codex-switch resume --account NAME --all
codex-switch resume --account NAME --session SESSION_ID
codex-switch run NAME -- login
```

Rules:

- Run `codex-switch current` first inside an existing Codex session when the
  user asks which account the process is using.
- Use `codex-switch accounts` and `codex-switch quota --all --refresh` for live
  account status; quota values are time-sensitive.
- Use `codex-switch run NAME -- login` or `codex-switch login NAME` to refresh
  a stale account without touching the default Codex home.
- Do not run plain `codex login` when the goal is account isolation.
- Do not inspect or print access tokens, refresh tokens, or raw auth files.
  Email, plan, quota, and reset times are acceptable status fields.
- Use `--account NAME` for `new` and `resume` when the account may be new,
  unconfigured, or easy to confuse with a prompt/session id. Positional account
  names are only recognized for configured accounts.
- Use `--print` when the user asks for a command to run manually; print mode
  shows the shell-specific `CODEX_HOME` command and does not initialize or
  launch anything.
- Use `codex-switch init-account NAME` to prepare an account home without
  starting a login flow.
- Use `codex-switch doctor` for config/account-home diagnostics; it must not
  fetch quota or print secrets.
- For same-conversation account switching, quit Codex, keep the session id, and
  resume it with `codex-switch resume --account NAME --session SESSION_ID`.
- `resume --all` is explicit cross-directory resume; default resume should stay
  scoped to the current directory.
