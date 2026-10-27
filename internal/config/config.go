// Package config stores fs's per-user settings in the user's config folder
// (%AppData%\fs on Windows, ~/Library/Application Support/fs on macOS and
// ~/.config/fs on Linux).
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// defaultPathFile holds the folder served when fs runs without one.
const defaultPathFile = "default-path"

// Dir returns fs's config folder. It is not created.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "fs"), nil
}

// DefaultPath returns the folder set with --set-default-path, or "" if none
// was set.
func DefaultPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, defaultPathFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SetDefaultPath stores folder, an absolute path, as the default folder.
func SetDefaultPath(folder string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, defaultPathFile), []byte(folder+"\n"), 0o600)
}
