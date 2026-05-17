package main

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"
)

func formatAccountStatus(status AccountStatus) string {
	return fmt.Sprintf("%-14s %s", accountDisplayName(status.Account), formatAccountDetails(status))
}

func accountDisplayName(account Account) string {
	if account.Label != "" {
		return account.Label
	}
	return account.Name
}

func formatAccountDetails(status AccountStatus) string {
	email := status.Auth.Email
	if email == "" && status.Auth.LoggedIn {
		email = "logged in"
	}
	if !status.Auth.LoggedIn {
		return "not logged in  " + shortPath(status.Account.Home)
	}
	if status.QuotaError != "" {
		return "quota unavailable  " + email
	}
	if status.Quota == nil {
		return "logged in  " + email
	}
	quota := formatCompactLiveQuota(status.Quota.RateLimits)
	if quota == "" {
		return "logged in  " + email
	}
	return quota + "  " + email
}

func formatCompactLiveQuota(q LiveRateLimit) string {
	secondary := ""
	if q.Secondary != nil {
		secondary = formatCompactWindow(*q.Secondary, true)
	}
	primary := ""
	if q.Primary != nil {
		primary = "  " + formatCompactPrimaryWindow(*q.Primary)
	}
	if secondary == "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(secondary + primary)
}

func formatCompactPrimaryWindow(window LiveQuotaWindow) string {
	parts := []string{formatWindowLabel("5h", window.WindowDurationMins) + " " + formatRemainingUsed(window.UsedPercent)}
	if window.ResetsAt > 0 {
		if hours := time.Until(time.Unix(window.ResetsAt, 0)).Hours(); hours > 0 {
			parts = append(parts, formatDurationLeft(hours))
		}
	}
	return strings.Join(parts, " ")
}

func formatCompactWindow(window LiveQuotaWindow, includeAverage bool) string {
	remaining := remainingPercent(window.UsedPercent)
	parts := []string{fmt.Sprintf("%s %.0f%% left/%.0f%% used", formatWindowLabel("7d", window.WindowDurationMins), remaining, window.UsedPercent)}
	if window.ResetsAt > 0 {
		deadline := time.Unix(window.ResetsAt, 0)
		parts = append(parts, "reset "+formatDeadline(deadline))
		if hours := time.Until(deadline).Hours(); hours > 0 {
			parts = append(parts, formatDurationLeft(hours))
		}
		if includeAverage {
			if avg, ok := averageRemainingPerDay(remaining, deadline); ok {
				parts = append(parts, fmt.Sprintf("avg/day %.1f%%", avg))
			}
		}
	}
	return strings.Join(parts, " ")
}

func formatRemainingUsed(usedPercent float64) string {
	return fmt.Sprintf("%.0f%% left/%.0f%% used", remainingPercent(usedPercent), usedPercent)
}

func formatLiveRateLimit(q LiveRateLimit) string {
	parts := []string{}
	if q.PlanType != nil && *q.PlanType != "" {
		parts = append(parts, *q.PlanType)
	}
	if q.LimitName != nil && *q.LimitName != "" {
		parts = append(parts, *q.LimitName)
	}
	if q.Secondary != nil {
		parts = append(parts, formatQuotaWindow("7d", q.Secondary.WindowDurationMins, q.Secondary.UsedPercent, q.Secondary.ResetsAt, true))
	}
	if q.Primary != nil {
		parts = append(parts, formatQuotaWindow("5h", q.Primary.WindowDurationMins, q.Primary.UsedPercent, q.Primary.ResetsAt, false))
	}
	if q.RateLimitReachedType != nil && q.RateLimitReachedType.Value != "" {
		parts = append(parts, "limited="+q.RateLimitReachedType.Value)
	}
	return strings.Join(parts, " | ")
}

func formatQuota(q *RateLimits) string {
	if q == nil {
		return ""
	}
	parts := []string{}
	if q.PlanType != "" {
		parts = append(parts, q.PlanType)
	}
	if q.Secondary != nil {
		parts = append(parts, formatQuotaWindow("7d", q.Secondary.WindowMin, q.Secondary.UsedPercent, q.Secondary.ResetsAtUnix, true))
	}
	if q.Primary != nil {
		parts = append(parts, formatQuotaWindow("5h", q.Primary.WindowMin, q.Primary.UsedPercent, q.Primary.ResetsAtUnix, false))
	}
	return strings.Join(parts, " | ")
}

func formatQuotaWindow(fallbackLabel string, windowMins int, usedPercent float64, resetsAt int64, includeDailyAverage bool) string {
	remaining := remainingPercent(usedPercent)
	parts := []string{fmt.Sprintf("%s %.0f%% left/%.0f%% used", formatWindowLabel(fallbackLabel, windowMins), remaining, usedPercent)}
	if resetsAt > 0 {
		deadline := time.Unix(resetsAt, 0)
		parts = append(parts, "reset "+formatDeadline(deadline))
		if hours := time.Until(deadline).Hours(); hours > 0 {
			parts = append(parts, formatDurationLeft(hours))
		}
		if includeDailyAverage {
			if avg, ok := averageRemainingPerDay(remaining, deadline); ok {
				parts = append(parts, fmt.Sprintf("avg/day %.1f%%", avg))
			}
		}
	}
	return strings.Join(parts, " ")
}

func formatWindowLabel(fallback string, mins int) string {
	if mins <= 0 {
		return fallback
	}
	if mins%1440 == 0 {
		return fmt.Sprintf("%dd", mins/1440)
	}
	if mins%60 == 0 {
		return fmt.Sprintf("%dh", mins/60)
	}
	return fmt.Sprintf("%dm", mins)
}

func formatDeadline(deadline time.Time) string {
	now := time.Now()
	if deadline.Year() != now.Year() {
		return deadline.Format("2 Jan 2006 15:04")
	}
	return deadline.Format("2 Jan 15:04")
}

func formatDurationLeft(hours float64) string {
	if hours < 24 {
		return fmt.Sprintf("%.0fh left", math.Ceil(hours))
	}
	days := math.Floor(hours / 24)
	remainder := math.Ceil(math.Mod(hours, 24))
	if remainder <= 0 {
		return fmt.Sprintf("%.0fd left", days)
	}
	if remainder >= 24 {
		days++
		return fmt.Sprintf("%.0fd left", days)
	}
	return fmt.Sprintf("%.0fd %.0fh left", days, remainder)
}

func averageRemainingPerDay(remainingPercent float64, deadline time.Time) (float64, bool) {
	hoursLeft := time.Until(deadline).Hours()
	if hoursLeft <= 0 {
		return 0, false
	}
	daysLeft := hoursLeft / 24
	return remainingPercent / daysLeft, true
}

func remainingPercent(used float64) float64 {
	remaining := 100 - used
	if remaining < 0 {
		return 0
	}
	if remaining > 100 {
		return 100
	}
	return remaining
}

func shortPath(path string) string {
	home := homeDir()
	path = cleanPath(path)
	if home != "" {
		home = cleanPath(home)
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(filepath.Separator)) {
			return "~" + strings.TrimPrefix(path, home)
		}
	}
	return path
}

func compactJSON(data []byte) string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return strings.TrimSpace(string(data))
	}
	out, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(data))
	}
	return string(out)
}
