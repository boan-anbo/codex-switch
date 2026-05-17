package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorDoesNotPrintSecrets(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"user@example.com"}`))
	body := `{"auth_mode":"chatgpt","tokens":{"id_token":"` + header + "." + payload + `.","refresh_token":"secret-refresh-token"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = home
	cfg.Accounts = []AccountConfig{{Name: "default", Home: home}}

	out := captureStdout(t, func() {
		if err := cmdDoctor(context.Background(), cfg, CodexProvider{}, nil); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "secret-refresh-token") || strings.Contains(out, "auth.json") {
		t.Fatalf("doctor leaked secret-bearing auth detail: %s", out)
	}
	if !strings.Contains(out, "user@example.com") {
		t.Fatalf("doctor should keep safe identity details, got %s", out)
	}
}

func TestSharedAssetState(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".codex")
	accountHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(accountHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "config.toml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	account := Account{Name: "work", Home: accountHome}
	if got := sharedAssetState(cfg, account, "config.toml"); got != "not linked" {
		t.Fatalf("expected not linked, got %q", got)
	}
	if got := sharedAssetState(cfg, account, "missing.toml"); got != "source missing" {
		t.Fatalf("expected source missing, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(accountHome, "config.toml"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := sharedAssetState(cfg, account, "config.toml"); got != "exists locally" {
		t.Fatalf("expected exists locally, got %q", got)
	}
}

func TestSharedAssetStateReportsSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".codex")
	accountHome := filepath.Join(root, ".codex-work")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(accountHome, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(shared, "config.toml")
	if err := os.WriteFile(source, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Defaults.ShareFromHome = shared
	account := Account{Name: "work", Home: accountHome}

	dest := filepath.Join(accountHome, "config.toml")
	if err := os.Symlink(source, dest); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if got := sharedAssetState(cfg, account, "config.toml"); got != "linked" {
		t.Fatalf("expected linked, got %q", got)
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "other.toml")
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, dest); err != nil {
		t.Fatal(err)
	}
	if got := sharedAssetState(cfg, account, "config.toml"); got != "linked elsewhere" {
		t.Fatalf("expected linked elsewhere, got %q", got)
	}
}

func TestDoctorReportsRuntimeDuplicateHomes(t *testing.T) {
	cfg := providerTestConfig(t)
	defaultAccount, ok := cfg.Account("default")
	if !ok {
		t.Fatal("expected default account")
	}
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", defaultAccount.Home)
	provider := &fakeProvider{}

	out := captureStdout(t, func() {
		if err := cmdDoctor(context.Background(), cfg, provider, nil); err != nil {
			t.Fatal(err)
		}
	})
	want := "runtime account homes: account home " + defaultAccount.Home + " already used by default and work"
	if !strings.Contains(out, want) {
		t.Fatalf("doctor did not report runtime duplicate home.\nwant: %s\ngot: %s", want, out)
	}
}

func TestDoctorReportsAccountHomeEnvOverride(t *testing.T) {
	cfg := providerTestConfig(t)
	overrideHome := filepath.Join(t.TempDir(), ".codex-work-env")
	t.Setenv("CODEX_SWITCH_ACCOUNT_WORK_HOME", overrideHome)

	out := captureStdout(t, func() {
		if err := cmdDoctor(context.Background(), cfg, &fakeProvider{}, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "home override: CODEX_SWITCH_ACCOUNT_WORK_HOME") {
		t.Fatalf("doctor did not report account home env override: %s", out)
	}
	if !strings.Contains(out, cleanPath(overrideHome)) {
		t.Fatalf("doctor did not show resolved override home: %s", out)
	}
}
