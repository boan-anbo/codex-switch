package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed skills/codex-switch/SKILL.md
var embeddedSkills embed.FS

func cmdCurrent(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	fs := flag.NewFlagSet("current", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: codex-switch current [--json]")
	}
	if err := ensureRuntimeAccountHomesUnique(cfg); err != nil {
		return err
	}
	account := provider.CurrentAccount(cfg)
	status := provider.Status(ctx, cfg, account, false)
	if *jsonOut {
		return printJSON(status)
	}
	fmt.Println(formatAccountStatus(status))
	fmt.Println("home:", status.Account.Home)
	return nil
}

func cmdAccounts(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	fs := flag.NewFlagSet("accounts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit JSON")
	refresh := fs.Bool("refresh", false, "ignore quota cache")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: codex-switch accounts [--json] [--refresh]")
	}
	if err := ensureRuntimeAccountHomesUnique(cfg); err != nil {
		return err
	}
	statuses := provider.Statuses(ctx, cfg, *refresh)
	if *jsonOut {
		return printJSON(statuses)
	}
	for _, status := range statuses {
		fmt.Println(formatAccountStatus(status))
	}
	return nil
}

func cmdQuota(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	name, all, jsonOut, refresh, err := parseQuotaArgs(args)
	if err != nil {
		return err
	}
	var statuses []AccountStatus
	if all {
		if err := ensureRuntimeAccountHomesUnique(cfg); err != nil {
			return err
		}
		statuses = provider.Statuses(ctx, cfg, refresh)
	} else if name == "" {
		if err := ensureRuntimeAccountHomesUnique(cfg); err != nil {
			return err
		}
		statuses = provider.Statuses(ctx, cfg, refresh)
	} else {
		account, ok := resolveAccount(cfg, name)
		if !ok {
			return errors.New("invalid account name " + name)
		}
		if err := ensureAccountHomeUnique(cfg, account); err != nil {
			return err
		}
		status := provider.Status(ctx, cfg, account, refresh)
		statuses = []AccountStatus{status}
	}
	if jsonOut {
		return printJSON(statuses)
	}
	for _, status := range statuses {
		if !status.Auth.LoggedIn {
			fmt.Printf("%-14s not logged in\n", status.Account.Name)
			continue
		}
		if status.QuotaError != "" {
			fmt.Printf("%-14s quota unavailable: %s\n", status.Account.Name, status.QuotaError)
			continue
		}
		if status.Quota == nil {
			fmt.Printf("%-14s quota unavailable\n", status.Account.Name)
			continue
		}
		fmt.Printf("%-14s %s\n", status.Account.Name, formatLiveRateLimit(status.Quota.RateLimits))
	}
	return nil
}

func cmdAccount(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	if len(args) == 0 {
		return errors.New(accountUsage)
	}
	switch args[0] {
	case "add":
		return cmdAccountAdd(ctx, cfg, provider, args[1:])
	case "list":
		return cmdAccounts(ctx, cfg, provider, args[1:])
	default:
		return fmt.Errorf("unknown account subcommand %q", args[0])
	}
}

const accountUsage = "usage: codex-switch account list [--json] [--refresh]\n       codex-switch account add NAME [--home PATH] [--label TEXT] [--login]"

func cmdAccountAdd(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	name, accountHome, label, login, err := parseAccountAddArgs(args)
	if err != nil {
		return err
	}
	if !validAccountName(name) {
		return errors.New("account name " + accountNameRule)
	}
	if cfg.HasAccount(name) {
		return fmt.Errorf("account %s already exists", name)
	}
	if _, ok := accountHomeEnvOverride(name); ok {
		return fmt.Errorf("account %s is set by %s; unset it before adding a configured account", name, accountHomeEnvName(name))
	}
	if accountHome == "" {
		if name == "default" {
			accountHome = filepath.Join(homeDir(), ".codex")
		} else {
			accountHome = filepath.Join(homeDir(), ".codex-"+name)
		}
	}
	account := Account{Name: name, Home: cleanPath(accountHome), Label: label}
	for _, existing := range cfg.AccountsList() {
		if existing.Name != account.Name && samePath(existing.Home, account.Home) {
			return fmt.Errorf("account home %s already used by %s", account.Home, existing.Name)
		}
	}
	if err := provider.InitAccountHome(cfg, account); err != nil {
		return err
	}
	next := cloneConfig(cfg)
	next.UpsertAccount(account)
	if err := SaveConfig(next); err != nil {
		return err
	}
	*cfg = *next
	fmt.Printf("added %s at %s\n", account.Name, account.Home)
	if login {
		if err := ensureAccountHomeUnique(cfg, account); err != nil {
			return err
		}
		return provider.Launch(LaunchOptions{Account: account, Args: []string{"login"}}, cfg)
	}
	_ = ctx
	return nil
}

