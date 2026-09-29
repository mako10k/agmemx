// Package domain resolves directory identities and maps canonical path subtrees.
// R: Define canonical domain paths and their component-safe move correspondence.
package domain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidPath  = errors.New("invalid domain path")
	ErrNotDirectory = errors.New("domain path is not a directory")
	ErrInvalidMove  = errors.New("invalid domain move")
)

// DomainPath is a normalized absolute directory identity. Its zero value is invalid.
// A stored path can remain valid after the directory itself has moved away.
type DomainPath struct{ value string }

func (p DomainPath) String() string { return p.value }

func (p DomainPath) Valid() bool { return validStoredPath(p.value) }

// ResolveDirectory resolves an existing directory, including any symbolic links,
// to the canonical absolute path used for a live --dir or move destination.
func ResolveDirectory(input string) (DomainPath, error) {
	if input == "" || !utf8.ValidString(input) || strings.IndexByte(input, 0) >= 0 {
		return DomainPath{}, ErrInvalidPath
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return DomainPath{}, fmt.Errorf("absolute domain path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return DomainPath{}, fmt.Errorf("resolve domain directory: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return DomainPath{}, fmt.Errorf("inspect domain directory: %w", err)
	}
	if !info.IsDir() {
		return DomainPath{}, ErrNotDirectory
	}
	return ParseStoredPath(canonical)
}

// ParseStoredPath accepts a previously canonicalized absolute path without
// requiring that its directory still exists. It never silently cleans a path.
func ParseStoredPath(value string) (DomainPath, error) {
	if !validStoredPath(value) {
		return DomainPath{}, ErrInvalidPath
	}
	return DomainPath{value: value}, nil
}

func validStoredPath(value string) bool {
	return value != "" && utf8.ValidString(value) &&
		strings.IndexByte(value, 0) < 0 && filepath.IsAbs(value) &&
		filepath.Clean(value) == value
}

// Contains reports whether candidate is this path or one of its descendants.
// It compares path components, so /a does not contain /ab.
func (p DomainPath) Contains(candidate DomainPath) bool {
	if !p.Valid() || !candidate.Valid() {
		return false
	}
	rel, err := filepath.Rel(p.value, candidate.value)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Move maps a saved old subtree to a live new directory. It does not mutate
// the filesystem or any record, reference, relation, or ledger state.
type Move struct {
	from DomainPath
	to   DomainPath
}

// ResolveMove accepts an old saved prefix even after it has disappeared. The
// destination must currently be an existing directory and is canonicalized.
func ResolveMove(fromStored, toDirectory string) (Move, error) {
	from, err := ParseStoredPath(fromStored)
	if err != nil {
		return Move{}, fmt.Errorf("old move prefix: %w", err)
	}
	to, err := ResolveDirectory(toDirectory)
	if err != nil {
		return Move{}, fmt.Errorf("new move directory: %w", err)
	}
	if from.value == string(filepath.Separator) || to.value == string(filepath.Separator) || from == to {
		return Move{}, ErrInvalidMove
	}
	return Move{from: from, to: to}, nil
}

func (m Move) From() DomainPath { return m.from }
func (m Move) To() DomainPath   { return m.to }

// Map preserves the relative suffix of a path in the old subtree. A path
// outside that subtree returns the zero DomainPath and false.
func (m Move) Map(path DomainPath) (DomainPath, bool) {
	if !m.from.Contains(path) || !m.to.Valid() {
		return DomainPath{}, false
	}
	rel, err := filepath.Rel(m.from.value, path.value)
	if err != nil {
		return DomainPath{}, false
	}
	return DomainPath{value: filepath.Join(m.to.value, rel)}, true
}
