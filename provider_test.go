package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type fakeProvider struct {
	statusCalled bool
	authCalled   bool
	initCalled   bool
	initCount    int
	initAccount  *Account
	launch       *LaunchOptions
	newArgs      []string
	resumeArgs   []string
	resumeLast   bool
	resumeAll    bool
	status       *AccountStatus
	statuses     []AccountStatus
	sessions     []SessionSummary
}

func (p *fakeProvider) Name() string {
	return "fake"
}

func (p *fakeProvider) CurrentAccount(cfg *Config) Account {
	account, _ := cfg.Account(cfg.Defaults.Account)
	return account
}

func (p *fakeProvider) AuthInfo(account Account) AuthInfo {
	p.authCalled = true
	return AuthInfo{LoggedIn: true, Email: "safe@example.com"}
}

func (p *fakeProvider) InitAccountHome(cfg *Config, account Account) error {
	p.initCalled = true
	p.initCount++
	copied := account
	p.initAccount = &copied
	return nil
}

func (p *fakeProvider) Statuses(ctx context.Context, cfg *Config, refresh bool) []AccountStatus {
	p.statusCalled = true
	if p.statuses != nil {
		return p.statuses
	}
	return []AccountStatus{{Account: Account{Name: "default", Home: "/tmp/default"}}}
}

func (p *fakeProvider) Status(ctx context.Context, cfg *Config, account Account, refresh bool) AccountStatus {
	p.statusCalled = true
	if p.status != nil {
		return *p.status
	}
	return AccountStatus{Account: account, Auth: AuthInfo{LoggedIn: true, Email: "safe@example.com"}}
}

func (p *fakeProvider) NewArgs(cfg *Config, presetName string, cwd string, passthrough []string) ([]string, error) {
	p.newArgs = append([]string{"new", cwd}, passthrough...)
	return p.newArgs, nil
}

func (p *fakeProvider) ResumeArgs(cfg *Config, presetName string, cwd string, session string, last bool, all bool, passthrough []string) ([]string, error) {
	p.resumeLast = last
	p.resumeAll = all
	p.resumeArgs = append([]string{"resume", cwd, session}, passthrough...)
	return p.resumeArgs, nil
}

func (p *fakeProvider) Launch(opts LaunchOptions, cfg *Config) error {
	copied := opts
	copied.Args = append([]string{}, opts.Args...)
	p.launch = &copied
	return nil
}

func (p *fakeProvider) DefaultSessionsDir() string {
	return "/fake/sessions"
}

func (p *fakeProvider) CollectSessions(root string, limit int, cwdFilter string) ([]SessionSummary, error) {
	if root != "/fake/sessions" {
		return nil, errors.New("unexpected sessions root " + root)
	}
	return p.sessions, nil
}

func (p *fakeProvider) SharedAssetState(cfg *Config, account Account, item string) string {
	return "fake-shared"
}

func TestCmdNewUsesProviderForArgsAndLaunch(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdNew(context.Background(), cfg, provider, []string{"work", "--cwd", "/repo", "--", "--model", "gpt-test"}); err != nil {
		t.Fatal(err)
	}
	if provider.launch == nil {
		t.Fatal("expected provider launch")
	}
	if provider.launch.Account.Name != "work" {
		t.Fatalf("expected work account, got %#v", provider.launch.Account)
	}
	if !provider.initCalled {
		t.Fatal("expected account home initialization before launch")
	}
	want := []string{"new", "/repo", "--model", "gpt-test"}
	if !equalStrings(provider.launch.Args, want) {
		t.Fatalf("launch args mismatch: got %#v want %#v", provider.launch.Args, want)
	}
}

