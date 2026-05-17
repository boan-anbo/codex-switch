package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndVersionDoNotRequireValidConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(config, []byte("not valid toml ="), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_SWITCH_CONFIG", config)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"help"}, want: "codex-switch - switch Codex accounts"},
		{name: "version", args: []string{"version"}, want: "codex-switch " + version},
		{name: "help flag", args: []string{"--help"}, want: "codex-switch - switch Codex accounts"},
		{name: "version flag", args: []string{"--version"}, want: "codex-switch " + version},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := run(context.Background(), tc.args); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(out, tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, out)
			}
		})
	}
}

func TestUnknownCommandDoesNotRequireValidConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(config, []byte("not valid toml ="), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_SWITCH_CONFIG", config)

	err := run(context.Background(), []string{"bogus"})
	if err == nil || err.Error() != `unknown command "bogus"; run codex-switch help` {
		t.Fatalf("unexpected unknown command error: %v", err)
	}
}

func TestAccountListSubcommandUsesConfiguredAccounts(t *testing.T) {
	home := t.TempDir()
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: filepath.Join(home, ".codex")},
		{Name: "work", Home: filepath.Join(home, ".codex-work")},
	}

	provider := CodexProvider{}
	out := captureStdout(t, func() {
		if err := cmdAccount(context.Background(), cfg, provider, []string{"list"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "default") || !strings.Contains(out, "work") {
		t.Fatalf("expected account list output to include configured accounts, got %q", out)
	}
}

func TestCurrentHumanOutputIncludesStatusAndHome(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	provider := &fakeProvider{status: &AccountStatus{
		Account: defaultAccount,
		Auth:    AuthInfo{LoggedIn: true, Email: "default@example.com"},
	}}

	out := captureStdout(t, func() {
		if err := cmdCurrent(context.Background(), cfg, provider, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !provider.statusCalled {
		t.Fatal("expected current to collect selected account status")
	}
	if !strings.Contains(out, "default@example.com") || !strings.Contains(out, "home: "+defaultAccount.Home) {
		t.Fatalf("current output missing status/home: %q", out)
	}
}

func TestCurrentJSONOutputUsesSelectedStatus(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	provider := &fakeProvider{status: &AccountStatus{
		Account: defaultAccount,
		Auth:    AuthInfo{LoggedIn: true, Email: "default@example.com"},
	}}

	out := captureStdout(t, func() {
		if err := cmdCurrent(context.Background(), cfg, provider, []string{"--json"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{`"name": "default"`, `"loggedIn": true`, `"email": "default@example.com"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("current JSON output missing %q:\n%s", want, out)
		}
	}
}

func TestAccountAddRejectsUnsafeName(t *testing.T) {
	cfg := defaultConfig()
	err := cmdAccountAdd(context.Background(), cfg, CodexProvider{}, []string{"../bad"})
	if err == nil || !strings.Contains(err.Error(), "account name must use") {
		t.Fatalf("expected account name validation error, got %v", err)
	}
}

func TestAccountAddRejectsDuplicateHomeBeforeInit(t *testing.T) {
	home := t.TempDir()
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(home, ".codex")}}
	provider := &fakeProvider{}
	err := cmdAccountAdd(context.Background(), cfg, provider, []string{"work", "--home", filepath.Join(home, ".codex")})
	if err == nil || !strings.Contains(err.Error(), "already used by default") {
		t.Fatalf("expected duplicate home error, got %v", err)
	}
	if provider.initCalled {
		t.Fatal("account add should not initialize duplicate account home")
	}
}

func TestAccountAddRejectsExistingNameBeforeInit(t *testing.T) {
	home := t.TempDir()
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "work", Home: filepath.Join(home, ".codex-work")}}
	cfg.Defaults.Account = "work"
	provider := &fakeProvider{}
	err := cmdAccountAdd(context.Background(), cfg, provider, []string{"work", "--home", filepath.Join(home, ".codex-other")})
	if err == nil || err.Error() != "account work already exists" {
		t.Fatalf("expected existing account error, got %v", err)
	}
	if provider.initCalled {
		t.Fatal("account add should not initialize or rewrite an existing account")
	}
}

func TestAccountAddRejectsActiveEnvOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_SWITCH_CONFIG", filepath.Join(root, "config.toml"))
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", filepath.Join(root, ".codex-env-work"))
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}}
	provider := &fakeProvider{}
	err := cmdAccountAdd(context.Background(), cfg, provider, []string{"work", "--home", filepath.Join(root, ".codex-work")})
	if err == nil || !strings.Contains(err.Error(), "CODEX_SWITCH_ACCOUNT_WORK_HOME") {
		t.Fatalf("expected env override error, got %v", err)
	}
	if provider.initCalled {
		t.Fatal("account add should not initialize when an env override shadows the name")
	}
}

func TestAccountAddAcceptsFlagsAfterName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_SWITCH_CONFIG", filepath.Join(root, "config.toml"))
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}}
	provider := &fakeProvider{}

	out := captureStdout(t, func() {
		err := cmdAccountAdd(context.Background(), cfg, provider, []string{
			"work",
			"--home", filepath.Join(root, ".codex-work"),
			"--label", "Work Account",
			"--login",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "added work") {
		t.Fatalf("expected add output, got %q", out)
	}
	if !provider.initCalled {
		t.Fatal("expected account home initialization")
	}
	if provider.initCount != 1 {
		t.Fatalf("account add --login should initialize once, got %d", provider.initCount)
	}
	if provider.launch == nil || !equalStrings(provider.launch.Args, []string{"login"}) {
		t.Fatalf("expected login launch, got %#v", provider.launch)
	}
	if provider.initAccount == nil || provider.launch.Account.Home != provider.initAccount.Home {
		t.Fatalf("expected account add --login to use normalized init/launch home: init=%#v launch=%#v", provider.initAccount, provider.launch)
	}
	account, ok := cfg.Account("work")
	if !ok || account.Label != "Work Account" || !strings.HasSuffix(account.Home, ".codex-work") {
		t.Fatalf("unexpected account after add: %#v ok=%v", account, ok)
	}
}

func TestAccountAddStoresAbsoluteHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_SWITCH_CONFIG", filepath.Join(root, "config.toml"))
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}}
	provider := &fakeProvider{}
	relativeHome := filepath.Join("relative", ".codex-work")
	if err := cmdAccountAdd(context.Background(), cfg, provider, []string{"work", "--home", relativeHome}); err != nil {
		t.Fatal(err)
	}
	account, ok := cfg.Account("work")
	if !ok {
		t.Fatal("expected work account")
	}
	if !filepath.IsAbs(account.Home) {
		t.Fatalf("expected account home to be absolute, got %q", account.Home)
	}
	if !strings.HasSuffix(account.Home, filepath.Join("relative", ".codex-work")) {
		t.Fatalf("unexpected account home: %q", account.Home)
	}
}

func TestAccountAddSaveFailureDoesNotMutateConfigOrLaunch(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config-as-directory")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_SWITCH_CONFIG", configDir)
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}}
	provider := &fakeProvider{}

	err := cmdAccountAdd(context.Background(), cfg, provider, []string{"work", "--login"})
	if err == nil {
		t.Fatal("expected config save error")
	}
	if !provider.initCalled {
		t.Fatal("expected account home initialization before save")
	}
	if provider.launch != nil {
		t.Fatalf("account add --login should not launch after save failure, got %#v", provider.launch)
	}
	if cfg.HasAccount("work") {
		t.Fatalf("failed account add should not mutate configured accounts: %#v", cfg.Accounts)
	}
}

func TestInitAccountExistingNameInitializesMappedHome(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	out := captureStdout(t, func() {
		if err := cmdInitAccount(context.Background(), cfg, provider, []string{"work"}); err != nil {
			t.Fatal(err)
		}
	})
	if !provider.initCalled || provider.initAccount == nil || provider.initAccount.Name != "work" {
		t.Fatalf("expected init-account to initialize existing work account, provider=%#v", provider)
	}
	if !strings.Contains(out, "initialized work") {
		t.Fatalf("expected initialized output, got %q", out)
	}
}

func TestInitAccountUsesUnconfiguredEnvOverride(t *testing.T) {
	root := t.TempDir()
	envHome := filepath.Join(root, ".codex-env-work")
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", envHome)
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}}
	provider := &fakeProvider{}
	out := captureStdout(t, func() {
		if err := cmdInitAccount(context.Background(), cfg, provider, []string{"work"}); err != nil {
			t.Fatal(err)
		}
	})
	if provider.initAccount == nil || provider.initAccount.Home != cleanPath(envHome) {
		t.Fatalf("expected init-account to initialize env override home, got %#v", provider.initAccount)
	}
	if !strings.Contains(out, "initialized work") {
		t.Fatalf("expected initialized output, got %q", out)
	}
	if cfg.HasAccount("work") {
		t.Fatal("init-account should not persist an env-only account")
	}
}

func TestAccountsRejectsRuntimeDuplicateHomes(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", defaultAccount.Home)
	provider := &fakeProvider{}
	err := cmdAccounts(context.Background(), cfg, provider, nil)
	if err == nil || err.Error() != "account home "+defaultAccount.Home+" already used by default and work" {
		t.Fatalf("expected runtime duplicate home error, got %v", err)
	}
	if provider.statusCalled {
		t.Fatal("accounts should not collect status after duplicate runtime home")
	}
}

func TestQuotaRejectsRuntimeDuplicateHomeForSingleAccount(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", defaultAccount.Home)
	provider := &fakeProvider{}
	err := cmdQuota(context.Background(), cfg, provider, []string{"work"})
	if err == nil || err.Error() != "account home "+defaultAccount.Home+" already used by default" {
		t.Fatalf("expected runtime duplicate home error, got %v", err)
	}
	if provider.statusCalled {
		t.Fatal("quota should not collect status after duplicate runtime home")
	}
}

func TestCurrentRejectsRuntimeDuplicateHomes(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", defaultAccount.Home)
	provider := &fakeProvider{}
	err := cmdCurrent(context.Background(), cfg, provider, nil)
	if err == nil || err.Error() != "account home "+defaultAccount.Home+" already used by default and work" {
		t.Fatalf("expected runtime duplicate home error, got %v", err)
	}
	if provider.statusCalled {
		t.Fatal("current should not collect status after duplicate runtime home")
	}
}

func TestQuotaAcceptsFlagsAfterAccount(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	out := captureStdout(t, func() {
		if err := cmdQuota(context.Background(), cfg, provider, []string{"work", "--json", "--refresh"}); err != nil {
			t.Fatal(err)
		}
	})
	if !provider.statusCalled {
		t.Fatal("expected quota to fetch account status")
	}
	if !strings.Contains(out, `"name": "work"`) {
		t.Fatalf("expected JSON quota output for work, got %q", out)
	}
}

func TestQuotaHumanOutputBranches(t *testing.T) {
	cfg := providerTestConfig(t)
	plan := "pro"
	provider := &fakeProvider{statuses: []AccountStatus{
		{Account: Account{Name: "default", Home: "/tmp/default"}},
		{Account: Account{Name: "stale", Home: "/tmp/stale"}, Auth: AuthInfo{LoggedIn: true, Email: "stale@example.com"}, QuotaError: "login refresh needed"},
		{Account: Account{Name: "empty", Home: "/tmp/empty"}, Auth: AuthInfo{LoggedIn: true, Email: "empty@example.com"}},
		{
			Account: Account{Name: "work", Home: "/tmp/work"},
			Auth:    AuthInfo{LoggedIn: true, Email: "work@example.com"},
			Quota: &LiveRateLimitResponse{RateLimits: LiveRateLimit{
				PlanType:  &plan,
				Primary:   &LiveQuotaWindow{UsedPercent: 10, WindowDurationMins: 300},
				Secondary: &LiveQuotaWindow{UsedPercent: 20, WindowDurationMins: 10080},
			}},
		},
	}}

	out := captureStdout(t, func() {
		if err := cmdQuota(context.Background(), cfg, provider, []string{"--all"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{
		"default        not logged in",
		"stale          quota unavailable: login refresh needed",
		"empty          quota unavailable",
		"work           pro | 7d 80% left/20% used | 5h 90% left/10% used",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("quota output missing %q:\n%s", want, out)
		}
	}
}

func TestQuotaNamedHumanOutputUsesProviderStatus(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{status: &AccountStatus{
		Account: Account{Name: "work", Home: "/tmp/work"},
		Auth:    AuthInfo{LoggedIn: true, Email: "work@example.com"},
		Quota: &LiveRateLimitResponse{RateLimits: LiveRateLimit{
			Primary: &LiveQuotaWindow{UsedPercent: 15, WindowDurationMins: 90},
		}},
	}}

	out := captureStdout(t, func() {
		if err := cmdQuota(context.Background(), cfg, provider, []string{"work"}); err != nil {
			t.Fatal(err)
		}
	})
	if !provider.statusCalled {
		t.Fatal("expected named quota to use provider status")
	}
	if !strings.Contains(out, "work           90m 85% left/15% used") {
		t.Fatalf("unexpected named quota output: %q", out)
	}
}

func TestParseQuotaRejectsAllWithAccount(t *testing.T) {
	err := cmdQuota(context.Background(), providerTestConfig(t), &fakeProvider{}, []string{"work", "--all"})
	if err == nil || err.Error() != "usage: codex-switch quota [NAME|--all] [--json] [--refresh]" {
		t.Fatalf("expected quota usage error, got %v", err)
	}
}

func TestReadOnlyCommandsRejectUnexpectedArgs(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	cases := []struct {
		name string
		fn   func() error
	}{
		{name: "current", fn: func() error { return cmdCurrent(context.Background(), cfg, provider, []string{"extra"}) }},
		{name: "accounts", fn: func() error { return cmdAccounts(context.Background(), cfg, provider, []string{"extra"}) }},
		{name: "quota all", fn: func() error { return cmdQuota(context.Background(), cfg, provider, []string{"--all", "extra"}) }},
		{name: "quota many", fn: func() error { return cmdQuota(context.Background(), cfg, provider, []string{"default", "extra"}) }},
		{name: "list", fn: func() error { return cmdList(context.Background(), cfg, provider, []string{"extra"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil || !strings.Contains(err.Error(), "usage: codex-switch") {
				t.Fatalf("expected usage error, got %v", err)
			}
		})
	}
}

func TestListUsageDocumentsSessionsFlag(t *testing.T) {
	cfg := providerTestConfig(t)
	err := cmdList(context.Background(), cfg, &fakeProvider{}, []string{"extra"})
	want := "usage: codex-switch list [--cwd DIR] [--limit N] [--json] [--sessions DIR]"
	if err == nil || err.Error() != want {
		t.Fatalf("unexpected list usage: got %v want %q", err, want)
	}
}

func TestListRejectsNonPositiveLimit(t *testing.T) {
	cfg := providerTestConfig(t)
	err := cmdList(context.Background(), cfg, &fakeProvider{}, []string{"--limit", "0"})
	if err == nil || err.Error() != "limit must be greater than zero" {
		t.Fatalf("expected limit validation error, got %v", err)
	}
}

func TestUsageErrorsStayAlignedWithHelp(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "account root",
			err:  cmdAccount(context.Background(), cfg, provider, nil),
			want: accountUsage,
		},
		{
			name: "account add",
			err:  cmdAccountAdd(context.Background(), cfg, provider, nil),
			want: "usage: codex-switch account add NAME [--home PATH] [--label TEXT] [--login]",
		},
		{
			name: "login",
			err:  cmdLogin(context.Background(), cfg, provider, []string{"--print", "--bad"}),
			want: "usage: codex-switch login [NAME|--account NAME] [--print]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil || tc.err.Error() != tc.want {
				t.Fatalf("unexpected usage error: got %v want %q", tc.err, tc.want)
			}
		})
	}
}

func TestRunRejectsUnsafeAccountName(t *testing.T) {
	cfg := defaultConfig()
	err := cmdRunCodex(context.Background(), cfg, &fakeProvider{}, []string{"../bad", "--print"})
	if err == nil || !strings.Contains(err.Error(), "account name must use") {
		t.Fatalf("expected unsafe account name validation error, got %v", err)
	}
}

func TestNewRejectsUnsafeAccountFlag(t *testing.T) {
	cfg := defaultConfig()
	err := cmdNew(context.Background(), cfg, &fakeProvider{}, []string{"--account=../bad", "--print"})
	if err == nil || !strings.Contains(err.Error(), "account name must use") {
		t.Fatalf("expected unsafe account name validation error, got %v", err)
	}
}

func TestConsumeLeadingConfiguredAccountOnlyConsumesConfiguredPositionalAccount(t *testing.T) {
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: "~/.codex"}, {Name: "work", Home: "~/.codex-work"}}
	account, rest, consumed := consumeLeadingConfiguredAccount([]string{"work", "--print"}, "default", cfg)
	if account != "work" || !consumed || len(rest) != 1 || rest[0] != "--print" {
		t.Fatalf("unexpected configured account parse: account=%q rest=%#v", account, rest)
	}

	account, rest, consumed = consumeLeadingConfiguredAccount([]string{"prompt text"}, "default", cfg)
	if account != "default" || consumed || len(rest) != 1 || rest[0] != "prompt text" {
		t.Fatalf("unexpected unknown positional parse: account=%q rest=%#v", account, rest)
	}

	account, rest, consumed = consumeLeadingConfiguredAccount([]string{"singleword"}, "default", cfg)
	if account != "default" || consumed || len(rest) != 1 || rest[0] != "singleword" {
		t.Fatalf("unexpected single-word passthrough parse: account=%q rest=%#v", account, rest)
	}

	account, rest, consumed = consumeLeadingConfiguredAccount([]string{"--account", "scratch", "--print"}, "default", cfg)
	if account != "default" || consumed || len(rest) != 3 || rest[0] != "--account" {
		t.Fatalf("unexpected explicit account passthrough: account=%q rest=%#v", account, rest)
	}
}

func TestConsumeLeadingRunAccountConsumesAnyValidPositionalAccount(t *testing.T) {
	account, rest, consumed, err := consumeLeadingRunAccount([]string{"scratch", "--print"}, "default")
	if err != nil {
		t.Fatal(err)
	}
	if account != "scratch" || !consumed || len(rest) != 1 || rest[0] != "--print" {
		t.Fatalf("unexpected run account parse: account=%q consumed=%v rest=%#v", account, consumed, rest)
	}
	if _, _, _, err := consumeLeadingRunAccount([]string{"../bad"}, "default"); err == nil {
		t.Fatal("expected invalid run account to be rejected")
	}
}

func TestResumeSessionIDIsNotConsumedAsUnconfiguredAccount(t *testing.T) {
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: "~/.codex"}}
	sessionID := "019d30aa-4798-7891-a56f-1f87a629e02c"
	account, rest, consumed := consumeLeadingConfiguredAccount([]string{sessionID, "--print"}, "default", cfg)
	if account != "default" || consumed || len(rest) != 2 || rest[0] != sessionID {
		t.Fatalf("session id should pass through, account=%q rest=%#v", account, rest)
	}
}

func TestLaunchCommandsRejectMixedPositionalAndFlagAccount(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "new",
			err:  cmdNew(context.Background(), cfg, provider, []string{"work", "--account", "default", "--print"}),
			want: "usage: codex-switch new [NAME|--account NAME] [--preset NAME] [--cwd DIR] [--print] -- [codex args...]",
		},
		{
			name: "resume",
			err:  cmdResume(context.Background(), cfg, provider, []string{"work", "--account", "default", "--print"}),
			want: "usage: codex-switch resume [NAME|--account NAME] [--last|--all|--session ID] [--cwd DIR] [--print] -- [codex args...]",
		},
		{
			name: "run",
			err:  cmdRunCodex(context.Background(), cfg, provider, []string{"work", "--account", "default", "--print"}),
			want: "usage: codex-switch run [NAME|--account NAME] [--print] -- [codex args...]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil || tc.err.Error() != tc.want {
				t.Fatalf("unexpected error: got %v want %q", tc.err, tc.want)
			}
		})
	}
	if provider.launch != nil {
		t.Fatalf("ambiguous launch commands should not launch Codex, got %#v", provider.launch)
	}
}

func TestConsumePrintBeforeDoubleDashPreservesCodexPrintArg(t *testing.T) {
	found, rest := consumeBoolFlagBeforeDoubleDash([]string{"--print", "--", "--print"}, "--print")
	if !found {
		t.Fatal("expected wrapper print flag")
	}
	want := []string{"--", "--print"}
	if !equalStrings(rest, want) {
		t.Fatalf("unexpected rest: got %#v want %#v", rest, want)
	}
}

func TestCmdSkillInstallUsesCodeHomeOverride(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	codexHome := filepath.Join(t.TempDir(), ".codex-skill")
	t.Setenv("CODEX_HOME", codexHome)

	out := captureStdout(t, func() {
		if err := cmdSkill(context.Background(), cfg, provider, []string{"install"}); err != nil {
			t.Fatal(err)
		}
	})
	skillPath := filepath.Join(codexHome, "skills", "codex-switch", "SKILL.md")
	if !strings.Contains(out, skillPath) {
		t.Fatalf("expected install output to include %q, got %q", skillPath, out)
	}
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "codex-switch new --account NAME") {
		t.Fatalf("installed skill is missing account guidance:\n%s", string(data))
	}
}

func TestCmdSkillRejectsUnknownArgs(t *testing.T) {
	err := cmdSkill(context.Background(), providerTestConfig(t), &fakeProvider{}, []string{"install", "extra"})
	if err == nil || err.Error() != "usage: codex-switch skill install" {
		t.Fatalf("expected skill usage error, got %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	fn()
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, read); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
