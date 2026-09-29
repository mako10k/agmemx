package domain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDirectoryAndSubtree(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "notes", "child")
	outside := filepath.Join(root, "notebook")
	for _, dir := range []string{inside, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "notes"), link); err != nil {
		t.Fatal(err)
	}

	parent, err := ResolveDirectory(link)
	if err != nil {
		t.Fatal(err)
	}
	child, err := ResolveDirectory(filepath.Join(link, "child"))
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := ResolveDirectory(outside)
	if err != nil {
		t.Fatal(err)
	}
	if parent.String() != filepath.Join(root, "notes") || child.String() != inside {
		t.Fatalf("symlink was not canonicalized: parent=%q child=%q", parent, child)
	}
	if !parent.Contains(parent) || !parent.Contains(child) || parent.Contains(sibling) || child.Contains(parent) {
		t.Fatal("subtree membership ignored path component boundaries")
	}
}

func TestStoredPathCanOutliveDirectory(t *testing.T) {
	old := filepath.Join(t.TempDir(), "already-moved", "child")
	p, err := ParseStoredPath(old)
	if err != nil || p.String() != old {
		t.Fatalf("valid absent path: %q, %v", p, err)
	}
	for _, value := range []string{"", "relative", old + "/../child", old + "/", old + "\x00bad", string([]byte{'/', 0xff})} {
		if _, err := ParseStoredPath(value); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("accepted invalid stored path %q: %v", value, err)
		}
	}
	if (DomainPath{}).Valid() || (DomainPath{}).Contains(p) {
		t.Fatal("zero path acted as a valid scope")
	}
}

func TestResolveDirectoryRejectsMissingAndFile(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveDirectory(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory error = %v", err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDirectory(file); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("regular file error = %v", err)
	}
	if _, err := ResolveDirectory(""); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("empty path error = %v", err)
	}
}

func TestMovePreservesSuffixAndExcludesOtherSubtrees(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old") // Deliberately absent after a physical move.
	newDir := filepath.Join(root, "new")
	if err := os.Mkdir(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	move, err := ResolveMove(old, newDir)
	if err != nil {
		t.Fatal(err)
	}
	if move.From().String() != old || move.To().String() != newDir {
		t.Fatalf("wrong move endpoints: %q -> %q", move.From(), move.To())
	}
	for input, want := range map[string]string{
		old:                                 newDir,
		filepath.Join(old, "child"):         filepath.Join(newDir, "child"),
		filepath.Join(old, "child", "leaf"): filepath.Join(newDir, "child", "leaf"),
	} {
		path, _ := ParseStoredPath(input)
		got, matched := move.Map(path)
		if !matched || got.String() != want {
			t.Errorf("Map(%q) = %q, %t; want %q", input, got, matched, want)
		}
	}
	for _, input := range []string{root, filepath.Join(root, "oldish"), filepath.Join(root, "elsewhere")} {
		path, _ := ParseStoredPath(input)
		got, matched := move.Map(path)
		if matched || got.Valid() {
			t.Errorf("mapped unrelated path %q to %q", input, got)
		}
	}
}

func TestMoveRejectsRootEqualAndMissingDestination(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	newDir := filepath.Join(root, "new")
	if err := os.Mkdir(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, endpoints := range [][2]string{{"/", newDir}, {old, "/"}, {newDir, newDir}} {
		if _, err := ResolveMove(endpoints[0], endpoints[1]); !errors.Is(err, ErrInvalidMove) {
			t.Errorf("accepted invalid move %q -> %q: %v", endpoints[0], endpoints[1], err)
		}
	}
	if _, err := ResolveMove(old, filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing destination error = %v", err)
	}
}
