package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type LaunchOptions struct {
	Account Account
	Args    []string
	Print   bool
}

func CurrentAccount(cfg *Config) Account {
	currentHome := os.Getenv("CODEX_HOME")
	if currentHome == "" {
		currentHome = filepath.Join(homeDir(), ".codex")
	}
	currentHome = cleanPath(currentHome)
	for _, account := range cfg.AccountsList() {
		if samePath(currentHome, account.Home) {
			return account
		}
	}
	return Account{Name: "custom", Home: currentHome}
}

func InitAccountHome(cfg *Config, account Account) error {
	account.Home = cleanPath(account.Home)
	for _, name := range cfg.Defaults.Share {
		if err := validateSharedAssetName(name); err != nil {
			return fmt.Errorf("invalid shared asset %q: %w", name, err)
		}
	}
	if err := os.MkdirAll(account.Home, 0o700); err != nil {
		return err
	}
	shared := cleanPath(cfg.Defaults.ShareFromHome)
	for _, name := range cfg.Defaults.Share {
		target := filepath.Join(shared, name)
		dest := filepath.Join(account.Home, name)
		if _, err := os.Lstat(dest); err == nil {
			continue
		}
		info, err := os.Stat(target)
		if err != nil {
			continue
		}
		if err := linkSharedAsset(target, dest, info); err != nil {
			return fmt.Errorf("could not link shared asset %s: %w", name, err)
		}
	}
	return nil
}

func linkSharedAsset(target string, dest string, info os.FileInfo) error {
	if err := os.Symlink(target, dest); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	if info.IsDir() {
		return createWindowsJunction(target, dest)
	}
	if err := os.Link(target, dest); err == nil {
		return nil
	}
	return copyFile(target, dest, info.Mode())
}

func createWindowsJunction(target string, dest string) error {
	output, err := exec.Command("cmd", "/c", "mklink", "/J", dest, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

func CodexArgsNew(cfg *Config, presetName string, cwd string, passthrough []string) ([]string, error) {
	args, err := cfg.PresetArgs(presetName)
	if err != nil {
		return nil, err
	}
	if cwd != "" {
		args = append(args, "--cd", cwd)
	}
	args = append(args, passthrough...)
	return args, nil
}

func CodexArgsResume(cfg *Config, presetName string, cwd string, session string, last bool, all bool, passthrough []string) ([]string, error) {
	args := []string{"resume"}
	preset, err := cfg.PresetArgs(presetName)
	if err != nil {
		return nil, err
	}
	args = append(args, preset...)
	if last {
		args = append(args, "--last")
	}
	if all {
		args = append(args, "--all", "--include-non-interactive")
	}
	if cwd != "" {
		args = append(args, "--cd", cwd)
	}
	if session != "" {
		args = append(args, session)
	}
	args = append(args, passthrough...)
	return args, nil
}

func LaunchCodex(opts LaunchOptions, cfg *Config) error {
	bin := cfg.Codex.Bin
	if bin == "" {
		bin = "codex"
	}
	if opts.Print {
		fmt.Println(printableCommand(opts.Account.Home, bin, opts.Args))
		return nil
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return err
	}
	cmd := exec.Command(path, opts.Args...)
	cmd.Env = codexEnv(os.Environ(), opts.Account)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("codex exited with status %d", exitErr.ExitCode())
	}
	return err
}

func codexEnv(env []string, account Account) []string {
	return setEnv(env, "CODEX_HOME", account.Home)
}

func printableCommand(home, bin string, args []string) string {
	return printableCommandForOS(runtime.GOOS, home, bin, args)
}

func printableCommandForOS(osName, home, bin string, args []string) string {
	if osName == "windows" {
		parts := []string{"$env:CODEX_HOME=" + powershellQuote(home) + ";", "&", powershellQuote(bin)}
		for _, arg := range args {
			parts = append(parts, powershellQuote(arg))
		}
		return strings.Join(parts, " ")
	}
	parts := []string{"CODEX_HOME=" + shellQuote(home), shellQuote(bin)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '/' || r == '.' || r == ':' || r == '=' || r == '+' || r == ',' || r == '@' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func copySkillFile(dst string, contents string) error {
	return writeFileAtomic(dst, []byte(contents), 0o644)
}
