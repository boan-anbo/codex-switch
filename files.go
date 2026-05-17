package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := renameFile(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func renameFile(oldPath string, newPath string) error {
	err := os.Rename(oldPath, newPath)
	if err == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return err
	}
	if _, statErr := os.Stat(oldPath); statErr != nil {
		return err
	}
	if _, statErr := os.Stat(newPath); statErr != nil {
		return err
	}
	if err := os.Remove(newPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(oldPath, newPath)
}
