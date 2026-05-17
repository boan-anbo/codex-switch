package main

import "context"

type Provider interface {
	Name() string
	CurrentAccount(cfg *Config) Account
	AuthInfo(account Account) AuthInfo
	InitAccountHome(cfg *Config, account Account) error
	Statuses(ctx context.Context, cfg *Config, refresh bool) []AccountStatus
	Status(ctx context.Context, cfg *Config, account Account, refresh bool) AccountStatus
	NewArgs(cfg *Config, presetName string, cwd string, passthrough []string) ([]string, error)
	ResumeArgs(cfg *Config, presetName string, cwd string, session string, last bool, all bool, passthrough []string) ([]string, error)
	Launch(opts LaunchOptions, cfg *Config) error
	DefaultSessionsDir() string
	CollectSessions(root string, limit int, cwdFilter string) ([]SessionSummary, error)
	SharedAssetState(cfg *Config, account Account, item string) string
}

type CodexProvider struct{}

func (CodexProvider) Name() string {
	return "codex"
}

func (CodexProvider) CurrentAccount(cfg *Config) Account {
	return CurrentAccount(cfg)
}

func (CodexProvider) AuthInfo(account Account) AuthInfo {
	return ReadAuthInfo(account.Home)
}

func (CodexProvider) InitAccountHome(cfg *Config, account Account) error {
	return InitAccountHome(cfg, account)
}

func (CodexProvider) Statuses(ctx context.Context, cfg *Config, refresh bool) []AccountStatus {
	return CollectStatuses(ctx, cfg, refresh)
}

func (CodexProvider) Status(ctx context.Context, cfg *Config, account Account, refresh bool) AccountStatus {
	status := AccountStatus{Account: account, Auth: ReadAuthInfo(account.Home)}
	if status.Auth.LoggedIn {
		if quota, err := GetQuota(ctx, cfg, account, refresh); err == nil {
			status.Quota = quota
		} else {
			status.QuotaError = friendlyQuotaError(err)
		}
	}
	return status
}

func (CodexProvider) NewArgs(cfg *Config, presetName string, cwd string, passthrough []string) ([]string, error) {
	return CodexArgsNew(cfg, presetName, cwd, passthrough)
}

func (CodexProvider) ResumeArgs(cfg *Config, presetName string, cwd string, session string, last bool, all bool, passthrough []string) ([]string, error) {
	return CodexArgsResume(cfg, presetName, cwd, session, last, all, passthrough)
}

func (CodexProvider) Launch(opts LaunchOptions, cfg *Config) error {
	return LaunchCodex(opts, cfg)
}

func (CodexProvider) DefaultSessionsDir() string {
	return defaultSessionsDir()
}

func (CodexProvider) CollectSessions(root string, limit int, cwdFilter string) ([]SessionSummary, error) {
	return collectSessions(root, limit, cwdFilter)
}

func (CodexProvider) SharedAssetState(cfg *Config, account Account, item string) string {
	return sharedAssetState(cfg, account, item)
}