func cmdInitAccount(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: codex-switch init-account NAME")
	}
	name := args[0]
	if !validAccountName(name) {
		return errors.New("account name " + accountNameRule)
	}
	if cfg.HasAccount(name) {
		account, ok := resolveAccount(cfg, name)
		if !ok {
			return errors.New("invalid account name " + name)
		}
		if err := ensureAccountHomeUnique(cfg, account); err != nil {
			return err
		}
		if err := provider.InitAccountHome(cfg, account); err != nil {
			return err
		}
		fmt.Printf("initialized %s at %s\n", account.Name, account.Home)
		_ = ctx
		return nil
	}
	if _, ok := accountHomeEnvOverride(name); ok {
		account, ok := resolveAccount(cfg, name)
		if !ok {
			return errors.New("invalid account name " + name)
		}
		if err := ensureAccountHomeUnique(cfg, account); err != nil {
			return err
		}
		if err := provider.InitAccountHome(cfg, account); err != nil {
			return err
		}
		fmt.Printf("initialized %s at %s\n", account.Name, account.Home)
		_ = ctx
		return nil
	}
	return cmdAccountAdd(ctx, cfg, provider, []string{name})
}

func cmdLogin(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	name, printOnly, err := parseLoginArgs(args, cfg.Defaults.Account)
	if err != nil {
		return err
	}
	account, ok := resolveAccount(cfg, name)
	if !ok {
		return errors.New("invalid account name " + name)
	}
	_ = ctx
	return launchAccountCodex(provider, cfg, LaunchOptions{Account: account, Args: []string{"login"}, Print: printOnly})
}

func cmdRunCodex(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	accountName, rest, consumedLeadingAccount, err := consumeLeadingRunAccount(args, cfg.Defaults.Account)
	if err != nil {
		return err
	}
	accountName, printOnly, rest, err := parseRunWrapperArgs(accountName, rest, consumedLeadingAccount)
	if err != nil {
		return err
	}
	account, ok := resolveAccount(cfg, accountName)
	if !ok {
		return errors.New("invalid account name " + accountName)
	}
	_ = ctx
	return launchAccountCodex(provider, cfg, LaunchOptions{Account: account, Args: rest, Print: printOnly})
}

func cmdNew(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	accountName, rest, consumedLeadingAccount := consumeLeadingConfiguredAccount(args, cfg.Defaults.Account, cfg)
	printOnly, rest := consumeBoolFlagBeforeDoubleDash(rest, "--print")
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	accountFlag := fs.String("account", "", "account name")
	preset := fs.String("preset", "", "launch preset")
	cwd := fs.String("cwd", "", "working directory")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if *accountFlag != "" {
		if consumedLeadingAccount {
			return errors.New("usage: codex-switch new [NAME|--account NAME] [--preset NAME] [--cwd DIR] [--print] -- [codex args...]")
		}
		if !validAccountName(*accountFlag) {
			return errors.New("account name " + accountNameRule)
		}
		accountName = *accountFlag
	}
	if *cwd == "" {
		*cwd, _ = os.Getwd()
	}
	codexArgs, err := provider.NewArgs(cfg, *preset, *cwd, trimDoubleDash(fs.Args()))
	if err != nil {
		return err
	}
	account, ok := resolveAccount(cfg, accountName)
	if !ok {
		return errors.New("invalid account name " + accountName)
	}
	_ = ctx
	return launchAccountCodex(provider, cfg, LaunchOptions{Account: account, Args: codexArgs, Print: printOnly})
}

