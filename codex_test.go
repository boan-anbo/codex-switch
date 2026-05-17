package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCodexArgsNewAppliesPresetCWDAndPassthrough(t *testing.T) {
	cfg := defaultConfig()
	args, err := CodexArgsNew(cfg, "full-access", "/tmp/project", []string{"--model", "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--dangerously-bypass-approvals-and-sandbox", "--sandbox", "danger-full-access", "--cd", "/tmp/project", "--model", "gpt-5"}
	if !equalStrings(args, want) {
		t.Fatalf("args mismatch:\n got %#v\nwant %#v", args, want)
	}
}

func TestCodexArgsResume(t *testing.T) {
	cfg := defaultConfig()
	args, err := CodexArgsResume(cfg, "full-access", "/tmp/project", "session-id", false, false, []string{"--model", "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"resume", "--dangerously-bypass-approvals-and-sandbox", "--sandbox", "danger-full-access", "--cd", "/tmp/project", "session-id", "--model", "gpt-5"}
	if !equalStrings(args, want) {
		t.Fatalf("args mismatch:\n got %#v\nwant %#v", args, want)
	}
}

func TestCodexArgsResumeAllIncludesNonInteractive(t *testing.T) {
	cfg := defaultConfig()
	args, err := CodexArgsResume(cfg, "", "", "", false, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(args, "--all") || !contains(args, "--include-non-interactive") {
		t.Fatalf("expected resume-all flags, got %#v", args)
	}
}

func TestCodexEnvSetsOnlySelectedHome(t *testing.T) {
	env := codexEnv([]string{"PATH=/bin", "CODEX_HOME=/old/home"}, Account{Name: "work", Home: "/new/home"})
	if !contains(env, "PATH=/bin") {
		t.Fatalf("expected unrelated env to remain, got %#v", env)
	}
	if !contains(env, "CODEX_HOME=/new/home") {
		t.Fatalf("expected selected CODEX_HOME, got %#v", env)
	}
	if contains(env, "CODEX_HOME=/old/home") {
		t.Fatalf("old CODEX_HOME should be replaced, got %#v", env)
	}
}

func TestPrintableCommandIncludesSelectedHome(t *testing.T) {
	got := printableCommandForOS("linux", "/tmp/codex work", "codex", []string{"resume", "abc"})
	if got != "CODEX_HOME='/tmp/codex work' codex resume abc" {
		t.Fatalf("unexpected printable command: %s", got)
	}
}

func TestPrintableCommandUsesPowerShellSyntaxOnWindows(t *testing.T) {
	got := printableCommandForOS("windows", `C:\Users\Alice\.codex-work`, "codex", []string{"resume", "abc def", "O'Brien"})
	want := `$env:CODEX_HOME='C:\Users\Alice\.codex-work'; & 'codex' 'resume' 'abc def' 'O''Brien'`
	if got != want {
		t.Fatalf("unexpected printable PowerShell command:\n got %s\nwant %s", got, want)
	}
}

func TestLaunchCodexPrintsSelectedHomeCommand(t *testing.T) {
	cfg := defaultConfig()
	cfg.Codex.Bin = "codex"
	var launchErr error
	out := captureStdout(t, func() {
		launchErr = LaunchCodex(LaunchOptions{
			Account: Account{Name: "work", Home: "/tmp/codex work"},
			Args:    []string{"resume", "abc"},
			Print:   true,
		}, cfg)
	})
	if launchErr != nil {
		t.Fatal(launchErr)
	}
	want := "CODEX_HOME='/tmp/codex work' codex resume abc"
	if runtime.GOOS == "windows" {
		want = "$env:CODEX_HOME='/tmp/codex work'; & 'codex' 'resume' 'abc'"
	}
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected print output: %q", out)
	}
}

func TestLaunchCodexRunsWithSelectedHomeAndArgs(t *testing.T) {
	if os.Getenv("CODEX_SWITCH_TEST_HELPER") == "1" {
		launchCodexHelper()
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "launch.txt")
	t.Setenv("CODEX_SWITCH_TEST_HELPER", "1")
	t.Setenv("CODEX_SWITCH_TEST_HELPER_OUT", outPath)
	cfg := defaultConfig()
	cfg.Codex.Bin = exe

	err = LaunchCodex(LaunchOptions{
		Account: Account{Name: "work", Home: "/tmp/codex-work"},
		Args:    []string{"-test.run=TestLaunchCodexRunsWithSelectedHomeAndArgs", "--", "resume", "abc"},
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "CODEX_HOME=/tmp/codex-work\nargs=resume abc\n"
	if string(data) != want {
		t.Fatalf("unexpected helper output:\n got %q\nwant %q", data, want)
	}
}

func launchCodexHelper() {
	out := os.Getenv("CODEX_SWITCH_TEST_HELPER_OUT")
	if out == "" {
		os.Exit(2)
	}
	args := []string{}
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	data := "CODEX_HOME=" + os.Getenv("CODEX_HOME") + "\nargs=" + strings.Join(args, " ") + "\n"
	if err := os.WriteFile(out, []byte(data), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestInitAccountHomeLinksSharedAssets(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".codex")
	accountHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(filepath.Join(shared, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "config.toml"), []byte("model = \"test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	cfg.Defaults.Share = []string{"config.toml", "sessions"}

	if err := InitAccountHome(cfg, Account{Name: "work", Home: accountHome}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(accountHome, "config.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(accountHome, "sessions")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(accountHome, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("auth.json should not be created or linked, err=%v", err)
	}
}

func TestInitAccountHomeNormalizesHomeAndSharedRoot(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared", "..", ".codex")
	accountHome := filepath.Join(root, "accounts", "..", ".codex-work")
	if err := os.MkdirAll(cleanPath(shared), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cleanPath(shared), "config.toml"), []byte("model = \"test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	cfg.Defaults.Share = []string{"config.toml"}

	if err := InitAccountHome(cfg, Account{Name: "work", Home: accountHome}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(cleanPath(accountHome), "config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestInitAccountHomePreservesExistingLocalSharedAssets(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".codex")
	accountHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(filepath.Join(shared, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(accountHome, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "config.toml"), []byte("model = \"shared\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "sessions", "shared.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountHome, "config.toml"), []byte("model = \"local\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountHome, "sessions", "local.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	cfg.Defaults.Share = []string{"config.toml", "sessions"}

	if err := InitAccountHome(cfg, Account{Name: "work", Home: accountHome}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(accountHome, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "model = \"local\"\n" {
		t.Fatalf("local config.toml should be preserved, got %q", data)
	}
	if _, err := os.Stat(filepath.Join(accountHome, "sessions", "local.jsonl")); err != nil {
		t.Fatalf("local sessions directory should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(accountHome, "sessions", "shared.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("existing sessions directory should not be replaced by shared sessions, err=%v", err)
	}
}

func TestInitAccountHomeRejectsUnsafeSharedAsset(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".codex")
	accountHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	cfg.Defaults.Share = []string{"auth.json"}

	err := InitAccountHome(cfg, Account{Name: "work", Home: accountHome})
	if err == nil {
		t.Fatal("expected unsafe shared asset error")
	}
	if _, statErr := os.Stat(filepath.Join(accountHome, "auth.json")); !os.IsNotExist(statErr) {
		t.Fatalf("auth.json should not be created or linked, err=%v", statErr)
	}
}

func TestInitAccountHomeRejectsUnsafeSharedAssetBeforeTouchingHome(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".codex")
	accountHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "config.toml"), []byte("model = \"test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	cfg.Defaults.Share = []string{"config.toml", "auth.json"}

	err := InitAccountHome(cfg, Account{Name: "work", Home: accountHome})
	if err == nil {
		t.Fatal("expected unsafe shared asset error")
	}
	if _, statErr := os.Stat(accountHome); !os.IsNotExist(statErr) {
		t.Fatalf("account home should not be created for invalid shared config, err=%v", statErr)
	}
}

func TestCopyFilePreservesContents(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source.txt")
	dst := filepath.Join(root, "dest.txt")
	if err := os.WriteFile(src, []byte("shared config"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o640); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "shared config" {
		t.Fatalf("unexpected copied content: %q", data)
	}
}

func TestCopySkillFileReplacesExistingFileWithoutTempFiles(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "skills", "codex-switch", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copySkillFile(dst, "new skill"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new skill" {
		t.Fatalf("skill file was not replaced cleanly: %q", data)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(dst), ".SKILL.md.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("copySkillFile left temporary files: %#v", matches)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
