package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashCompletionProtocol(t *testing.T) {
	data := t.TempDir()
	home := t.TempDir()
	cwd := t.TempDir()
	env := []string{
		"HOME=" + home,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=rel-cache",
		"XDG_STATE_HOME=rel-state",
	}
	relEnv := []string{
		"HOME=" + home,
		"XDG_DATA_HOME=rel-xdg",
		"XDG_CACHE_HOME=rel-cache",
		"XDG_STATE_HOME=rel-state",
	}

	mode, candidates := completeBash(t, env, cwd, "")
	if mode != "plain" {
		t.Fatalf("top mode %q", mode)
	}
	assertSame(t, candidates, []string{
		"init", "observe", "belief", "relation", "search", "domain", "embed", "help", "schema",
		"relate", "domain-attach", "reindex",
		"--dir", "--format", "--embed-provider", "--embed-model", "--embed-base-url", "--embed-api-key-env", "--embed-fixture",
	})
	for _, forbidden := range []string{"__completion", "believe"} {
		if containsCandidate(candidates, forbidden) {
			t.Fatalf("top-level listed %s: %v", forbidden, candidates)
		}
	}

	mode, candidates = completeBash(t, relEnv, cwd, "")
	if mode != "plain" || !containsCandidate(candidates, "observe") {
		t.Fatalf("relative XDG mode %q candidates %v", mode, candidates)
	}

	mode, candidates = completeBash(t, env, cwd, "belief", "")
	if mode != "plain" {
		t.Fatalf("belief mode %q", mode)
	}
	assertSame(t, candidates, []string{
		"add", "--text", "--reason-kind", "--reason-id", "--reason-text", "--about", "--interval-start", "--interval-end",
	})

	mode, candidates = completeBash(t, env, cwd, "relation", "")
	if mode != "plain" {
		t.Fatalf("relation mode %q", mode)
	}
	assertSame(t, candidates, []string{"add"})

	mode, candidates = completeBash(t, env, cwd, "domain", "")
	assertSame(t, candidates, []string{"attach"})
	if mode != "plain" {
		t.Fatalf("domain mode %q", mode)
	}

	mode, candidates = completeBash(t, env, cwd, "embed", "")
	assertSame(t, candidates, []string{"reindex"})
	if mode != "plain" {
		t.Fatalf("embed mode %q", mode)
	}

	mode, candidates = completeBash(t, env, cwd, "help", "")
	if mode != "plain" {
		t.Fatalf("help mode %q", mode)
	}
	assertSame(t, candidates, []string{
		"concepts", "usecases",
		"init", "observe", "belief", "relation", "search", "domain", "embed", "help", "schema",
		"relate", "domain-attach", "reindex",
	})

	mode, candidates = completeBash(t, env, cwd, "--embed-provider", "")
	if mode != "plain" {
		t.Fatalf("provider mode %q", mode)
	}
	assertSame(t, candidates, []string{"ollama", "openai", "fixture"})

	mode, candidates = completeBash(t, env, cwd, "--format", "json", "")
	if mode != "plain" || !containsCandidate(candidates, "observe") {
		t.Fatalf("after format mode %q candidates %v", mode, candidates)
	}

	mode, candidates = completeBash(t, env, cwd, "--dir", "")
	if mode != "dir" || len(candidates) != 0 {
		t.Fatalf("dir mode %q candidates %v", mode, candidates)
	}

	mode, candidates = completeBash(t, env, cwd, "--embed-fixture", "")
	if mode != "file" || len(candidates) != 0 {
		t.Fatalf("fixture mode %q candidates %v", mode, candidates)
	}
	mode, candidates = completeBash(t, env, cwd, "observe", "--source", "")
	if mode != "file" || len(candidates) != 0 {
		t.Fatalf("source mode %q candidates %v", mode, candidates)
	}

	mode, candidates = completeBash(t, env, cwd, "observe", "--")
	if mode != "plain" {
		t.Fatalf("observe flags mode %q", mode)
	}
	assertSame(t, candidates, []string{"--text", "--source", "--start", "--end", "--interval-start", "--interval-end"})

	mode, candidates = completeBash(t, env, cwd, "belief", "add", "--text", "")
	if mode != "none" || len(candidates) != 0 {
		t.Fatalf("text value mode %q candidates %v", mode, candidates)
	}

	for _, root := range []string{
		filepath.Join(data, "agmemx"),
		filepath.Join(home, ".local", "share", "agmemx"),
		filepath.Join(cwd, "agmemx"),
	} {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("completion created %s: %v", root, err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wd, "rel-xdg", "agmemx")); !os.IsNotExist(err) {
		t.Fatalf("relative XDG store: %v", err)
	}
}

func completeBash(t *testing.T, env []string, cwd string, args ...string) (string, []string) {
	t.Helper()
	full := append([]string{"__completion", "--bash"}, args...)
	out, stderr, code := run(t, full, nil, env, cwd)
	if code != 0 || stderr != "" {
		t.Fatalf("completion code %d stderr %q out %q", code, stderr, out)
	}
	text := string(out)
	line, rest, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(line, "__agmemx_completion_mode=") {
		t.Fatalf("protocol %q", text)
	}
	mode := strings.TrimPrefix(line, "__agmemx_completion_mode=")
	if rest == "" {
		return mode, nil
	}
	rest = strings.TrimSuffix(rest, "\n")
	if rest == "" {
		return mode, nil
	}
	return mode, strings.Split(rest, "\n")
}

func assertSame(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("candidates %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidates %#v want %#v", got, want)
		}
	}
}

func containsCandidate(candidates []string, want string) bool {
	for _, candidate := range candidates {
		if candidate == want {
			return true
		}
	}
	return false
}
