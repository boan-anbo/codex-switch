package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func LoadConfig() (*Config, error) {
	cfg := defaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	normalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func SaveConfig(cfg *Config) error {
	normalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o644)
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	clone.Defaults.Share = append([]string{}, cfg.Defaults.Share...)
	clone.Accounts = append([]AccountConfig{}, cfg.Accounts...)
	clone.Presets = append([]PresetConfig{}, cfg.Presets...)
	for i := range clone.Presets {
		clone.Presets[i].Args = append([]string{}, cfg.Presets[i].Args...)
	}
	return &clone
}

func defaultConfig() *Config {
	home := homeDir()
	cfg := &Config{
		Version: 1,
		Codex:   CodexConfig{Bin: "codex"},
		UI: UIConfig{
			QuotaTTLSeconds: 30,
			SessionLimit:    80,
		},
		Defaults: DefaultsConfig{
			Account:       "default",
			Preset:        "safe",
			ShareFromHome: filepath.Join(home, ".codex"),
			Share:         []string{"config.toml", "skills", "memories", "AGENTS.md", "sessions"},
		},
		Accounts: discoverAccountConfigs(home),
		Presets: []PresetConfig{
			{Name: "safe", Args: nil},
			{Name: "full-access", Args: []string{"--dangerously-bypass-approvals-and-sandbox", "--sandbox", "danger-full-access"}},
		},
	}
	normalizeConfig(cfg)
	return cfg
}

func normalizeConfig(cfg *Config) {
	if cfg.Version <= 0 {
		cfg.Version = 1
	}
	if cfg.Codex.Bin == "" {
		cfg.Codex.Bin = "codex"
	}
	if cfg.UI.QuotaTTLSeconds <= 0 {
		cfg.UI.QuotaTTLSeconds = 30
	}
	if cfg.UI.SessionLimit <= 0 {
		cfg.UI.SessionLimit = 80
	}
	if cfg.Defaults.Account == "" {
		cfg.Defaults.Account = "default"
	}
	if cfg.Defaults.Preset == "" {
		cfg.Defaults.Preset = "safe"
	}
	if cfg.Defaults.ShareFromHome == "" {
		cfg.Defaults.ShareFromHome = filepath.Join(homeDir(), ".codex")
	}
	if len(cfg.Defaults.Share) == 0 {
		cfg.Defaults.Share = []string{"config.toml", "skills", "memories", "AGENTS.md", "sessions"}
	}
	if len(cfg.Accounts) == 0 {
		cfg.Accounts = discoverAccountConfigs(homeDir())
	} else {
		cfg.Accounts = mergeDiscoveredAccountConfigs(cfg.Accounts, homeDir())
	}
	if len(cfg.Presets) == 0 {
		cfg.Presets = []PresetConfig{{Name: "safe"}, {Name: "full-access", Args: []string{"--dangerously-bypass-approvals-and-sandbox", "--sandbox", "danger-full-access"}}}
	}
}

