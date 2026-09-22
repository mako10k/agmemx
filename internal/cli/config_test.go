package cli_test

import (
	"strings"
	"testing"
)

func TestConfigSetsOllamaDefault(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home + "/cfg", "XDG_DATA_HOME=" + home + "/data", "XDG_CACHE_HOME=" + home + "/cache", "XDG_STATE_HOME=" + home + "/state"}
	out, stderr, code := run(t, []string{"config", "show"}, nil, env, t.TempDir())
	if code != 0 || stderr != "" {
		t.Fatalf("show %d %s %s", code, stderr, out)
	}
	text := string(out)
	if !strings.Contains(text, "embed-provider ollama default") || !strings.Contains(text, "embed-model nomic-embed-text default") {
		t.Fatalf("defaults %s", text)
	}
	out, stderr, code = run(t, []string{"config", "set", "embed-model", "other-model"}, nil, env, t.TempDir())
	if code != 0 || stderr != "" {
		t.Fatalf("set %d %s %s", code, stderr, out)
	}
	out, _, code = run(t, []string{"config", "show"}, nil, env, t.TempDir())
	if code != 0 || !strings.Contains(string(out), "embed-model other-model config") {
		t.Fatalf("saved %s", out)
	}
	out, stderr, code = run(t, []string{"help", "config"}, nil, []string{"HOME=" + home, "XDG_DATA_HOME=rel", "XDG_CONFIG_HOME=rel"}, t.TempDir())
	if code != 0 || stderr != "" || !strings.Contains(string(out), "nomic-embed-text") {
		t.Fatalf("help config %d %s %s", code, stderr, out)
	}
}

func TestSearchQueryIsAnArgument(t *testing.T) {
	out, stderr, code := run(t, []string{"--embed-provider", "fixture", "search"}, []byte(`{"query":"ignored"}`), []string{"HOME=" + t.TempDir()}, t.TempDir())
	assertCode(t, out, stderr, code, 2, "missing_field")
}
