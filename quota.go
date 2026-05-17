package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errMissingAccessToken = errors.New("missing access token")

var (
	usageURL   = "https://chatgpt.com/backend-api/wham/usage"
	httpClient = http.Client{Timeout: 15 * time.Second}
)

type quotaHTTPError struct {
	StatusCode int
	Status     string
}

func (err quotaHTTPError) Error() string {
	return "usage request failed: " + err.Status
}

type whamUsageResponse struct {
	Email                string                 `json:"email"`
	PlanType             string                 `json:"plan_type"`
	RateLimit            whamRateLimitContainer `json:"rate_limit"`
	AdditionalRateLimits []whamAdditionalLimit  `json:"additional_rate_limits"`
	RateLimitReachedType *FlexibleString        `json:"rate_limit_reached_type"`
}

type whamAdditionalLimit struct {
	LimitName      string                 `json:"limit_name"`
	MeteredFeature string                 `json:"metered_feature"`
	RateLimit      whamRateLimitContainer `json:"rate_limit"`
}

type whamRateLimitContainer struct {
	PrimaryWindow   whamWindow `json:"primary_window"`
	SecondaryWindow whamWindow `json:"secondary_window"`
}

type whamWindow struct {
	UsedPercent       float64 `json:"used_percent"`
	LimitWindowSecs   int     `json:"limit_window_seconds"`
	ResetAfterSeconds int     `json:"reset_after_seconds"`
	ResetAt           int64   `json:"reset_at"`
}

type FlexibleString struct {
	Value string
}

func (value *FlexibleString) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		value.Value = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		value.Value = text
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err == nil {
		for _, key := range []string{"type", "reason", "value", "name"} {
			if text := str(object[key], ""); text != "" {
				value.Value = text
				return nil
			}
		}
		value.Value = compactJSON(data)
		return nil
	}
	value.Value = compactJSON(data)
	return nil
}

func (value FlexibleString) MarshalJSON() ([]byte, error) {
	if value.Value == "" {
		return []byte("null"), nil
	}
	return json.Marshal(value.Value)
}

func CollectStatuses(ctx context.Context, cfg *Config, refresh bool) []AccountStatus {
	accounts := cfg.AccountsList()
	statuses := make([]AccountStatus, len(accounts))
	var wg sync.WaitGroup
	for i, account := range accounts {
		wg.Add(1)
		go func(i int, account Account) {
			defer wg.Done()
			auth := ReadAuthInfo(account.Home)
			status := AccountStatus{Account: account, Auth: auth}
			if auth.LoggedIn {
				quota, err := GetQuota(ctx, cfg, account, refresh)
				if err != nil {
					status.QuotaError = friendlyQuotaError(err)
				} else {
					status.Quota = quota
				}
			}
			statuses[i] = status
		}(i, account)
	}
	wg.Wait()
	return statuses
}

func GetQuota(ctx context.Context, cfg *Config, account Account, refresh bool) (*LiveRateLimitResponse, error) {
	if !refresh {
		if cached, ok := readQuotaCache(cfg, account); ok {
			return cached, nil
		}
	}
	live, err := readLiveRateLimits(ctx, account)
	if err != nil {
		return nil, err
	}
	_ = writeQuotaCache(account, live)
	return live, nil
}

func readLiveRateLimits(ctx context.Context, account Account) (*LiveRateLimitResponse, error) {
	token, err := readAccessToken(account.Home)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", appName+"/"+version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, quotaHTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	var wham whamUsageResponse
	if err := json.Unmarshal(body, &wham); err != nil {
		return nil, err
	}
	return whamToLiveRateLimits(wham), nil
}

func friendlyQuotaError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errMissingAccessToken) {
		return "not logged in or unsupported auth file"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "not logged in or unsupported auth file"
	}
	if os.IsPermission(err) {
		return "could not read auth file"
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "could not read auth file"
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return "not logged in or unsupported auth file"
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return "not logged in or unsupported auth file"
	}
	var httpErr quotaHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden {
			return "login refresh needed"
		}
	}
	return err.Error()
}

func whamToLiveRateLimits(usage whamUsageResponse) *LiveRateLimitResponse {
	plan := usage.PlanType
	limitID := "codex"
	primary := whamWindowToLive(usage.RateLimit.PrimaryWindow)
	secondary := whamWindowToLive(usage.RateLimit.SecondaryWindow)
	root := LiveRateLimit{
		LimitID:              &limitID,
		Primary:              &primary,
		Secondary:            &secondary,
		PlanType:             &plan,
		RateLimitReachedType: usage.RateLimitReachedType,
	}
	byID := map[string]LiveRateLimit{"codex": root}
	for _, additional := range usage.AdditionalRateLimits {
		id := additional.MeteredFeature
		name := additional.LimitName
		additionalPrimary := whamWindowToLive(additional.RateLimit.PrimaryWindow)
		additionalSecondary := whamWindowToLive(additional.RateLimit.SecondaryWindow)
		byID[id] = LiveRateLimit{
			LimitID:   &id,
			LimitName: &name,
			Primary:   &additionalPrimary,
			Secondary: &additionalSecondary,
			PlanType:  &plan,
		}
	}
	return &LiveRateLimitResponse{
		RateLimits:          root,
		RateLimitsByLimitID: byID,
		FetchedAtUnix:       time.Now().Unix(),
	}
}

func whamWindowToLive(window whamWindow) LiveQuotaWindow {
	resetsAt := window.ResetAt
	if resetsAt == 0 && window.ResetAfterSeconds > 0 {
		resetsAt = time.Now().Add(time.Duration(window.ResetAfterSeconds) * time.Second).Unix()
	}
	return LiveQuotaWindow{
		UsedPercent:        window.UsedPercent,
		WindowDurationMins: window.LimitWindowSecs / 60,
		ResetsAt:           resetsAt,
	}
}

func readQuotaCache(cfg *Config, account Account) (*LiveRateLimitResponse, bool) {
	path, err := quotaCachePath(account)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > cfg.QuotaTTL() {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cached LiveRateLimitResponse
	if json.Unmarshal(data, &cached) != nil {
		return nil, false
	}
	return &cached, true
}

func writeQuotaCache(account Account, quota *LiveRateLimitResponse) error {
	path, err := quotaCachePath(account)
	if err != nil {
		return err
	}
	data, err := json.Marshal(quota)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o600)
}

func quotaCachePath(account Account) (string, error) {
	if !validAccountName(account.Name) {
		return "", errors.New("invalid account name")
	}
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	homeHash := sha256.Sum256([]byte(cleanPath(account.Home)))
	filename := fmt.Sprintf("%s-%x.json", account.Name, homeHash[:8])
	return filepath.Join(dir, "quota", filename), nil
}
