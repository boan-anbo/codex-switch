package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func ReadAuthInfo(home string) AuthInfo {
	data, err := os.ReadFile(filepath.Join(expandHome(home), "auth.json"))
	if err != nil {
		return AuthInfo{}
	}
	var parsed struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			IDToken     string `json:"id_token"`
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if json.Unmarshal(data, &parsed) != nil {
		return AuthInfo{}
	}
	info := AuthInfo{LoggedIn: parsed.AuthMode != "" || parsed.Tokens.AccessToken != ""}
	claims := jwtClaims(parsed.Tokens.IDToken)
	info.Email = str(claims["email"], "")
	if authClaim := asMap(claims["https://api.openai.com/auth"]); authClaim != nil {
		info.Plan = str(authClaim["chatgpt_plan_type"], "")
		if info.Email == "" {
			info.Email = str(authClaim["email"], "")
		}
	}
	return info
}

func readAccessToken(home string) (string, error) {
	data, err := os.ReadFile(filepath.Join(expandHome(home), "auth.json"))
	if err != nil {
		return "", err
	}
	var parsed struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if parsed.Tokens.AccessToken == "" {
		return "", errMissingAccessToken
	}
	return parsed.Tokens.AccessToken, nil
}

func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}
