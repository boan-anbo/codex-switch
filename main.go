package main

import (
	"context"
	"fmt"
	"os"
)

const (
	appName = "codex-switch"
)

var version = "0.1.0-dev"

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printHelp()
			return nil
		case "version", "--version":
			fmt.Printf("%s %s\n", appName, version)
			return nil
		}
		if !isConfigCommand(args[0]) {
			return fmt.Errorf("unknown command %q; run %s help", args[0], appName)
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	provider := CodexProvider{}
	if len(args) == 0 {
		return RunPicker(ctx, cfg, provider)
	}

	switch args[0] {
	case "current":
		return cmdCurrent(ctx, cfg, provider, args[1:])
	case "accounts":
		return cmdAccounts(ctx, cfg, provider, args[1:])
	case "quota":
		return cmdQuota(ctx, cfg, provider, args[1:])
	case "account":
		return cmdAccount(ctx, cfg, provider, args[1:])
	case "init-account":
		return cmdInitAccount(ctx, cfg, provider, args[1:])
	case "login":
		return cmdLogin(ctx, cfg, provider, args[1:])
	case "run":
		return cmdRunCodex(ctx, cfg, provider, args[1:])
	case "new":
		return cmdNew(ctx, cfg, provider, args[1:])
	case "resume":
		return cmdResume(ctx, cfg, provider, args[1:])
	case "list":
		return cmdList(ctx, cfg, provider, args[1:])
	case "doctor":
		return cmdDoctor(ctx, cfg, provider, args[1:])
	case "skill":
		return cmdSkill(ctx, cfg, provider, args[1:])
	default:
		return fmt.Errorf("unknown command %q; run %s help", args[0], appName)
	}
}

func isConfigCommand(command string) bool {
	switch command {
	case "current", "accounts", "quota", "account", "init-account", "login", "run", "new", "resume", "list", "doctor", "skill":
		return true
	default:
		return false
	}
}

func printHelp() {
	fmt.Printf(`codex-switch - switch Codex accounts without breaking active sessions

Usage:
  codex-switch                         Open the interactive picker
  cs                                   Same, when the optional alias is installed
  codex-switch current [--json]
  codex-switch accounts [--json] [--refresh]
  codex-switch quota [NAME|--all] [--json] [--refresh]
  codex-switch account list [--json] [--refresh]
  codex-switch account add NAME [--home PATH] [--label TEXT] [--login]
  codex-switch init-account NAME
  codex-switch login [NAME|--account NAME] [--print]
  codex-switch new [NAME|--account NAME] [--preset NAME] [--cwd DIR] [--print] -- [codex args...]
  codex-switch resume [NAME|--account NAME] [--last|--all|--session ID] [--cwd DIR] [--print] -- [codex args...]
  codex-switch run [NAME|--account NAME] [--print] -- [codex args...]
  codex-switch list [--cwd DIR] [--limit N] [--json] [--sessions DIR]
  codex-switch doctor
  codex-switch skill install

Defaults:
  Account homes use ~/.codex for default and ~/.codex-NAME for named accounts.
  Existing ~/.codex-* homes are discovered automatically.
  Named accounts are initialized before non-print launches.
  Quota is fetched live when possible and cached briefly so the picker stays fast.
`)
}
