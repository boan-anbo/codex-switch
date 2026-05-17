package main

import (
	"strings"
	"testing"
)

func TestEmbeddedSkillDocumentsSafeAccountWorkflow(t *testing.T) {
	data, err := embeddedSkills.ReadFile("skills/codex-switch/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"codex-switch current",
		"codex-switch doctor",
		"codex-switch new --account NAME",
		"codex-switch list --cwd .",
		"codex-switch resume --account NAME --session SESSION_ID",
		"Do not inspect or print access tokens",
		"Use `--print`",
		"Positional account",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("embedded skill is missing %q:\n%s", want, text)
		}
	}
}
