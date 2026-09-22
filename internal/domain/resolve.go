// Package domain resolves the directory that names a memory domain.
package domain

import (
	"errors"
	"os"
	"path/filepath"
)

// Kind classifies a resolution failure.
type Kind int

const (
	OK Kind = iota
	NotFound
	NotDirectory
	IsRoot
)

// Resolve canonicalizes dir, or cwd when dir is empty.
// Relative dir is joined to cwd. cwd is the process directory captured at startup.
func Resolve(cwd, dir string) (string, Kind) {
	raw := dir
	if raw == "" {
		raw = cwd
	}
	if raw == "" {
		return "", NotFound
	}
	if !filepath.IsAbs(raw) {
		if cwd == "" || !filepath.IsAbs(cwd) {
			return "", NotFound
		}
		raw = filepath.Join(cwd, raw)
	}
	raw = filepath.Clean(raw)
	eval, err := filepath.EvalSymlinks(raw)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", NotFound
		}
		if _, statErr := os.Lstat(raw); statErr == nil {
			return "", NotDirectory
		}
		return "", NotFound
	}
	eval = filepath.Clean(eval)
	info, err := os.Stat(eval)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", NotFound
		}
		return "", NotFound
	}
	if !info.IsDir() {
		return "", NotDirectory
	}
	if eval == string(filepath.Separator) {
		return "", IsRoot
	}
	return eval, OK
}
