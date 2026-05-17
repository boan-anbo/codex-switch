package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func cleanPath(path string) string {
	expanded := expandHome(path)
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return filepath.Clean(expanded)
	}
	return filepath.Clean(abs)
}

func samePath(a, b string) bool {
	a = cleanPath(a)
	b = cleanPath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func setEnv(env []string, key, value string) []string {
	for i, item := range env {
		existingKey, _, ok := strings.Cut(item, "=")
		if ok && sameEnvKey(existingKey, key) {
			env[i] = key + "=" + value
			return env
		}
	}
	return append(env, key+"="+value)
}

func sameEnvKey(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
