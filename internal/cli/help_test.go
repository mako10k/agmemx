package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHelpMatchesCommandFlag(t *testing.T) {
	data := t.TempDir()
	env := []string{"HOME=" + t.TempDir(), "XDG_DATA_HOME=" + data, "XDG_CACHE_HOME=rel", "XDG_STATE_HOME=rel"}
	left, errLeft, codeLeft := run(t, []string{"help", "observe"}, nil, env, t.TempDir())
	right, errRight, codeRight := run(t, []string{"observe", "--help"}, []byte(`{"text":"ignored"}`), env, t.TempDir())
	if codeLeft != 0 || codeRight != 0 || errLeft != "" || errRight != "" {
		t.Fatalf("help codes %d %d", codeLeft, codeRight)
	}
	if string(left) != string(right) {
		t.Fatalf("help mismatch\n%s\n%s", left, right)
	}
	if _, err := os.Stat(filepath.Join(data, "agmemx")); !os.IsNotExist(err) {
		t.Fatal("help opened the store")
	}
	for _, topic := range []string{"concepts", "usecases", "belief add", "relation add", "domain attach", "embed reindex"} {
		out, _, code := run(t, []string{"help", topic}, nil, env, t.TempDir())
		if code != 0 || !bytes.Contains(out, []byte(topic)) {
			t.Fatalf("topic %s code %d body %s", topic, code, out)
		}
	}
	catalog, _, code := run(t, []string{"help"}, nil, env, t.TempDir())
	if code != 0 || !bytes.Contains(catalog, []byte("belief add")) || !bytes.Contains(catalog, []byte("relate")) {
		t.Fatalf("catalog %s", catalog)
	}
}

func TestSchemaListsMachineCommands(t *testing.T) {
	out, stderr, code := run(t, []string{"schema"}, nil, []string{"HOME=" + t.TempDir(), "XDG_DATA_HOME=rel"}, t.TempDir())
	if code != 0 || stderr != "" {
		t.Fatalf("schema %d %s", code, stderr)
	}
	var doc struct {
		Commands []struct {
			Name   string   `json:"name"`
			Fields []string `json:"fields"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, command := range doc.Commands {
		got[command.Name] = true
	}
	for _, name := range []string{"init", "observe", "belief add", "relation add", "search", "domain attach", "embed reindex"} {
		if !got[name] {
			t.Fatalf("missing %s in %#v", name, got)
		}
	}
}

func TestFieldFlagsExcludeJSON(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("FILEBODY-NOT-MEMORY"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{"alpha-record": {1, 0}})
	env := []string{"HOME=" + data, "XDG_DATA_HOME=" + data, "XDG_CACHE_HOME=" + t.TempDir(), "XDG_STATE_HOME=" + t.TempDir()}
	args := []string{
		"--dir", dir,
		"--embed-provider", "fixture",
		"--embed-model", "fixture-model",
		"--embed-fixture", fixture,
		"observe",
		"--text", "alpha-record",
		"--source", "a.txt",
		"--start", "0",
		"--end", "4",
	}
	out, stderr, code := run(t, args, nil, env, dir)
	if code != 0 || stderr != "" {
		t.Fatalf("flag observe %d %s %s", code, stderr, out)
	}
	if objectCount(t, data) != 1 {
		t.Fatal(objectCount(t, data))
	}
	conflict := append(args, "--text")
	_ = conflict
	out, stderr, code = run(t, args, []byte(`{"text":"alpha-record"}`), env, dir)
	assertCode(t, out, stderr, code, 2, "invalid_flag")
	if objectCount(t, data) != 1 {
		t.Fatal("conflict wrote another object")
	}
}

func TestBeliefAddAndCompatRelate(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{"plain-belief": {0, 1}, "second-belief": {0, 1}})
	env := []string{"HOME=" + data, "XDG_DATA_HOME=" + data, "XDG_CACHE_HOME=" + t.TempDir(), "XDG_STATE_HOME=" + t.TempDir()}
	base := []string{"--dir", dir, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	out, stderr, code := run(t, append(base, "belief", "add", "--text", "plain-belief"), nil, env, dir)
	if code != 0 || stderr != "" {
		t.Fatalf("belief add %d %s %s", code, stderr, out)
	}
	var first created
	if err := json.Unmarshal(out, &first); err != nil {
		t.Fatal(err)
	}
	second, stderr, code := run(t, append(base, "believe"), []byte(`{"text":"second-belief"}`), env, dir)
	if code != 0 {
		t.Fatalf("compat believe %d %s", code, stderr)
	}
	var other created
	if err := json.Unmarshal(second, &other); err != nil {
		t.Fatal(err)
	}
	out, stderr, code = run(t, []string{"--dir", dir, "relation", "add", "--kind", "next", "--from", first.ID, "--to", other.ID}, nil, env, dir)
	if code != 0 || stderr != "" {
		t.Fatalf("relation add %d %s %s", code, stderr, out)
	}
}