func TestCmdRunAcceptsAccountFlagAfterPrint(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdRunCodex(context.Background(), cfg, provider, []string{"--print", "--account", "work", "--", "login"}); err != nil {
		t.Fatal(err)
	}
	if provider.initCalled {
		t.Fatal("print mode should not initialize account home")
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
	if !equalStrings(provider.launch.Args, []string{"login"}) {
		t.Fatalf("run args mismatch: got %#v", provider.launch.Args)
	}
}

func TestCmdRunPreservesAccountArgAfterDoubleDash(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdRunCodex(context.Background(), cfg, provider, []string{"work", "--print", "--", "--account", "provider-side"}); err != nil {
		t.Fatal(err)
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
	if !equalStrings(provider.launch.Args, []string{"--account", "provider-side"}) {
		t.Fatalf("run args mismatch: got %#v", provider.launch.Args)
	}
}

func TestCmdRunRejectsMixedAccountSelectors(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	err := cmdRunCodex(context.Background(), cfg, provider, []string{"work", "--account", "other", "--print", "--", "login"})
	if err == nil || err.Error() != "usage: codex-switch run [NAME|--account NAME] [--print] -- [codex args...]" {
		t.Fatalf("expected usage error for mixed account selectors, got %v", err)
	}
	if provider.launch != nil || provider.initCalled {
		t.Fatalf("run should not launch after ambiguous account selection: launch=%#v init=%v", provider.launch, provider.initCalled)
	}
}

func TestCmdNewPrintDoesNotInitializeAccountHome(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdNew(context.Background(), cfg, provider, []string{"work", "--print"}); err != nil {
		t.Fatal(err)
	}
	if provider.initCalled {
		t.Fatal("print mode should not initialize account home")
	}
	if provider.launch == nil || !provider.launch.Print {
		t.Fatalf("expected print launch, got %#v", provider.launch)
	}
}

func TestCmdNewAcceptsAccountFlagAfterPrint(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdNew(context.Background(), cfg, provider, []string{"--print", "--account", "work", "--", "--model", "gpt-test"}); err != nil {
		t.Fatal(err)
	}
	if provider.initCalled {
		t.Fatal("print mode should not initialize account home")
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
	want := []string{"new", provider.launch.Args[1], "--model", "gpt-test"}
	if !equalStrings(provider.launch.Args, want) {
		t.Fatalf("launch args mismatch: got %#v want %#v", provider.launch.Args, want)
	}
}

func TestCmdNewRejectsMixedAccountSelectors(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	err := cmdNew(context.Background(), cfg, provider, []string{"work", "--account", "other", "--print"})
	if err == nil || err.Error() != "usage: codex-switch new [NAME|--account NAME] [--preset NAME] [--cwd DIR] [--print] -- [codex args...]" {
		t.Fatalf("expected usage error for mixed account selectors, got %v", err)
	}
	if provider.launch != nil || provider.initCalled {
		t.Fatalf("new should not launch after ambiguous account selection: launch=%#v init=%v", provider.launch, provider.initCalled)
	}
}

func TestCmdNewPreservesProviderAccountFlagAfterDoubleDash(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdNew(context.Background(), cfg, provider, []string{"--account", "work", "--print", "--", "--account", "provider-side"}); err != nil {
		t.Fatal(err)
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
	if !contains(provider.launch.Args, "--account") || !contains(provider.launch.Args, "provider-side") {
		t.Fatalf("new should pass provider-side account flag through, got %#v", provider.launch.Args)
	}
}

func TestLaunchRejectsRuntimeDuplicateHome(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", defaultAccount.Home)
	provider := &fakeProvider{}
	err := cmdNew(context.Background(), cfg, provider, []string{"work", "--print"})
	if err == nil || err.Error() != "account home "+defaultAccount.Home+" already used by default" {
		t.Fatalf("expected runtime duplicate home error, got %v", err)
	}
	if provider.launch != nil || provider.initCalled {
		t.Fatalf("new should not launch after duplicate runtime home: launch=%#v init=%v", provider.launch, provider.initCalled)
	}
}

func TestLaunchNormalizesAccountHomeBeforeInitAndLaunch(t *testing.T) {
	root := t.TempDir()
	setTestHome(t, root)
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}}
	cfg.Defaults.Account = "default"
	provider := &fakeProvider{}
	uncleanHome := filepath.Join(root, "nested", "..", ".codex-scratch")
	if err := launchAccountCodex(provider, cfg, LaunchOptions{Account: Account{Name: "scratch", Home: uncleanHome}, Args: []string{"login"}}); err != nil {
		t.Fatal(err)
	}
	want := cleanPath(uncleanHome)
	if provider.initAccount == nil || provider.initAccount.Home != want {
		t.Fatalf("expected normalized init home %q, got %#v", want, provider.initAccount)
	}
	if provider.launch == nil || provider.launch.Account.Home != want {
		t.Fatalf("expected normalized launch home %q, got %#v", want, provider.launch)
	}
}

func TestLaunchRejectsUnconfiguredFallbackHomeCollision(t *testing.T) {
	root := t.TempDir()
	setTestHome(t, root)
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: filepath.Join(root, ".codex")},
		{Name: "mapped", Home: filepath.Join(root, ".codex-scratch")},
	}
	cfg.Defaults.Account = "default"
	provider := &fakeProvider{}
	err := cmdNew(context.Background(), cfg, provider, []string{"--account", "scratch", "--print"})
	if err == nil || err.Error() != "account home "+filepath.Join(root, ".codex-scratch")+" already used by mapped" {
		t.Fatalf("expected fallback home collision error, got %v", err)
	}
	if provider.launch != nil || provider.initCalled {
		t.Fatalf("new should not launch after fallback home collision: launch=%#v init=%v", provider.launch, provider.initCalled)
	}
}