func cmdResume(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	accountName, rest, consumedLeadingAccount := consumeLeadingConfiguredAccount(args, cfg.Defaults.Account, cfg)
	printOnly, rest := consumeBoolFlagBeforeDoubleDash(rest, "--print")
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	accountFlag := fs.String("account", "", "account name")
	preset := fs.String("preset", "", "launch preset")
	cwd := fs.String("cwd", "", "working directory")
	session := fs.String("session", "", "session id")
	last := fs.Bool("last", false, "resume latest")
	all := fs.Bool("all", false, "show all sessions")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if *accountFlag != "" {
		if consumedLeadingAccount {
			return errors.New("usage: codex-switch resume [NAME|--account NAME] [--last|--all|--session ID] [--cwd DIR] [--print] -- [codex args...]")
		}
		if !validAccountName(*accountFlag) {
			return errors.New("account name " + accountNameRule)
		}
		accountName = *accountFlag
	}
	if *cwd == "" && !*all {
		*cwd, _ = os.Getwd()
	}
	passthrough := trimDoubleDash(fs.Args())
	if *session == "" && len(passthrough) > 0 && !strings.HasPrefix(passthrough[0], "-") {
		*session = passthrough[0]
		passthrough = passthrough[1:]
	}
	resumeSelectors := 0
	if *last {
		resumeSelectors++
	}
	if *all {
		resumeSelectors++
	}
	if *session != "" {
		resumeSelectors++
	}
	if resumeSelectors > 1 {
		return errors.New("choose only one of --last, --all, or --session")
	}
	codexArgs, err := provider.ResumeArgs(cfg, *preset, *cwd, *session, *last, *all, passthrough)
	if err != nil {
		return err
	}
	account, ok := resolveAccount(cfg, accountName)
	if !ok {
		return errors.New("invalid account name " + accountName)
	}
	_ = ctx
	return launchAccountCodex(provider, cfg, LaunchOptions{Account: account, Args: codexArgs, Print: printOnly})
}

func launchAccountCodex(provider Provider, cfg *Config, opts LaunchOptions) error {
	opts.Account.Home = cleanPath(opts.Account.Home)
	if err := ensureAccountHomeUnique(cfg, opts.Account); err != nil {
		return err
	}
	if !opts.Print {
		if err := provider.InitAccountHome(cfg, opts.Account); err != nil {
			return err
		}
	}
	return provider.Launch(opts, cfg)
}

func ensureAccountHomeUnique(cfg *Config, selected Account) error {
	selected.Home = cleanPath(selected.Home)
	for _, account := range cfg.AccountsList() {
		if account.Name == selected.Name {
			continue
		}
		if samePath(account.Home, selected.Home) {
			return fmt.Errorf("account home %s already used by %s", selected.Home, account.Name)
		}
	}
	return nil
}

func ensureRuntimeAccountHomesUnique(cfg *Config) error {
	accounts := cfg.AccountsList()
	for i, account := range accounts {
		for _, other := range accounts[i+1:] {
			if samePath(account.Home, other.Home) {
				return fmt.Errorf("account home %s already used by %s and %s", cleanPath(account.Home), account.Name, other.Name)
			}
		}
	}
	return nil
}

func cmdList(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", cfg.UI.SessionLimit, "number of sessions")
	cwd := fs.String("cwd", "", "filter by cwd")
	jsonOut := fs.Bool("json", false, "emit JSON")
	sessionsDir := fs.String("sessions", provider.DefaultSessionsDir(), "sessions directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: codex-switch list [--cwd DIR] [--limit N] [--json] [--sessions DIR]")
	}
	if *limit <= 0 {
		return errors.New("limit must be greater than zero")
	}
	sessions, err := provider.CollectSessions(*sessionsDir, *limit, *cwd)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(sessions)
	}
	for i, session := range sessions {
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(formatSession(session))
		fmt.Println()
	}
	_ = ctx
	return nil
}

