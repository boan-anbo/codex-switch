package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultSessionsDirPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	codexHome := filepath.Join(home, ".codex-work")
	sessionsDir := filepath.Join(home, "custom-sessions")
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CODEX_SESSIONS_DIR", sessionsDir)
	if got := defaultSessionsDir(); got != sessionsDir {
		t.Fatalf("CODEX_SESSIONS_DIR should win: got %q want %q", got, sessionsDir)
	}

	t.Setenv("CODEX_SESSIONS_DIR", "")
	if got := defaultSessionsDir(); got != filepath.Join(codexHome, "sessions") {
		t.Fatalf("CODEX_HOME should provide sessions dir: got %q want %q", got, filepath.Join(codexHome, "sessions"))
	}

	t.Setenv("CODEX_HOME", "")
	if got := defaultSessionsDir(); got != filepath.Join(home, ".codex", "sessions") {
		t.Fatalf("default sessions dir should use home .codex: got %q", got)
	}
}

func TestDefaultSessionsDirExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	t.Setenv("CODEX_HOME", "~/.codex-alt")
	t.Setenv("CODEX_SESSIONS_DIR", "")
	if got := defaultSessionsDir(); got != filepath.Join(home, ".codex-alt", "sessions") {
		t.Fatalf("CODEX_HOME should expand ~: got %q", got)
	}

	t.Setenv("CODEX_SESSIONS_DIR", "~/sessions-alt")
	if got := defaultSessionsDir(); got != filepath.Join(home, "sessions-alt") {
		t.Fatalf("CODEX_SESSIONS_DIR should expand ~: got %q", got)
	}
}

func TestCollectSessionsFiltersByPathBoundary(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	child := filepath.Join(project, "child")
	sibling := filepath.Join(root, "project-other")
	sessionsDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSession(t, filepath.Join(sessionsDir, "rollout-2026-01-01T00-00-00-019d30aa-4798-7891-a56f-1f87a629e02c.jsonl"), child)
	writeSession(t, filepath.Join(sessionsDir, "rollout-2026-01-02T00-00-00-119d30aa-4798-7891-a56f-1f87a629e02c.jsonl"), sibling)

	sessions, err := collectSessions(sessionsDir, 10, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one matching session, got %d", len(sessions))
	}
	if sessions[0].CWD != child {
		t.Fatalf("unexpected cwd %q", sessions[0].CWD)
	}
}

func TestCollectSessionsMissingRootIsEmpty(t *testing.T) {
	sessions, err := collectSessions(filepath.Join(t.TempDir(), "missing"), 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions for missing root, got %d", len(sessions))
	}
}

func TestCollectSessionsNonPositiveLimitIsEmpty(t *testing.T) {
	sessions, err := collectSessions(t.TempDir(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions for zero limit, got %d", len(sessions))
	}
}

func TestSummarizeSessionReadsEscapedWindowsPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rollout-2026-01-01T00-00-00-019d30aa-4798-7891-a56f-1f87a629e02c.jsonl")
	cwd := `C:\Users\Alice\project`
	writeSession(t, file, cwd)

	summary, err := summarizeSession(file)
	if err != nil {
		t.Fatal(err)
	}
	if summary.CWD != cwd {
		t.Fatalf("expected Windows cwd %q, got %q", cwd, summary.CWD)
	}
	if summary.ID != "019d30aa-4798-7891-a56f-1f87a629e02c" {
		t.Fatalf("unexpected session id: %q", summary.ID)
	}
	if len(summary.Messages) != 2 {
		t.Fatalf("expected two message summaries, got %#v", summary.Messages)
	}
}

func TestFormatSessionUsesCodexSwitchResumeHint(t *testing.T) {
	summary := SessionSummary{
		ID:        "019d30aa-4798-7891-a56f-1f87a629e02c",
		UpdatedAt: "2026-01-01T00:00:00Z",
		CWD:       "/repo",
	}
	got := formatSession(summary)
	want := "resume: codex-switch resume --session 019d30aa-4798-7891-a56f-1f87a629e02c"
	if !strings.Contains(got, want) {
		t.Fatalf("expected switcher resume hint %q, got %q", want, got)
	}
	if strings.Contains(got, "resume: codex resume") {
		t.Fatalf("resume hint should not bypass codex-switch, got %q", got)
	}
}

func TestNormalizeTime(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "utc nanos", value: "2026-05-16T12:34:56.987654321Z", want: "2026-05-16T12:34:56Z"},
		{name: "offset", value: "2026-05-16T20:34:56+08:00", want: "2026-05-16T12:34:56Z"},
		{name: "invalid", value: "not a timestamp", want: "not a timestamp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTime(tc.value); got != tc.want {
				t.Fatalf("normalizeTime(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestNormalizeTimeAcceptsRFC3339Nano(t *testing.T) {
	value := time.Date(2026, 5, 16, 12, 34, 56, 123, time.FixedZone("HKT", 8*60*60)).Format(time.RFC3339Nano)
	if got := normalizeTime(value); got != "2026-05-16T04:34:56Z" {
		t.Fatalf("unexpected normalized time: %q", got)
	}
}

func writeSession(t *testing.T, path string, cwd string) {
	t.Helper()
	items := []map[string]any{
		{
			"type": "session_meta",
			"payload": map[string]any{
				"id":    "019d30aa-4798-7891-a56f-1f87a629e02c",
				"cwd":   cwd,
				"model": "gpt-test",
			},
		},
		{
			"type": "response_item",
			"payload": map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []map[string]string{{"type": "input_text", "text": "hello"}},
			},
		},
		{
			"type": "response_item",
			"payload": map[string]any{
				"type":    "message",
				"role":    "assistant",
				"content": []map[string]string{{"type": "output_text", "text": "world"}},
			},
		},
	}
	var body []byte
	for _, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, line...)
		body = append(body, '\n')
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
