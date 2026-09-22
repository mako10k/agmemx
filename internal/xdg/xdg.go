// Package xdg resolves the XDG directories that hold memory records.
package xdg

import (
	"os"
	"path/filepath"
	"strings"
)

// Roots are the absolute directories for records, cache, and state.
type Roots struct {
	Data  string
	Cache string
	State string
}

// Resolve applies the XDG base-directory defaults. A set relative value is rejected.
func Resolve(environ []string) (Roots, error) {
	env := map[string]string{}
	for _, pair := range environ {
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		env[key] = val
	}
	home := env["HOME"]
	data, err := one(env, "XDG_DATA_HOME", home, filepath.Join(".local", "share"))
	if err != nil {
		return Roots{}, err
	}
	cache, err := one(env, "XDG_CACHE_HOME", home, ".cache")
	if err != nil {
		return Roots{}, err
	}
	state, err := one(env, "XDG_STATE_HOME", home, filepath.Join(".local", "state"))
	if err != nil {
		return Roots{}, err
	}
	return Roots{Data: data, Cache: cache, State: state}, nil
}

func one(env map[string]string, key, home, def string) (string, error) {
	val, ok := env[key]
	if !ok || val == "" {
		val = filepath.Join(home, def)
	}
	if !filepath.IsAbs(val) {
		return "", os.ErrInvalid
	}
	return filepath.Clean(val), nil
}