func TestCmdResumeAcceptsAccountFlagAfterPrint(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdResume(context.Background(), cfg, provider, []string{"--print", "--account", "work", "--last"}); err != nil {
		t.Fatal(err)
	}
	if provider.initCalled {
		t.Fatal("print mode should not initialize account home")
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
}

func TestCmdResumePreservesProviderAccountFlagAfterDoubleDash(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdResume(context.Background(), cfg, provider, []string{"--account", "work", "--print", "--last", "--", "--account", "provider-side"}); err != nil {
		t.Fatal(err)
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
	if !contains(provider.launch.Args, "--account") || !contains(provider.launch.Args, "provider-side") {
		t.Fatalf("resume should pass provider-side account flag through, got %#v", provider.launch.Args)
	}
}

func TestCmdResumeRejectsMixedSessionSelectors(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	err := cmdResume(context.Background(), cfg, provider, []string{"--last", "--session", "019d30aa-4798-7891-a56f-1f87a629e02c"})
	if err == nil || err.Error() != "choose only one of --last, --all, or --session" {
		t.Fatalf("expected mixed selector error, got %v", err)
	}
	if provider.launch != nil || provider.initCalled {
		t.Fatalf("resume should not launch after ambiguous session selection: launch=%#v init=%v", provider.launch, provider.initCalled)
	}

	err = cmdResume(context.Background(), cfg, provider, []string{"--all", "019d30aa-4798-7891-a56f-1f87a629e02c"})
	if err == nil || err.Error() != "choose only one of --last, --all, or --session" {
		t.Fatalf("expected mixed selector error for positional session, got %v", err)
	}
}

func TestCmdLoginPrintDoesNotInitializeAccountHome(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdLogin(context.Background(), cfg, provider, []string{"work", "--print"}); err != nil {
		t.Fatal(err)
	}
	if provider.initCalled {
		t.Fatal("print mode should not initialize account home")
	}
	if provider.launch == nil || !provider.launch.Print {
		t.Fatalf("expected print launch, got %#v", provider.launch)
	}
}

func TestCmdLoginPrintAcceptsFlagBeforeAccount(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdLogin(context.Background(), cfg, provider, []string{"--print", "work"}); err != nil {
		t.Fatal(err)
	}
	if provider.initCalled {
		t.Fatal("print mode should not initialize account home")
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
}

func TestCmdLoginAccountFlagIsNotOverriddenByPositionalAccount(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	err := cmdLogin(context.Background(), cfg, provider, []string{"--account", "work", "other", "--print"})
	if err == nil || err.Error() != "usage: codex-switch login [NAME|--account NAME] [--print]" {
		t.Fatalf("expected usage error for mixed account selectors, got %v", err)
	}
	if provider.launch != nil || provider.initCalled {
		t.Fatalf("login should not launch after ambiguous account selection: launch=%#v init=%v", provider.launch, provider.initCalled)
	}
}

func TestCmdLoginAcceptsAccountFlagAfterPrint(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	if err := cmdLogin(context.Background(), cfg, provider, []string{"--print", "--account", "work"}); err != nil {
		t.Fatal(err)
	}
	if provider.launch == nil || provider.launch.Account.Name != "work" || !provider.launch.Print {
		t.Fatalf("expected work print launch, got %#v", provider.launch)
	}
}

func TestCmdDoctorUsesAuthInfoNotQuotaStatus(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{}
	_ = captureStdout(t, func() {
		if err := cmdDoctor(context.Background(), cfg, provider, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !provider.authCalled {
		t.Fatal("expected doctor to call provider AuthInfo")
	}
	if provider.statusCalled {
		t.Fatal("doctor should not call provider Status because that may fetch quota")
	}
}

func TestCmdListUsesProviderSessionSource(t *testing.T) {
	cfg := providerTestConfig(t)
	provider := &fakeProvider{sessions: []SessionSummary{{ID: "abc", CWD: "/repo", UpdatedAt: "now"}}}
	out := captureStdout(t, func() {
		if err := cmdList(context.Background(), cfg, provider, nil); err != nil {
			t.Fatal(err)
		}
	})
	if out == "" || provider.statusCalled {
		t.Fatalf("unexpected list output/status behavior: out=%q statusCalled=%v", out, provider.statusCalled)
	}
}

func TestCodexProviderDelegatesCoreOperations(t *testing.T) {
	root := t.TempDir()
	setTestHome(t, root)
	cfg := defaultConfig()
	workHome := filepath.Join(root, ".codex-work")
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: filepath.Join(root, ".codex")},
		{Name: "work", Home: workHome},
	}
	cfg.Defaults.Account = "default"
	cfg.Defaults.Share = nil
	provider := CodexProvider{}

	t.Setenv("CODEX_HOME", filepath.Join(workHome, "..", ".codex-work"))
	if got := provider.CurrentAccount(cfg); got.Name != "work" || got.Home != cleanPath(workHome) {
		t.Fatalf("unexpected current account: %#v", got)
	}
	if err := provider.InitAccountHome(cfg, Account{Name: "work", Home: workHome}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workHome); err != nil {
		t.Fatal(err)
	}
	newArgs, err := provider.NewArgs(cfg, "full-access", "/repo", []string{"--model", "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(newArgs, "--cd") || !contains(newArgs, "/repo") || !contains(newArgs, "--model") {
		t.Fatalf("unexpected new args: %#v", newArgs)
	}
	resumeArgs, err := provider.ResumeArgs(cfg, "", "/repo", "session-id", false, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(resumeArgs, "--all") || !contains(resumeArgs, "--include-non-interactive") || !contains(resumeArgs, "session-id") {
		t.Fatalf("unexpected resume args: %#v", resumeArgs)
	}

	sessionsDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSession(t, filepath.Join(sessionsDir, "rollout-2026-01-01T00-00-00-019d30aa-4798-7891-a56f-1f87a629e02c.jsonl"), "/repo")
	t.Setenv("CODEX_SESSIONS_DIR", sessionsDir)
	if got := provider.DefaultSessionsDir(); got != sessionsDir {
		t.Fatalf("unexpected sessions dir: %q", got)
	}
	sessions, err := provider.CollectSessions(provider.DefaultSessionsDir(), 10, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].CWD != "/repo" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
}

func TestCodexProviderStatusSanitizesQuotaFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", filepath.Join(root, "cache"))
	home := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"work-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret failure body", http.StatusForbidden)
	}))
	defer server.Close()
	oldURL := usageURL
	oldClient := httpClient
	usageURL = server.URL
	httpClient = *server.Client()
	defer func() {
		usageURL = oldURL
		httpClient = oldClient
	}()

	status := CodexProvider{}.Status(context.Background(), defaultConfig(), Account{Name: "work", Home: home}, true)
	if !status.Auth.LoggedIn || status.Quota != nil || status.QuotaError != "login refresh needed" {
		t.Fatalf("unexpected provider status: %#v", status)
	}
}

func providerTestConfig(t *testing.T) *Config {
	t.Helper()
	root := t.TempDir()
	setTestHome(t, root)
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: filepath.Join(root, ".codex")},
		{Name: "work", Home: filepath.Join(root, ".codex-work")},
	}
	cfg.Defaults.Account = "default"
	return cfg
}