func cmdDoctor(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: codex-switch doctor")
	}
	configPath, _ := ConfigPath()
	cacheDir, _ := CacheDir()
	fmt.Println("version:", version)
	fmt.Println("config:", configPath)
	fmt.Println("cache:", cacheDir)
	fmt.Println("provider:", provider.Name())
	fmt.Println("codex:", cfg.Codex.Bin)
	if path, err := exec.LookPath(cfg.Codex.Bin); err == nil {
		fmt.Println("codex path:", path)
	} else {
		fmt.Println("codex path: not found")
		fmt.Println("repair: install Codex CLI or set [codex].bin in the config file")
	}
	if err := ValidateConfig(cfg); err != nil {
		fmt.Println("config validation:", err)
	} else {
		fmt.Println("config validation: ok")
	}
	if err := ensureRuntimeAccountHomesUnique(cfg); err != nil {
		fmt.Println("runtime account homes:", err)
	} else {
		fmt.Println("runtime account homes: ok")
	}
	if _, err := os.Stat(configPath); err != nil {
		fmt.Println("config file: not written yet")
	} else {
		fmt.Println("config file: present")
	}
	fmt.Println("shared home:", expandHome(cfg.Defaults.ShareFromHome))
	for _, account := range cfg.AccountsList() {
		status := AccountStatus{Account: account, Auth: provider.AuthInfo(account)}
		fmt.Println(formatAccountStatus(status))
		fmt.Println("  home:", account.Home)
		if _, ok := accountHomeEnvOverride(account.Name); ok {
			fmt.Println("  home override:", accountHomeEnvName(account.Name))
		}
		for _, item := range cfg.Defaults.Share {
			fmt.Println("  share:", item, provider.SharedAssetState(cfg, account, item))
		}
	}
	_ = ctx
	return nil
}

func sharedAssetState(cfg *Config, account Account, item string) string {
	source := filepath.Join(expandHome(cfg.Defaults.ShareFromHome), item)
	dest := filepath.Join(expandHome(account.Home), item)
	if _, err := os.Stat(source); err != nil {
		return "source missing"
	}
	info, err := os.Lstat(dest)
	if err != nil {
		return "not linked"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(dest)
		if err != nil {
			return "link unreadable"
		}
		if samePath(target, source) {
			return "linked"
		}
		return "linked elsewhere"
	}
	return "exists locally"
}

func cmdSkill(ctx context.Context, cfg *Config, provider Provider, args []string) error {
	if len(args) != 1 || args[0] != "install" {
		return errors.New("usage: codex-switch skill install")
	}
	data, err := embeddedSkills.ReadFile("skills/codex-switch/SKILL.md")
	if err != nil {
		return err
	}
	account := provider.CurrentAccount(cfg)
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		home = account.Home
	}
	dst := filepath.Join(expandHome(home), "skills", "codex-switch", "SKILL.md")
	if err := copySkillFile(dst, string(data)); err != nil {
		return err
	}
	fmt.Println("installed skill:", dst)
	_ = ctx
	return nil
}

func printJSON(value any) error {
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func consumeLeadingConfiguredAccount(args []string, fallback string, cfg *Config) (string, []string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || !cfg.HasAccount(args[0]) {
		return fallback, args, false
	}
	return args[0], args[1:], true
}

func consumeLeadingRunAccount(args []string, fallback string) (string, []string, bool, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fallback, args, false, nil
	}
	if !validAccountName(args[0]) {
		return "", nil, false, errors.New("account name " + accountNameRule)
	}
	return args[0], args[1:], true, nil
}

func resolveAccount(cfg *Config, name string) (Account, bool) {
	if !validAccountName(name) {
		return Account{}, false
	}
	return cfg.Account(name)
}

func parseLoginArgs(args []string, fallback string) (string, bool, error) {
	name := fallback
	printOnly := false
	hasAccount := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--print":
			printOnly = true
		case arg == "--account":
			if i+1 >= len(args) {
				return "", false, errors.New("missing value for --account")
			}
			if hasAccount {
				return "", false, errors.New("usage: codex-switch login [NAME|--account NAME] [--print]")
			}
			name = args[i+1]
			if !validAccountName(name) {
				return "", false, errors.New("account name " + accountNameRule)
			}
			hasAccount = true
			i++
		case strings.HasPrefix(arg, "--account="):
			if hasAccount {
				return "", false, errors.New("usage: codex-switch login [NAME|--account NAME] [--print]")
			}
			name = strings.TrimPrefix(arg, "--account=")
			if !validAccountName(name) {
				return "", false, errors.New("account name " + accountNameRule)
			}
			hasAccount = true
		case strings.HasPrefix(arg, "-"):
			return "", false, errors.New("usage: codex-switch login [NAME|--account NAME] [--print]")
		default:
			if hasAccount {
				return "", false, errors.New("usage: codex-switch login [NAME|--account NAME] [--print]")
			}
			if !validAccountName(arg) {
				return "", false, errors.New("account name " + accountNameRule)
			}
			name = arg
			hasAccount = true
		}
	}
	return name, printOnly, nil
}

