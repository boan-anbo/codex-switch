package main

type Account struct {
	Name  string `json:"name" toml:"name"`
	Home  string `json:"home" toml:"home"`
	Label string `json:"label,omitempty" toml:"label,omitempty"`
}

type Config struct {
	Version  int             `toml:"version"`
	Codex    CodexConfig     `toml:"codex"`
	UI       UIConfig        `toml:"ui"`
	Defaults DefaultsConfig  `toml:"defaults"`
	Accounts []AccountConfig `toml:"accounts"`
	Presets  []PresetConfig  `toml:"presets"`
}

type CodexConfig struct {
	Bin string `toml:"bin"`
}

type UIConfig struct {
	QuotaTTLSeconds int `toml:"quota_ttl_seconds"`
	SessionLimit    int `toml:"session_limit"`
}

type DefaultsConfig struct {
	Account       string   `toml:"account"`
	Preset        string   `toml:"preset"`
	ShareFromHome string   `toml:"share_from_home"`
	Share         []string `toml:"share"`
}

type AccountConfig struct {
	Name  string `toml:"name"`
	Home  string `toml:"home"`
	Label string `toml:"label,omitempty"`
}

type PresetConfig struct {
	Name string   `toml:"name"`
	Args []string `toml:"args"`
}

type AuthInfo struct {
	LoggedIn bool   `json:"loggedIn"`
	Email    string `json:"email,omitempty"`
	Plan     string `json:"plan,omitempty"`
}

type AccountStatus struct {
	Account    Account                `json:"account"`
	Auth       AuthInfo               `json:"auth"`
	Quota      *LiveRateLimitResponse `json:"quota,omitempty"`
	QuotaError string                 `json:"quotaError,omitempty"`
}

type LiveRateLimitResponse struct {
	RateLimits          LiveRateLimit            `json:"rateLimits"`
	RateLimitsByLimitID map[string]LiveRateLimit `json:"rateLimitsByLimitId"`
	FetchedAtUnix       int64                    `json:"fetchedAtUnix,omitempty"`
}

type LiveRateLimit struct {
	LimitID              *string          `json:"limitId,omitempty"`
	LimitName            *string          `json:"limitName,omitempty"`
	Primary              *LiveQuotaWindow `json:"primary,omitempty"`
	Secondary            *LiveQuotaWindow `json:"secondary,omitempty"`
	Credits              any              `json:"credits,omitempty"`
	PlanType             *string          `json:"planType,omitempty"`
	RateLimitReachedType *FlexibleString  `json:"rateLimitReachedType,omitempty"`
}

type LiveQuotaWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins int     `json:"windowDurationMins"`
	ResetsAt           int64   `json:"resetsAt"`
}

type SessionSummary struct {
	ID        string         `json:"id"`
	Path      string         `json:"path"`
	CWD       string         `json:"cwd"`
	UpdatedAt string         `json:"updatedAt"`
	Timestamp string         `json:"timestamp,omitempty"`
	Source    string         `json:"source,omitempty"`
	Model     string         `json:"model,omitempty"`
	Quota     *RateLimits    `json:"quota,omitempty"`
	Messages  []MessageBrief `json:"messages"`
}

type MessageBrief struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type RateLimits struct {
	LimitID   string       `json:"limit_id"`
	LimitName *string      `json:"limit_name"`
	Primary   *QuotaWindow `json:"primary"`
	Secondary *QuotaWindow `json:"secondary"`
	Credits   any          `json:"credits"`
	PlanType  string       `json:"plan_type"`
}

type QuotaWindow struct {
	UsedPercent  float64 `json:"used_percent"`
	WindowMin    int     `json:"window_minutes"`
	ResetsAtUnix int64   `json:"resets_at"`
}
