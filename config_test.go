package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultConfigDiscoversCodexHomes(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Setenv("CODEX_SWITCH_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".codex-work"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	var names []string
	for _, account := range cfg.Accounts {
		names = append(names, account.Name)
	}
	if !contains(names, "default") || !contains(names, "work") {
		t.Fatalf("expected default and work accounts, got %#v", names)
	}
}

func TestDefaultConfigSkipsInvalidDiscoveredCodexHomes(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Setenv("CODEX_SWITCH_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	for _, dir := range []string{".codex", ".codex-work", ".codex-bad name", ".codex-bad@"} {
		if err := os.Mkdir(filepath.Join(home, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	cfg := defaultConfig()
	var names []string
	for _, account := range cfg.Accounts {
		names = append(names, account.Name)
	}
	if !contains(names, "default") || !contains(names, "work") {
		t.Fatalf("expected valid discovered accounts, got %#v", names)
	}
	if contains(names, "bad name") || contains(names, "bad@") {
		t.Fatalf("invalid account names should be skipped, got %#v", names)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("default config should stay valid after discovery: %v", err)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_SWITCH_CONFIG", path)
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "work", Home: "~/custom"}}
	cfg.Defaults.Account = "work"

	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Defaults.Account != "work" {
		t.Fatalf("expected default account work, got %q", loaded.Defaults.Account)
	}
	account, ok := loaded.Account("work")
	if !ok || account.Home != filepath.Join(homeDir(), "custom") {
		t.Fatalf("unexpected account: %#v ok=%v", account, ok)
	}
}

func TestSaveConfigReplacesExistingConfigWithoutTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	t.Setenv("CODEX_SWITCH_CONFIG", path)
	if err := os.WriteFile(path, []byte("old = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: "~/.codex"}, {Name: "work", Home: "~/custom"}}
	cfg.Defaults.Account = "work"

	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "account = 'work'") || strings.Contains(string(data), "old = true") {
		t.Fatalf("config was not replaced cleanly: %s", data)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".config.toml.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("SaveConfig left temporary files: %#v", matches)
	}
}

func TestLoadConfigMergesNewlyDiscoveredCodexHomes(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_SWITCH_CONFIG", path)
	if err := os.Mkdir(filepath.Join(home, ".codex-work"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`
version = 1

[codex]
bin = 'codex'

[ui]
quota_ttl_seconds = 30
session_limit = 80

[defaults]
account = 'default'
preset = 'safe'
share_from_home = '~/.codex'
share = ['config.toml', 'sessions']

[[accounts]]
name = 'default'
home = '~/.codex'

[[presets]]
name = 'safe'
args = []
`), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.HasAccount("work") {
		t.Fatalf("expected discovered work account to be added to configured accounts: %#v", loaded.Accounts)
	}
	account, ok := loaded.Account("work")
	if !ok {
		t.Fatal("expected discovered work account")
	}
	if account.Home != filepath.Join(home, ".codex-work") {
		t.Fatalf("unexpected discovered account home: %#v", account)
	}
}

func TestLoadConfigSkipsDiscoveredHomeAlreadyMappedByConfig(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_SWITCH_CONFIG", path)
	workHome := filepath.Join(home, ".codex-work")
	if err := os.Mkdir(workHome, 0o700); err != nil {
		t.Fatal(err)
	}
	data := strings.ReplaceAll(`
version = 1

[codex]
bin = 'codex'

[ui]
quota_ttl_seconds = 30
session_limit = 80

[defaults]
account = 'default'
preset = 'safe'
share_from_home = '~/.codex'
share = ['config.toml', 'sessions']

[[accounts]]
name = 'default'
home = '~/.codex'

[[accounts]]
name = 'custom'
home = '$WORK_HOME'

[[presets]]
name = 'safe'
args = []
`, "$WORK_HOME", workHome)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HasAccount("work") {
		t.Fatalf("discovered work account should not be added when home is already mapped: %#v", loaded.Accounts)
	}
	if _, ok := loaded.Account("custom"); !ok {
		t.Fatal("expected custom account to remain configured")
	}
}

func TestLoadConfigRejectsDuplicateConfiguredAccountsAfterDiscovery(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CODEX_SWITCH_CONFIG", path)
	if err := os.Mkdir(filepath.Join(home, ".codex-extra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`
version = 1

[codex]
bin = 'codex'

[ui]
quota_ttl_seconds = 30
session_limit = 80

[defaults]
account = 'default'
preset = 'safe'
share_from_home = '~/.codex'
share = ['config.toml', 'sessions']

[[accounts]]
name = 'default'
home = '~/.codex'

[[accounts]]
name = 'work'
home = '~/.codex-work'

[[accounts]]
name = 'work'
home = '~/.codex-work-2'

[[presets]]
name = 'safe'
args = []
`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "duplicate account name work") {
		t.Fatalf("expected duplicate account validation error, got %v", err)
	}
}

func TestDiscoveredAccountsStayIdempotentAcrossRepeatedAccess(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	if err := os.Mkdir(filepath.Join(home, ".codex-work"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()

	for range 3 {
		if !cfg.HasAccount("work") {
			t.Fatal("expected discovered work account")
		}
		_ = cfg.AccountsList()
		if _, ok := cfg.Account("work"); !ok {
			t.Fatal("expected work account lookup")
		}
	}

	count := 0
	for _, account := range cfg.Accounts {
		if account.Name == "work" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one discovered work account after repeated normalization, got %d in %#v", count, cfg.Accounts)
	}
}

func TestValidateConfigRejectsDuplicateAccounts(t *testing.T) {
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: "~/.codex"},
		{Name: "default", Home: "~/.codex-two"},
	}
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate account name default") {
		t.Fatalf("expected duplicate account error, got %v", err)
	}
}

func TestValidateConfigRejectsDuplicateAccountHomes(t *testing.T) {
	home := t.TempDir()
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: filepath.Join(home, ".codex")},
		{Name: "work", Home: filepath.Join(home, ".codex")},
	}
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate account home for default and work") {
		t.Fatalf("expected duplicate account home error, got %v", err)
	}
}

func TestValidateConfigRejectsMissingDefaultAccount(t *testing.T) {
	cfg := defaultConfig()
	cfg.Defaults.Account = "missing"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "defaults.account missing is not listed") {
		t.Fatalf("expected missing default account error, got %v", err)
	}
}

func TestValidateConfigRejectsRelativeAccountHome(t *testing.T) {
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: "relative/.codex"}}
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "accounts[0].home must be absolute or start with ~") {
		t.Fatalf("expected relative account home error, got %v", err)
	}
}

func TestValidateConfigRejectsRelativeShareFromHome(t *testing.T) {
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = "relative/.codex"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "defaults.share_from_home must be absolute or start with ~") {
		t.Fatalf("expected relative share_from_home error, got %v", err)
	}
}

func TestValidateConfigRejectsUnknownDefaultPreset(t *testing.T) {
	cfg := defaultConfig()
	cfg.Defaults.Preset = "unknown"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "defaults.preset unknown is not listed") {
		t.Fatalf("expected unknown preset error, got %v", err)
	}
}

func TestValidateConfigRejectsBadAccountName(t *testing.T) {
	cfg := defaultConfig()
	cfg.Accounts = append(cfg.Accounts, AccountConfig{Name: "../bad", Home: "~/.codex-bad"})
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "accounts[") {
		t.Fatalf("expected invalid account name error, got %v", err)
	}
}

func TestValidAccountNameRejectsPortableFilenameHazards(t *testing.T) {
	invalid := []string{"", ".", "..", "work.", "con", "CON", "con.prod", "prn", "aux", "nul", "com1", "COM9", "lpt1", "LPT9"}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			if validAccountName(name) {
				t.Fatalf("expected %q to be invalid", name)
			}
		})
	}
	valid := []string{"default", "work", "prod.us", "team-1", "team_2", ".hidden"}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			if !validAccountName(name) {
				t.Fatalf("expected %q to be valid", name)
			}
		})
	}
}

func TestAccountRejectsUnsafeName(t *testing.T) {
	cfg := defaultConfig()
	if account, ok := cfg.Account("../bad"); ok {
		t.Fatalf("expected unsafe account name to be rejected, got %#v", account)
	}
}

func TestAccountHomeEnvNameNormalizesSeparators(t *testing.T) {
	got := accountHomeEnvName("work.prod-us")
	if got != "CODEX_SWITCH_ACCOUNT_WORK_PROD_US_HOME" {
		t.Fatalf("unexpected env name: %s", got)
	}
}

func TestAccountHomeEnvOverrideNormalizesHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "nested", "..", ".codex-work")
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", home)
	got, ok := accountHomeEnvOverride("work")
	if !ok {
		t.Fatal("expected env override")
	}
	if got != cleanPath(home) {
		t.Fatalf("expected normalized env home %q, got %q", cleanPath(home), got)
	}
}

func TestSamePathUsesWindowsCaseInsensitivity(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !samePath(`C:\Users\Alice\.codex`, `c:\users\alice\.codex`) {
			t.Fatal("expected Windows paths to compare case-insensitively")
		}
		return
	}
	if samePath("/tmp/Codex", "/tmp/codex") {
		t.Fatal("expected non-Windows paths to compare case-sensitively")
	}
}

func TestSetEnvUsesWindowsCaseInsensitivity(t *testing.T) {
	env := setEnv([]string{"codex_home=C:\\old"}, "CODEX_HOME", "C:\\new")
	if runtime.GOOS == "windows" {
		if len(env) != 1 || env[0] != "CODEX_HOME=C:\\new" {
			t.Fatalf("expected Windows env key replacement, got %#v", env)
		}
		return
	}
	if len(env) != 2 || !contains(env, "codex_home=C:\\old") || !contains(env, "CODEX_HOME=C:\\new") {
		t.Fatalf("expected non-Windows env key append, got %#v", env)
	}
}

func TestCurrentAccountNormalizesCodeHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex-work")
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "default", Home: filepath.Join(root, ".codex")}, {Name: "work", Home: home}}
	t.Setenv("CODEX_HOME", filepath.Join(home, "..", ".codex-work"))

	account := CurrentAccount(cfg)
	if account.Name != "work" || account.Home != cleanPath(home) {
		t.Fatalf("expected normalized work account, got %#v", account)
	}
}

func TestValidateConfigRejectsUnsafeSharedAssets(t *testing.T) {
	tests := []string{
		"auth.json",
		"../auth.json",
		`..\auth.json`,
		`nested\auth.json`,
		"nested/auth.json",
		"state_5.sqlite",
		"state_5.sqlite-wal",
		"logs_2.sqlite-shm",
		"history.jsonl",
		".codex-global-state.json",
		"internal_storage.json",
	}
	for _, share := range tests {
		t.Run(share, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.Defaults.Share = []string{share}
			err := ValidateConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), "defaults.share[0]") {
				t.Fatalf("expected unsafe share error for %q, got %v", share, err)
			}
		})
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}