func parseAccountAddArgs(args []string) (name string, home string, label string, login bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--login":
			login = true
		case arg == "--home":
			if i+1 >= len(args) {
				return "", "", "", false, errors.New("missing value for --home")
			}
			home = args[i+1]
			i++
		case strings.HasPrefix(arg, "--home="):
			home = strings.TrimPrefix(arg, "--home=")
		case arg == "--label":
			if i+1 >= len(args) {
				return "", "", "", false, errors.New("missing value for --label")
			}
			label = args[i+1]
			i++
		case strings.HasPrefix(arg, "--label="):
			label = strings.TrimPrefix(arg, "--label=")
		case strings.HasPrefix(arg, "-"):
			return "", "", "", false, errors.New("usage: codex-switch account add NAME [--home PATH] [--label TEXT] [--login]")
		default:
			if name != "" {
				return "", "", "", false, errors.New("usage: codex-switch account add NAME [--home PATH] [--label TEXT] [--login]")
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", "", false, errors.New("usage: codex-switch account add NAME [--home PATH] [--label TEXT] [--login]")
	}
	return name, home, label, login, nil
}

func parseQuotaArgs(args []string) (name string, all bool, jsonOut bool, refresh bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--all":
			all = true
		case "--json":
			jsonOut = true
		case "--refresh":
			refresh = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, false, false, errors.New("usage: codex-switch quota [NAME|--all] [--json] [--refresh]")
			}
			if name != "" {
				return "", false, false, false, errors.New("usage: codex-switch quota [NAME|--all] [--json] [--refresh]")
			}
			name = arg
		}
	}
	if all && name != "" {
		return "", false, false, false, errors.New("usage: codex-switch quota [NAME|--all] [--json] [--refresh]")
	}
	return name, all, jsonOut, refresh, nil
}

func parseRunWrapperArgs(name string, args []string, consumedLeadingAccount bool) (string, bool, []string, error) {
	printOnly := false
	rest := make([]string, 0, len(args))
	passthrough := false
	hasAccount := consumedLeadingAccount
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if passthrough {
			rest = append(rest, arg)
			continue
		}
		switch {
		case arg == "--":
			passthrough = true
		case arg == "--print":
			printOnly = true
		case arg == "--account":
			if i+1 >= len(args) {
				return "", false, nil, errors.New("missing value for --account")
			}
			if hasAccount {
				return "", false, nil, errors.New("usage: codex-switch run [NAME|--account NAME] [--print] -- [codex args...]")
			}
			name = args[i+1]
			if !validAccountName(name) {
				return "", false, nil, errors.New("account name " + accountNameRule)
			}
			hasAccount = true
			i++
		case strings.HasPrefix(arg, "--account="):
			if hasAccount {
				return "", false, nil, errors.New("usage: codex-switch run [NAME|--account NAME] [--print] -- [codex args...]")
			}
			name = strings.TrimPrefix(arg, "--account=")
			if !validAccountName(name) {
				return "", false, nil, errors.New("account name " + accountNameRule)
			}
			hasAccount = true
		default:
			rest = append(rest, arg)
		}
	}
	return name, printOnly, rest, nil
}

func trimDoubleDash(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

func consumeBoolFlagBeforeDoubleDash(args []string, flagName string) (bool, []string) {
	out := make([]string, 0, len(args))
	found := false
	passthrough := false
	for _, arg := range args {
		if passthrough {
			out = append(out, arg)
			continue
		}
		if arg == "--" {
			passthrough = true
			out = append(out, arg)
			continue
		}
		if arg == flagName {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return found, out
}
