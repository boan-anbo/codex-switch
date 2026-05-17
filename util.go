package main

import "strings"

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func str(v any, fallback string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}

func clean(text string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func truncate(text string, n int) string {
	text = clean(text)
	if len([]rune(text)) <= n {
		return text
	}
	runes := []rune(text)
	return string(runes[:n-1]) + "..."
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