func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	var problems []string
	if cfg.Version != 1 {
		problems = append(problems, fmt.Sprintf("unsupported config version %d", cfg.Version))
	}
	if strings.TrimSpace(cfg.Codex.Bin) == "" {
		problems = append(problems, "codex.bin is required")
	}
	if cfg.UI.QuotaTTLSeconds <= 0 {
		problems = append(problems, "ui.quota_ttl_seconds must be greater than zero")
	}
	if cfg.UI.SessionLimit <= 0 {
		problems = append(problems, "ui.session_limit must be greater than zero")
	}
	if strings.TrimSpace(cfg.Defaults.Account) == "" {
		problems = append(problems, "defaults.account is required")
	}
	if strings.TrimSpace(cfg.Defaults.Preset) == "" {
		problems = append(problems, "defaults.preset is required")
	}
	if strings.TrimSpace(cfg.Defaults.ShareFromHome) == "" {
		problems = append(problems, "defaults.share_from_home is required")
	} else if !filepath.IsAbs(expandHome(cfg.Defaults.ShareFromHome)) {
		problems = append(problems, "defaults.share_from_home must be absolute or start with ~")
	}
	for i, item := range cfg.Defaults.Share {
		if err := validateSharedAssetName(item); err != nil {
			problems = append(problems, fmt.Sprintf("defaults.share[%d] %s", i, err.Error()))
		}
	}

	accountNames := map[string]bool{}
	type namedHome struct {
		name string
		home string
	}
	var accountHomes []namedHome
	for i, account := range cfg.Accounts {
		prefix := fmt.Sprintf("accounts[%d]", i)
		if !validAccountName(account.Name) {
			problems = append(problems, prefix+".name "+accountNameRule)
		}
		if accountNames[account.Name] {
			problems = append(problems, "duplicate account name "+account.Name)
		}
		accountNames[account.Name] = true
		if strings.TrimSpace(account.Home) == "" {
			problems = append(problems, prefix+".home is required")
		} else if !filepath.IsAbs(expandHome(account.Home)) {
			problems = append(problems, prefix+".home must be absolute or start with ~")
		} else {
			for _, seen := range accountHomes {
				if samePath(account.Home, seen.home) {
					problems = append(problems, fmt.Sprintf("duplicate account home for %s and %s", seen.name, account.Name))
					break
				}
			}
			accountHomes = append(accountHomes, namedHome{name: account.Name, home: account.Home})
		}
	}
	if !accountNames[cfg.Defaults.Account] {
		problems = append(problems, "defaults.account "+cfg.Defaults.Account+" is not listed in accounts")
	}

	presetNames := map[string]bool{}
	for i, preset := range cfg.Presets {
		prefix := fmt.Sprintf("presets[%d]", i)
		if strings.TrimSpace(preset.Name) == "" {
			problems = append(problems, prefix+".name is required")
			continue
		}
		if presetNames[preset.Name] {
			problems = append(problems, "duplicate preset name "+preset.Name)
		}
		presetNames[preset.Name] = true
	}
	if !presetNames[cfg.Defaults.Preset] {
		problems = append(problems, "defaults.preset "+cfg.Defaults.Preset+" is not listed in presets")
	}

	if len(problems) > 0 {
		return errors.New("invalid config: " + strings.Join(problems, "; "))
	}
	return nil
}

const accountNameRule = "must use letters, numbers, dot, dash, or underscore, must not end with dot, and must not use Windows reserved names"

func validAccountName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.HasSuffix(name, ".") {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return !windowsReservedAccountName(name)
}

