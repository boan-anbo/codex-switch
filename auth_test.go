package main

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAuthInfoParsesEmailAndPlan(t *testing.T) {
	home := t.TempDir()
	token := testJWT(t, `{"email":"user@example.com","https://api.openai.com/auth":{"chatgpt_plan_type":"plus"}}`)
	body := `{"auth_mode":"chatgpt","tokens":{"id_token":"` + token + `","access_token":"redacted"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	info := ReadAuthInfo(home)
	if !info.LoggedIn || info.Email != "user@example.com" || info.Plan != "plus" {
		t.Fatalf("unexpected auth info: %#v", info)
	}
}

func TestReadAuthInfoUsesAuthClaimEmailFallback(t *testing.T) {
	home := t.TempDir()
	token := testJWT(t, `{"https://api.openai.com/auth":{"email":"claim@example.com","chatgpt_plan_type":"team"}}`)
	body := `{"auth_mode":"chatgpt","tokens":{"id_token":"` + token + `","access_token":"redacted"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	info := ReadAuthInfo(home)
	if !info.LoggedIn || info.Email != "claim@example.com" || info.Plan != "team" {
		t.Fatalf("unexpected auth info: %#v", info)
	}
}

func TestReadAuthInfoHandlesMissingOrMalformedAuth(t *testing.T) {
	if info := ReadAuthInfo(filepath.Join(t.TempDir(), "missing")); info.LoggedIn || info.Email != "" || info.Plan != "" {
		t.Fatalf("missing auth should produce empty status, got %#v", info)
	}

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if info := ReadAuthInfo(home); info.LoggedIn || info.Email != "" || info.Plan != "" {
		t.Fatalf("malformed auth should produce empty status, got %#v", info)
	}
}

func TestReadAccessTokenRejectsMissingToken(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := readAccessToken(home)
	if token != "" || !errors.Is(err, errMissingAccessToken) {
		t.Fatalf("expected missing access token error, got token=%q err=%v", token, err)
	}
}

func testJWT(t *testing.T, payloadJSON string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + "."
}
