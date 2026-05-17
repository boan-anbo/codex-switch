package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFriendlyQuotaErrorMapsUnauthorizedToRefresh(t *testing.T) {
	got := friendlyQuotaError(quotaHTTPError{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"})
	if got != "login refresh needed" {
		t.Fatalf("unexpected quota error: %q", got)
	}
}

func TestFriendlyQuotaErrorMapsMissingToken(t *testing.T) {
	got := friendlyQuotaError(errMissingAccessToken)
	if got != "not logged in or unsupported auth file" {
		t.Fatalf("unexpected quota error: %q", got)
	}
}

func TestFriendlyQuotaErrorSanitizesAuthReadErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "missing",
			err:  &os.PathError{Op: "open", Path: "/secret/.codex/auth.json", Err: os.ErrNotExist},
			want: "not logged in or unsupported auth file",
		},
		{
			name: "permission",
			err:  &os.PathError{Op: "open", Path: "/secret/.codex/auth.json", Err: os.ErrPermission},
			want: "could not read auth file",
		},
		{
			name: "other path error",
			err:  &os.PathError{Op: "read", Path: "/secret/.codex/auth.json", Err: errors.New("is a directory")},
			want: "could not read auth file",
		},
		{
			name: "json syntax",
			err:  &json.SyntaxError{Offset: 1},
			want: "not logged in or unsupported auth file",
		},
		{
			name: "json type",
			err:  &json.UnmarshalTypeError{Value: "array", Type: nil},
			want: "not logged in or unsupported auth file",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := friendlyQuotaError(tc.err)
			if got != tc.want {
				t.Fatalf("unexpected quota error: got %q want %q", got, tc.want)
			}
			if strings.Contains(got, "/secret") || strings.Contains(got, "auth.json") {
				t.Fatalf("quota error leaked auth file detail: %q", got)
			}
		})
	}
}

func TestFriendlyQuotaErrorPreservesUnknownErrors(t *testing.T) {
	got := friendlyQuotaError(errors.New("network down"))
	if got != "network down" {
		t.Fatalf("unexpected quota error: %q", got)
	}
}

func TestFlexibleStringJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "null", data: `null`, want: ""},
		{name: "string", data: `"weekly"`, want: "weekly"},
		{name: "object type", data: `{"type":"weekly","reason":"fallback"}`, want: "weekly"},
		{name: "object compact fallback", data: `{"unknown":true}`, want: `{"unknown":true}`},
		{name: "array compact fallback", data: `[1,2]`, want: `[1,2]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var value FlexibleString
			if err := json.Unmarshal([]byte(tc.data), &value); err != nil {
				t.Fatal(err)
			}
			if value.Value != tc.want {
				t.Fatalf("unexpected value: got %q want %q", value.Value, tc.want)
			}
		})
	}

	empty, err := json.Marshal(FlexibleString{})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "null" {
		t.Fatalf("empty flexible string should marshal to null, got %s", empty)
	}

	text, err := json.Marshal(FlexibleString{Value: "weekly"})
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != `"weekly"` {
		t.Fatalf("flexible string should marshal as JSON string, got %s", text)
	}
}

func TestWhamWindowToLiveUsesResetAfterSecondsFallback(t *testing.T) {
	before := time.Now().Add(59 * time.Second).Unix()
	window := whamWindow{
		UsedPercent:       42,
		LimitWindowSecs:   7 * 24 * 60 * 60,
		ResetAfterSeconds: 60,
	}
	got := whamWindowToLive(window)
	after := time.Now().Add(61 * time.Second).Unix()
	if got.ResetsAt < before || got.ResetsAt > after {
		t.Fatalf("expected reset fallback near one minute from now, got %d want between %d and %d", got.ResetsAt, before, after)
	}
	if got.WindowDurationMins != 7*24*60 {
		t.Fatalf("unexpected duration: %d", got.WindowDurationMins)
	}
}

func TestWhamToLiveRateLimitsMapsRootAndAdditionalLimits(t *testing.T) {
	var usage whamUsageResponse
	data := []byte(`{
		"email": "user@example.com",
		"plan_type": "plus",
		"rate_limit_reached_type": {"type": "weekly"},
		"rate_limit": {
			"primary_window": {"used_percent": 11, "limit_window_seconds": 18000, "reset_at": 1710000100},
			"secondary_window": {"used_percent": 22, "limit_window_seconds": 604800, "reset_at": 1710000200}
		},
		"additional_rate_limits": [{
			"limit_name": "GPT-5",
			"metered_feature": "gpt-5",
			"rate_limit": {
				"primary_window": {"used_percent": 33, "limit_window_seconds": 18000, "reset_at": 1710000300},
				"secondary_window": {"used_percent": 44, "limit_window_seconds": 604800, "reset_at": 1710000400}
			}
		}]
	}`)
	if err := json.Unmarshal(data, &usage); err != nil {
		t.Fatal(err)
	}

	got := whamToLiveRateLimits(usage)
	if got.FetchedAtUnix == 0 {
		t.Fatal("expected fetched timestamp")
	}
	if got.RateLimits.LimitID == nil || *got.RateLimits.LimitID != "codex" {
		t.Fatalf("unexpected root limit id: %#v", got.RateLimits.LimitID)
	}
	if got.RateLimits.PlanType == nil || *got.RateLimits.PlanType != "plus" {
		t.Fatalf("unexpected plan type: %#v", got.RateLimits.PlanType)
	}
	if got.RateLimits.RateLimitReachedType == nil || got.RateLimits.RateLimitReachedType.Value != "weekly" {
		t.Fatalf("unexpected reached type: %#v", got.RateLimits.RateLimitReachedType)
	}
	if got.RateLimits.Primary == nil || got.RateLimits.Primary.UsedPercent != 11 || got.RateLimits.Primary.WindowDurationMins != 300 {
		t.Fatalf("unexpected root primary window: %#v", got.RateLimits.Primary)
	}
	if got.RateLimits.Secondary == nil || got.RateLimits.Secondary.UsedPercent != 22 || got.RateLimits.Secondary.WindowDurationMins != 10080 {
		t.Fatalf("unexpected root secondary window: %#v", got.RateLimits.Secondary)
	}

	additional, ok := got.RateLimitsByLimitID["gpt-5"]
	if !ok {
		t.Fatalf("expected additional gpt-5 limit, got %#v", got.RateLimitsByLimitID)
	}
	if additional.LimitName == nil || *additional.LimitName != "GPT-5" {
		t.Fatalf("unexpected additional limit name: %#v", additional.LimitName)
	}
	if additional.Primary == nil || additional.Primary.UsedPercent != 33 {
		t.Fatalf("unexpected additional primary window: %#v", additional.Primary)
	}
	if additional.Secondary == nil || additional.Secondary.UsedPercent != 44 {
		t.Fatalf("unexpected additional secondary window: %#v", additional.Secondary)
	}
}

func TestReadLiveRateLimitsUsesSelectedAccountTokenAndMapsResponse(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"selected-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing user-agent")
		}
		_, _ = io.WriteString(w, `{
			"plan_type": "plus",
			"rate_limit": {
				"primary_window": {"used_percent": 12, "limit_window_seconds": 18000, "reset_at": 1710000100},
				"secondary_window": {"used_percent": 34, "limit_window_seconds": 604800, "reset_at": 1710000200}
			}
		}`)
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

	got, err := readLiveRateLimits(context.Background(), Account{Name: "work", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer selected-token" {
		t.Fatalf("unexpected authorization header: %q", gotAuth)
	}
	if got.RateLimits.PlanType == nil || *got.RateLimits.PlanType != "plus" {
		t.Fatalf("unexpected plan type: %#v", got.RateLimits.PlanType)
	}
	if got.RateLimits.Primary == nil || got.RateLimits.Primary.UsedPercent != 12 || got.RateLimits.Primary.WindowDurationMins != 300 {
		t.Fatalf("unexpected primary quota: %#v", got.RateLimits.Primary)
	}
	if got.RateLimits.Secondary == nil || got.RateLimits.Secondary.UsedPercent != 34 || got.RateLimits.Secondary.WindowDurationMins != 10080 {
		t.Fatalf("unexpected secondary quota: %#v", got.RateLimits.Secondary)
	}
}

func TestReadLiveRateLimitsReturnsHTTPStatusWithoutBody(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"selected-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret body", http.StatusUnauthorized)
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

	_, err := readLiveRateLimits(context.Background(), Account{Name: "work", Home: home})
	var httpErr quotaHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected quotaHTTPError, got %T %v", err, err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized || httpErr.Status != "401 Unauthorized" {
		t.Fatalf("unexpected http error: %#v", httpErr)
	}
	if strings.Contains(err.Error(), "secret body") {
		t.Fatalf("http error leaked response body: %v", err)
	}
}

func TestGetQuotaUsesFreshCacheWithoutAuthOrNetwork(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", cache)
	cfg := defaultConfig()
	account := Account{Name: "work", Home: filepath.Join(t.TempDir(), ".codex-work")}
	cachedQuota := &LiveRateLimitResponse{RateLimits: LiveRateLimit{
		Primary: &LiveQuotaWindow{UsedPercent: 9, WindowDurationMins: 300},
	}}
	if err := writeQuotaCache(account, cachedQuota); err != nil {
		t.Fatal(err)
	}

	got, err := GetQuota(context.Background(), cfg, account, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.RateLimits.Primary == nil || got.RateLimits.Primary.UsedPercent != 9 {
		t.Fatalf("expected cached quota, got %#v", got.RateLimits.Primary)
	}
}

func TestGetQuotaFetchesLiveThenCachesForNextRead(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", cache)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"selected-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = io.WriteString(w, `{
			"rate_limit": {
				"primary_window": {"used_percent": 42, "limit_window_seconds": 18000, "reset_at": 1710000100}
			}
		}`)
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
	cfg := defaultConfig()
	account := Account{Name: "work", Home: home}

	got, err := GetQuota(context.Background(), cfg, account, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.RateLimits.Primary == nil || got.RateLimits.Primary.UsedPercent != 42 {
		t.Fatalf("expected live quota, got %#v", got.RateLimits.Primary)
	}
	if requests != 1 {
		t.Fatalf("expected one live request, got %d", requests)
	}

	cached, err := GetQuota(context.Background(), cfg, account, false)
	if err != nil {
		t.Fatal(err)
	}
	if cached.RateLimits.Primary == nil || cached.RateLimits.Primary.UsedPercent != 42 {
		t.Fatalf("expected cached live quota, got %#v", cached.RateLimits.Primary)
	}
	if requests != 1 {
		t.Fatalf("expected second read to use cache, got %d live requests", requests)
	}
}

func TestGetQuotaRefreshBypassesCacheAndRewritesIt(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", cache)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"tokens":{"access_token":"selected-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	account := Account{Name: "work", Home: home}
	if err := writeQuotaCache(account, &LiveRateLimitResponse{RateLimits: LiveRateLimit{
		Primary: &LiveQuotaWindow{UsedPercent: 5, WindowDurationMins: 300},
	}}); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = io.WriteString(w, `{
			"rate_limit": {
				"primary_window": {"used_percent": 55, "limit_window_seconds": 18000, "reset_at": 1710000100}
			}
		}`)
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
	cfg := defaultConfig()

	got, err := GetQuota(context.Background(), cfg, account, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.RateLimits.Primary == nil || got.RateLimits.Primary.UsedPercent != 55 {
		t.Fatalf("expected refreshed live quota, got %#v", got.RateLimits.Primary)
	}
	if requests != 1 {
		t.Fatalf("expected refresh to fetch live quota once, got %d", requests)
	}
	cached, ok := readQuotaCache(cfg, account)
	if !ok {
		t.Fatal("expected refresh to rewrite cache")
	}
	if cached.RateLimits.Primary == nil || cached.RateLimits.Primary.UsedPercent != 55 {
		t.Fatalf("expected refreshed quota in cache, got %#v", cached.RateLimits.Primary)
	}
}

func TestCollectStatusesSkipsLoggedOutAccountsAndPreservesOrder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_SWITCH_CACHE", filepath.Join(root, "cache"))
	defaultHome := filepath.Join(root, ".codex")
	workHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(defaultHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workHome, "auth.json"), []byte(`{"tokens":{"access_token":"work-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{
			"rate_limit": {
				"primary_window": {"used_percent": 18, "limit_window_seconds": 18000, "reset_at": 1710000100}
			}
		}`)
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
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{
		{Name: "default", Home: defaultHome},
		{Name: "work", Home: workHome},
	}

	statuses := CollectStatuses(context.Background(), cfg, false)
	if len(statuses) != 2 {
		t.Fatalf("expected two statuses, got %#v", statuses)
	}
	if statuses[0].Account.Name != "default" || statuses[0].Auth.LoggedIn || statuses[0].Quota != nil || statuses[0].QuotaError != "" {
		t.Fatalf("unexpected logged-out default status: %#v", statuses[0])
	}
	if statuses[1].Account.Name != "work" || !statuses[1].Auth.LoggedIn {
		t.Fatalf("unexpected logged-in work status: %#v", statuses[1])
	}
	if statuses[1].Quota == nil || statuses[1].Quota.RateLimits.Primary == nil || statuses[1].Quota.RateLimits.Primary.UsedPercent != 18 {
		t.Fatalf("unexpected work quota: %#v", statuses[1].Quota)
	}
	if requests != 1 {
		t.Fatalf("expected only logged-in account to fetch quota, got %d requests", requests)
	}
	if gotAuth != "Bearer work-token" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
}

func TestCollectStatusesSanitizesLoggedInQuotaFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_SWITCH_CACHE", filepath.Join(root, "cache"))
	workHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(workHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workHome, "auth.json"), []byte(`{"tokens":{"access_token":"work-token"}}`), 0o600); err != nil {
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
	cfg := defaultConfig()
	cfg.Accounts = []AccountConfig{{Name: "work", Home: workHome}}
	cfg.Defaults.Account = "work"

	statuses := CollectStatuses(context.Background(), cfg, true)
	if len(statuses) != 1 {
		t.Fatalf("expected one status, got %#v", statuses)
	}
	if !statuses[0].Auth.LoggedIn || statuses[0].Quota != nil || statuses[0].QuotaError != "login refresh needed" {
		t.Fatalf("unexpected quota failure status: %#v", statuses[0])
	}
	if strings.Contains(statuses[0].QuotaError, "secret failure body") {
		t.Fatalf("quota error leaked response body: %q", statuses[0].QuotaError)
	}
}

func TestFlexibleStringHandlesEndpointShapeChanges(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "string", data: `"hard_limit"`, want: "hard_limit"},
		{name: "object type", data: `{"type":"weekly"}`, want: "weekly"},
		{name: "object reason", data: `{"reason":"daily"}`, want: "daily"},
		{name: "object fallback", data: `{"unknown":true}`, want: `{"unknown":true}`},
		{name: "null", data: `null`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got FlexibleString
			if err := json.Unmarshal([]byte(tc.data), &got); err != nil {
				t.Fatal(err)
			}
			if got.Value != tc.want {
				t.Fatalf("unexpected flexible string: got %q want %q", got.Value, tc.want)
			}
		})
	}
}

func TestQuotaCachePathRejectsUnsafeAccountName(t *testing.T) {
	if path, err := quotaCachePath(Account{Name: "../bad"}); err == nil {
		t.Fatalf("expected unsafe account name error, got path %q", path)
	}
}

func TestQuotaCachePathIncludesAccountHomeHash(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", cache)
	homeA := filepath.Join(t.TempDir(), ".codex-work")
	homeB := filepath.Join(t.TempDir(), ".codex-work")

	pathA, err := quotaCachePath(Account{Name: "work", Home: homeA})
	if err != nil {
		t.Fatal(err)
	}
	pathB, err := quotaCachePath(Account{Name: "work", Home: homeB})
	if err != nil {
		t.Fatal(err)
	}
	if pathA == pathB {
		t.Fatalf("expected cache paths to differ for different homes, got %q", pathA)
	}
	if strings.Contains(pathA, homeA) || strings.Contains(pathB, homeB) {
		t.Fatalf("cache path should not leak full account home: %q %q", pathA, pathB)
	}
	if !strings.HasPrefix(filepath.Base(pathA), "work-") || !strings.HasSuffix(pathA, ".json") {
		t.Fatalf("unexpected cache filename: %q", pathA)
	}
}

func TestWriteQuotaCacheReplacesExistingCacheWithoutTempFiles(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", cache)
	account := Account{Name: "work", Home: filepath.Join(t.TempDir(), ".codex-work")}
	first := &LiveRateLimitResponse{RateLimits: LiveRateLimit{Primary: &LiveQuotaWindow{UsedPercent: 10}}}
	second := &LiveRateLimitResponse{RateLimits: LiveRateLimit{Primary: &LiveQuotaWindow{UsedPercent: 20}}}

	if err := writeQuotaCache(account, first); err != nil {
		t.Fatal(err)
	}
	if err := writeQuotaCache(account, second); err != nil {
		t.Fatal(err)
	}
	got, ok := readQuotaCache(defaultConfig(), account)
	if !ok {
		t.Fatal("expected quota cache to be readable")
	}
	if got.RateLimits.Primary == nil || got.RateLimits.Primary.UsedPercent != 20 {
		t.Fatalf("quota cache was not replaced cleanly: %#v", got.RateLimits.Primary)
	}
	path, err := quotaCachePath(account)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("writeQuotaCache left temporary files: %#v", matches)
	}
}

func TestReadQuotaCacheIgnoresExpiredAndMalformedCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CODEX_SWITCH_CACHE", cache)
	cfg := defaultConfig()
	cfg.UI.QuotaTTLSeconds = 1
	account := Account{Name: "work", Home: filepath.Join(t.TempDir(), ".codex-work")}
	quota := &LiveRateLimitResponse{RateLimits: LiveRateLimit{Primary: &LiveQuotaWindow{UsedPercent: 10}}}

	if err := writeQuotaCache(account, quota); err != nil {
		t.Fatal(err)
	}
	path, err := quotaCachePath(account)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if cached, ok := readQuotaCache(cfg, account); ok {
		t.Fatalf("expected expired quota cache to be ignored, got %#v", cached)
	}

	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cached, ok := readQuotaCache(defaultConfig(), account); ok {
		t.Fatalf("expected malformed quota cache to be ignored, got %#v", cached)
	}
}
