package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatCompactLiveQuotaShowsWeeklyFirst(t *testing.T) {
	deadline := time.Now().Add(49 * time.Hour).Unix()
	quota := LiveRateLimit{
		Primary:   &LiveQuotaWindow{UsedPercent: 20, WindowDurationMins: 300, ResetsAt: time.Now().Add(2 * time.Hour).Unix()},
		Secondary: &LiveQuotaWindow{UsedPercent: 70, WindowDurationMins: 10080, ResetsAt: deadline},
	}
	got := formatCompactLiveQuota(quota)
	for _, want := range []string{"7d 30% left/70% used", "reset", "2d", "avg/day", "5h 80% left/20% used", "2h left"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}
}

func TestFormatLiveRateLimitShowsDynamicWindowLabelsAndTimeLeft(t *testing.T) {
	plan := "pro"
	limit := "Codex"
	limited := FlexibleString{Value: "weekly"}
	quota := LiveRateLimit{
		PlanType:             &plan,
		LimitName:            &limit,
		RateLimitReachedType: &limited,
		Primary:              &LiveQuotaWindow{UsedPercent: 10, WindowDurationMins: 180, ResetsAt: time.Now().Add(2 * time.Hour).Unix()},
		Secondary:            &LiveQuotaWindow{UsedPercent: 60, WindowDurationMins: 20160, ResetsAt: time.Now().Add(73 * time.Hour).Unix()},
	}
	got := formatLiveRateLimit(quota)
	for _, want := range []string{"pro", "Codex", "14d 40% left/60% used", "3d 1h left", "avg/day", "3h 90% left/10% used", "2h left", "limited=weekly"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}
}

func TestFormatQuotaShowsDynamicWindowLabelsAndTimeLeft(t *testing.T) {
	quota := &RateLimits{
		PlanType: "team",
		Primary:  &QuotaWindow{UsedPercent: 15, WindowMin: 90, ResetsAtUnix: time.Now().Add(90 * time.Minute).Unix()},
		Secondary: &QuotaWindow{
			UsedPercent:  25,
			WindowMin:    10080,
			ResetsAtUnix: time.Now().Add(50 * time.Hour).Unix(),
		},
	}
	got := formatQuota(quota)
	for _, want := range []string{"team", "7d 75% left/25% used", "2d 2h left", "avg/day", "90m 85% left/15% used", "2h left"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}
}

func TestFormatWindowLabelFallsBackWhenDurationMissing(t *testing.T) {
	tests := map[int]string{
		0:     "7d",
		45:    "45m",
		180:   "3h",
		10080: "7d",
	}
	for mins, want := range tests {
		t.Run(want, func(t *testing.T) {
			if got := formatWindowLabel("7d", mins); got != want {
				t.Fatalf("formatWindowLabel(%d) = %q, want %q", mins, got, want)
			}
		})
	}
}

func TestFormatAccountStatusKeepsNameButPickerDetailsDoNotRepeatIt(t *testing.T) {
	status := AccountStatus{
		Account: Account{Name: "work", Home: "/tmp/work"},
		Auth:    AuthInfo{LoggedIn: true, Email: "user@example.com"},
	}
	full := formatAccountStatus(status)
	if !strings.Contains(full, "work") || !strings.Contains(full, "logged in") {
		t.Fatalf("expected command status to include account and details, got %q", full)
	}
	details := formatAccountDetails(status)
	if strings.Contains(details, "work") {
		t.Fatalf("picker details should not repeat account name, got %q", details)
	}
	if !strings.Contains(details, "logged in") {
		t.Fatalf("expected details to include status, got %q", details)
	}
}

func TestFormatAccountDetailsFallsBackWhenQuotaHasNoWindows(t *testing.T) {
	status := AccountStatus{
		Account: Account{Name: "work", Home: "/tmp/work"},
		Auth:    AuthInfo{LoggedIn: true, Email: "user@example.com"},
		Quota:   &LiveRateLimitResponse{RateLimits: LiveRateLimit{}},
	}
	got := formatAccountDetails(status)
	if got != "logged in  user@example.com" {
		t.Fatalf("unexpected details for empty quota: %q", got)
	}
}

func TestFormatDurationLeftNormalizesRoundedDayRemainder(t *testing.T) {
	tests := map[float64]string{
		23.1: "24h left",
		24:   "1d left",
		47.1: "2d left",
		49.1: "2d 2h left",
	}
	for hours, want := range tests {
		t.Run(want, func(t *testing.T) {
			if got := formatDurationLeft(hours); got != want {
				t.Fatalf("formatDurationLeft(%v) = %q, want %q", hours, got, want)
			}
		})
	}
}