func windowsReservedAccountName(name string) bool {
	base := strings.ToLower(name)
	if idx := strings.Index(base, "."); idx >= 0 {
		base = base[:idx]
	}
	switch base {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(base) == 4 {
		prefix := base[:3]
		suffix := base[3]
		if (prefix == "com" || prefix == "lpt") && suffix >= '1' && suffix <= '9' {
			return true
		}
	}
	return false
}

func validateSharedAssetName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("must not be empty")
	}
	if strings.ContainsAny(name, `/\`) || filepath.Base(name) != name || filepath.Clean(name) != name || name == "." || name == ".." {
		return errors.New("must be a single file or directory name")
	}
	lower := strings.ToLower(name)
	if lower == "auth.json" {
		return errors.New("must not share auth.json")
	}
	if lower == "history.jsonl" || lower == ".codex-global-state.json" || lower == "internal_storage.json" {
		return errors.New("must not share Codex state files")
	}
	if strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite-shm") || strings.HasSuffix(lower, ".sqlite-wal") {
		return errors.New("must not share Codex SQLite state files")
	}
	if strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".db-shm") || strings.HasSuffix(lower, ".db-wal") {
		return errors.New("must not share database state files")
	}
	return nil
}

func discoverAccountConfigs(home string) []AccountConfig {
	accounts := []AccountConfig{{Name: "default", Home: filepath.Join(home, ".codex")}}
	accounts = append(accounts, discoverNamedAccountConfigs(home)...)
	return dedupeAccountConfigs(accounts)
}

func discoverNamedAccountConfigs(home string) []AccountConfig {
	var accounts []AccountConfig
	matches, _ := filepath.Glob(filepath.Join(home, ".codex-*"))
	sort.Strings(matches)
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.IsDir() {
			continue
		}
		name := strings.TrimPrefix(filepath.Base(match), ".codex-")
		if !validAccountName(name) {
			continue
		}
		accounts = append(accounts, AccountConfig{Name: name, Home: match})
	}
	return dedupeAccountConfigs(accounts)
}

func mergeDiscoveredAccountConfigs(accounts []AccountConfig, home string) []AccountConfig {
	out := append([]AccountConfig{}, accounts...)
	for _, discovered := range discoverNamedAccountConfigs(home) {
		exists := false
		for _, account := range out {
			if account.Name == discovered.Name || samePath(account.Home, discovered.Home) {
				exists = true
				break
			}
		}
		if !exists {
			out = append(out, discovered)
		}
	}
	return out
}

func dedupeAccountConfigs(accounts []AccountConfig) []AccountConfig {
	seen := map[string]bool{}
	out := make([]AccountConfig, 0, len(accounts))
	for _, account := range accounts {
		if !validAccountName(account.Name) || seen[account.Name] {
			continue
		}
		seen[account.Name] = true
		out = append(out, account)
	}
	return out
}

func (cfg *Config) Account(name string) (Account, bool) {
	normalizeConfig(cfg)
	if name == "" {
		name = cfg.Defaults.Account
	}
	if !validAccountName(name) {
		return Account{}, false
	}
	if home, ok := accountHomeEnvOverride(name); ok {
		return Account{Name: name, Home: home}, true
	}
	for _, item := range cfg.Accounts {
		if item.Name == name {
			return Account{Name: item.Name, Home: cleanPath(item.Home), Label: item.Label}, true
		}
	}
	if name == "default" {
		return Account{Name: "default", Home: filepath.Join(homeDir(), ".codex")}, true
	}
	return Account{Name: name, Home: filepath.Join(homeDir(), ".codex-"+name)}, true
}

func accountHomeEnvName(name string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_")
	return "CODEX_SWITCH_ACCOUNT_" + strings.ToUpper(replacer.Replace(name)) + "_HOME"
}

func accountHomeEnvOverride(name string) (string, bool) {
	if !validAccountName(name) {
		return "", false
	}
	home := os.Getenv(accountHomeEnvName(name))
	if home == "" {
		return "", false
	}
	return cleanPath(home), true
}

func (cfg *Config) HasAccount(name string) bool {
	normalizeConfig(cfg)
	for _, item := range cfg.Accounts {
		if item.Name == name {
			return true
		}
	}
	return false
}

func (cfg *Config) AccountsList() []Account {
	normalizeConfig(cfg)
	out := make([]Account, 0, len(cfg.Accounts))
	for _, item := range cfg.Accounts {
		account, _ := cfg.Account(item.Name)
		out = append(out, account)
	}
	return out
}

func (cfg *Config) UpsertAccount(account Account) {
	normalizeConfig(cfg)
	if !validAccountName(account.Name) {
		return
	}
	for i, item := range cfg.Accounts {
		if item.Name == account.Name {
			cfg.Accounts[i].Home = account.Home
			cfg.Accounts[i].Label = account.Label
			return
		}
	}
	cfg.Accounts = append(cfg.Accounts, AccountConfig(account))
}

func (cfg *Config) PresetArgs(name string) ([]string, error) {
	normalizeConfig(cfg)
	if name == "" {
		name = cfg.Defaults.Preset
	}
	for _, preset := range cfg.Presets {
		if preset.Name == name {
			return append([]string{}, preset.Args...), nil
		}
	}
	return nil, errors.New("unknown preset " + name)
}

func (cfg *Config) QuotaTTL() time.Duration {
	normalizeConfig(cfg)
	return time.Duration(cfg.UI.QuotaTTLSeconds) * time.Second
}

func ConfigPath() (string, error) {
	if path := os.Getenv("CODEX_SWITCH_CONFIG"); path != "" {
		return expandHome(path), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-switch", "config.toml"), nil
}

func CacheDir() (string, error) {
	if path := os.Getenv("CODEX_SWITCH_CACHE"); path != "" {
		return expandHome(path), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-switch"), nil
}
