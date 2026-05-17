package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIBinarySmoke(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "codex-switch")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	runExternal(t, nil, "go", "build", "-o", bin, ".")

	home := filepath.Join(tmp, "home")
	cache := filepath.Join(tmp, "cache")
	config := filepath.Join(tmp, "config.toml")
	mustMkdir(t, filepath.Join(home, ".codex", "sessions"))
	mustMkdir(t, cache)
	mustWrite(t, filepath.Join(home, ".codex", "config.toml"), "")

	env := testCLIEnv(home, cache, config)
	runBinary(t, env, bin, "version")
	help := runBinary(t, env, bin, "help")
	for _, want := range []string{
		"accounts [--json] [--refresh]",
		"account add NAME [--home PATH] [--label TEXT] [--login]",
		"init-account NAME",
		"login [NAME|--account NAME] [--print]",
		"run [NAME|--account NAME] [--print]",
		"list [--cwd DIR] [--limit N] [--json] [--sessions DIR]",
		"Existing ~/.codex-* homes are discovered automatically.",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}

	accounts := runBinary(t, env, bin, "accounts", "--json")
	if !strings.Contains(accounts, `"name": "default"`) {
		t.Fatalf("accounts output did not include default: %s", accounts)
	}

	newCmd := runBinary(t, env, bin, "new", "--account", "work", "--print", "--", "--model", "gpt-test")
	for _, want := range []string{".codex-work", "--model", "gpt-test"} {
		if !strings.Contains(newCmd, want) {
			t.Fatalf("new --print output missing %q: %s", want, newCmd)
		}
	}
	assertMissing(t, filepath.Join(home, ".codex-work", "config.toml"))
	assertMissing(t, filepath.Join(home, ".codex-work", "sessions"))

	resumeCmd := runBinary(t, env, bin, "resume", "--account", "work", "--all", "--print")
	for _, want := range []string{".codex-work", "resume", "--all", "--include-non-interactive"} {
		if !strings.Contains(resumeCmd, want) {
			t.Fatalf("resume --print output missing %q: %s", want, resumeCmd)
		}
	}
	assertMissing(t, filepath.Join(home, ".codex-work", "config.toml"))
	assertMissing(t, filepath.Join(home, ".codex-work", "sessions"))

	sessionID := "019d30aa-4798-7891-a56f-1f87a629e02c"
	sessionCmd := runBinary(t, env, bin, "resume", sessionID, "--print")
	for _, want := range []string{"resume", "--cd", sessionID} {
		if !strings.Contains(sessionCmd, want) {
			t.Fatalf("resume positional session id output missing %q: %s", want, sessionCmd)
		}
	}
	accountSessionCmd := runBinary(t, env, bin, "resume", "--account", "work", "--session", sessionID, "--print")
	for _, want := range []string{".codex-work", "resume", "--cd", sessionID} {
		if !strings.Contains(accountSessionCmd, want) {
			t.Fatalf("resume --account --session output missing %q: %s", want, accountSessionCmd)
		}
	}
	assertMissing(t, filepath.Join(home, ".codex-work", "config.toml"))
	assertMissing(t, filepath.Join(home, ".codex-work", "sessions"))

	projectDir := filepath.Join(tmp, "project")
	customSessions := filepath.Join(tmp, "custom-sessions")
	mustMkdir(t, customSessions)
	writeSession(t, filepath.Join(customSessions, "rollout-2026-01-01T00-00-00-"+sessionID+".jsonl"), projectDir)
	listOutput := runBinary(t, env, bin, "list", "--sessions", customSessions, "--cwd", projectDir)
	if !strings.Contains(listOutput, "resume: codex-switch resume --session "+sessionID) {
		t.Fatalf("list output did not include account-aware resume hint: %s", listOutput)
	}
	if strings.Contains(listOutput, "resume: codex resume") {
		t.Fatalf("list output should not bypass codex-switch: %s", listOutput)
	}

	loginCmd := runBinary(t, env, bin, "run", "work", "--print", "--", "login")
	for _, want := range []string{".codex-work", "codex", "login"} {
		if !strings.Contains(loginCmd, want) {
			t.Fatalf("run --print output missing %q: %s", want, loginCmd)
		}
	}
	assertMissing(t, filepath.Join(home, ".codex-work", "config.toml"))
	assertMissing(t, filepath.Join(home, ".codex-work", "sessions"))

	runBinary(t, env, bin, "account", "add", "work")
	quickNewCmd := runBinary(t, env, bin, "new", "work", "--print")
	if !strings.Contains(quickNewCmd, ".codex-work") {
		t.Fatalf("documented positional new command did not select work account: %s", quickNewCmd)
	}
	quickResumeCmd := runBinary(t, env, bin, "resume", "work", "--last", "--print")
	for _, want := range []string{".codex-work", "resume", "--last"} {
		if !strings.Contains(quickResumeCmd, want) {
			t.Fatalf("documented positional resume command missing %q: %s", want, quickResumeCmd)
		}
	}
	quickRunCmd := runBinary(t, env, bin, "run", "work", "--print", "--", "login")
	for _, want := range []string{".codex-work", "codex", "login"} {
		if !strings.Contains(quickRunCmd, want) {
			t.Fatalf("documented positional run command missing %q: %s", want, quickRunCmd)
		}
	}

	runBinary(t, env, bin, "account", "add", "work2")
	configData, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), "name = 'work2'") {
		t.Fatalf("account add did not write work2 config: %s", configData)
	}
	assertMissing(t, filepath.Join(home, ".codex-work2", "auth.json"))

	runBinary(t, env, bin, "init-account", "workinit")
	configData, err = os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), "name = 'workinit'") {
		t.Fatalf("init-account did not write workinit config: %s", configData)
	}
	assertMissing(t, filepath.Join(home, ".codex-workinit", "auth.json"))

	fakeDir := filepath.Join(tmp, "fake-bin")
	mustMkdir(t, fakeDir)
	writeFakeCodex(t, fakeDir)
	launchEnv := setEnv(env, "PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runBinary(t, launchEnv, bin, "new", "--account", "work3")
	launchedHome, err := os.ReadFile(filepath.Join(home, "launched-home.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(launchedHome), ".codex-work3") {
		t.Fatalf("launch did not use work3 CODEX_HOME: %s", launchedHome)
	}
	assertExists(t, filepath.Join(home, ".codex-work3", "config.toml"))
	assertExists(t, filepath.Join(home, ".codex-work3", "sessions"))
	assertMissing(t, filepath.Join(home, ".codex-work3", "auth.json"))

	skillEnv := setEnv(env, "CODEX_HOME", filepath.Join(home, ".codex"))
	runBinary(t, skillEnv, bin, "skill", "install")
	assertExists(t, filepath.Join(home, ".codex", "skills", "codex-switch", "SKILL.md"))
}

func testCLIEnv(home string, cache string, config string) []string {
	env := os.Environ()
	env = setEnv(env, "HOME", home)
	env = setEnv(env, "USERPROFILE", home)
	env = setEnv(env, "CODEX_SWITCH_CONFIG", config)
	env = setEnv(env, "CODEX_SWITCH_CACHE", cache)
	return env
}

func writeFakeCodex(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		mustWrite(t, filepath.Join(dir, "codex.cmd"), "@echo off\r\necho %CODEX_HOME%> \"%USERPROFILE%\\launched-home.txt\"\r\n")
		return
	}
	path := filepath.Join(dir, "codex")
	mustWrite(t, path, "#!/usr/bin/env sh\nset -eu\nprintf '%s\\n' \"$CODEX_HOME\" >\"$HOME/launched-home.txt\"\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func runBinary(t *testing.T, env []string, bin string, args ...string) string {
	t.Helper()
	return runExternal(t, env, bin, args...)
}

func runExternal(t *testing.T, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err=%v", path, err)
	}
}
