package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAcceptanceSession is the contract's CLI sequence.
// It uses a temporary directory, a temporary XDG data home, and the fixture provider.
func TestAcceptanceSession(t *testing.T) {
	bin := buildAgmemx(t)
	data := t.TempDir()
	cache := t.TempDir()
	state := t.TempDir()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	other := t.TempDir()
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("FILEBODY-NOT-MEMORY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "b.txt"), []byte("FILEBODY-NOT-MEMORY"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{
		"alpha-record":     {1, 0},
		"plain-belief":     {0, 1},
		"from-observation": {0, 1},
		"from-belief":      {0, 1},
		"from-prose":       {0, 1},
		"records-conflict": {1, 1},
		"child-note":       {0.2, 0.8},
		"beta-lookup":      {1, 0},
	})
	env := []string{
		"HOME=" + t.TempDir(),
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + cache,
		"XDG_STATE_HOME=" + state,
	}
	flags := []string{
		"--format", "json",
		"--embed-provider", "fixture",
		"--embed-model", "fixture-model",
		"--embed-fixture", fixture,
	}

	initOut := mustCLI(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "init"), `{}`)
	if initOut["domain"] != canonical(t, root) {
		t.Fatalf("init domain %#v", initOut["domain"])
	}

	obs := mustCreate(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "observe"), `{
		"text":"alpha-record",
		"reference":{"source":"a.txt","start":0,"end":8},
		"interval":{"start":"2026-01-01T00:00:00Z","end":"2026-01-02T00:00:00Z"}
	}`)
	if objectCount(t, data) != 1 {
		t.Fatalf("objects %d", objectCount(t, data))
	}

	code, stderr, raw := callCLI(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "observe"), `{
		"text":"alpha-record",
		"reason":{"kind":"belief","id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"reference":{"source":"a.txt","start":0,"end":1}
	}`)
	assertCLICode(t, raw, stderr, code, 1, "observation_reason_forbidden")
	if objectCount(t, data) != 1 {
		t.Fatal("rejected observation left an object")
	}

	none := mustCreate(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "believe"), `{"text":"plain-belief"}`)
	fromObs := mustCreate(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "believe"), `{"text":"from-observation","reason":{"kind":"observation","id":"`+obs.ID+`"}}`)
	fromBelief := mustCreate(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "believe"), `{"text":"from-belief","reason":{"kind":"belief","id":"`+none.ID+`"}}`)
	fromText := mustCreate(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "believe"), `{"text":"from-prose","reason":{"kind":"text","text":"stated directly"}}`)
	if fromBelief.ContentSHA256 == "" || fromText.ContentSHA256 == "" {
		t.Fatal("belief hashes missing")
	}

	conflict := mustCreate(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "believe"), `{"text":"records-conflict","about":["`+fromText.ID+`","`+fromObs.ID+`"]}`)
	childObs := mustCreate(t, bin, child, env, append(append([]string{"--dir", child}, flags...), "observe"), `{"text":"child-note","reference":{"source":"b.txt","start":0,"end":4}}`)

	same := mustCLI(t, bin, root, env, []string{"--format", "json", "--dir", root, "relate"}, `{"kind":"next","from":"`+obs.ID+`","to":"`+none.ID+`"}`)
	if same["kind"] != "next" || same["domain"] != canonical(t, root) || same["from"] != obs.ID || same["to"] != none.ID {
		t.Fatalf("relate %#v", same)
	}
	code, stderr, raw = callCLI(t, bin, root, env, []string{"--format", "json", "--dir", root, "relate"}, `{"kind":"next","from":"`+obs.ID+`","to":"`+childObs.ID+`"}`)
	assertCLICode(t, raw, stderr, code, 1, "cross_domain_next")

	found := searchCLI(t, bin, root, env, append(append([]string{"--dir", root}, flags...), "search"), `{"query":"beta-lookup"}`)
	var seenObs bool
	for _, item := range found.Observations {
		if item.ID != obs.ID {
			continue
		}
		seenObs = true
		if item.Text != "alpha-record" {
			t.Fatalf("text %s", item.Text)
		}
		if item.Interval == nil || item.Interval.Start != "2026-01-01T00:00:00Z" || item.Interval.End != "2026-01-02T00:00:00Z" {
			t.Fatalf("interval %#v", item.Interval)
		}
		if item.Reference.Source != "a.txt" || item.Reference.Start != 0 || item.Reference.End != 8 {
			t.Fatalf("reference %#v", item.Reference)
		}
	}
	if !seenObs {
		t.Fatal("query with no shared word missed alpha-record")
	}
	if strings.Contains(found.raw, "FILEBODY-NOT-MEMORY") {
		t.Fatal("search returned the source file body")
	}
	var seenChild bool
	for _, item := range found.Observations {
		if item.ID == childObs.ID {
			seenChild = true
		}
	}
	if !seenChild {
		t.Fatal("parent search missed the child observation")
	}
	var seenConflict bool
	for _, item := range found.Beliefs {
		if item.ID == conflict.ID {
			seenConflict = true
			if len(item.About) != 2 || item.About[0] == item.About[1] {
				t.Fatalf("about %#v", item.About)
			}
		}
	}
	if !seenConflict {
		t.Fatal("conflict belief was not returned")
	}

	childFound := searchCLI(t, bin, child, env, append(append([]string{"--dir", child}, flags...), "search"), `{"query":"beta-lookup"}`)
	for _, item := range childFound.Observations {
		if item.ID == obs.ID {
			t.Fatal("child search returned the parent observation")
		}
	}
	for _, item := range childFound.Beliefs {
		if item.ID == none.ID || item.ID == conflict.ID {
			t.Fatal("child search returned a parent belief")
		}
	}

	attached := mustCreate(t, bin, root, env, []string{"--format", "json", "--dir", root, "domain-attach"}, `{"id":"`+obs.ID+`","domain":"`+other+`"}`)
	if attached.ContentSHA256 != obs.ContentSHA256 {
		t.Fatalf("hash %s became %s", obs.ContentSHA256, attached.ContentSHA256)
	}
	if attached.Domain != canonical(t, other) {
		t.Fatalf("attached domain %s", attached.Domain)
	}

	code, stderr, raw = callCLI(t, bin, root, env, []string{"--format", "json", "--dir", "/", "init"}, `{}`)
	assertCLICode(t, raw, stderr, code, 1, "dir_is_root")
	code, stderr, raw = callCLI(t, bin, root, []string{
		"HOME=" + t.TempDir(),
		"XDG_DATA_HOME=rel",
		"XDG_CACHE_HOME=" + cache,
		"XDG_STATE_HOME=" + state,
	}, []string{"--format", "json", "init"}, `{}`)
	assertCLICode(t, raw, stderr, code, 1, "xdg_relative")
}

func buildAgmemx(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agmemx")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/agmemx")
	cmd.Dir = moduleRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	return bin
}

func callCLI(t *testing.T, bin, dir string, env, args []string, stdin string) (int, string, []byte) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	return code, stderr.String(), stdout.Bytes()
}

func mustCLI(t *testing.T, bin, dir string, env, args []string, stdin string) map[string]any {
	t.Helper()
	code, stderr, stdout := callCLI(t, bin, dir, env, args, stdin)
	if code != 0 || stderr != "" {
		t.Fatalf("%v: %d %s %s", args, code, stderr, stdout)
	}
	var body map[string]any
	if err := json.Unmarshal(stdout, &body); err != nil {
		t.Fatalf("%s: %v", stdout, err)
	}
	return body
}

func mustCreate(t *testing.T, bin, dir string, env, args []string, stdin string) created {
	t.Helper()
	code, stderr, stdout := callCLI(t, bin, dir, env, args, stdin)
	if code != 0 || stderr != "" {
		t.Fatalf("%v %s: %d %s %s", args, stdin, code, stderr, stdout)
	}
	var body created
	if err := json.Unmarshal(stdout, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ID) != 32 || body.ContentSHA256 == "" {
		t.Fatalf("%#v", body)
	}
	return body
}

type acceptanceSearch struct {
	searchBody
	raw string
}

func searchCLI(t *testing.T, bin, dir string, env, args []string, stdin string) acceptanceSearch {
	t.Helper()
	var req struct {
		Query string `json:"query"`
		Limit *int   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(stdin), &req); err != nil {
		t.Fatal(err)
	}
	args = append(append([]string{}, args...), "--query", req.Query)
	if req.Limit != nil {
		args = append(args, "--limit", strconv.Itoa(*req.Limit))
	}
	code, stderr, stdout := callCLI(t, bin, dir, env, args, "")
	if code != 0 || stderr != "" {
		t.Fatalf("search: %d %s %s", code, stderr, stdout)
	}
	var body acceptanceSearch
	if err := json.Unmarshal(stdout, &body); err != nil {
		t.Fatal(err)
	}
	body.raw = string(stdout)
	return body
}

func assertCLICode(t *testing.T, stdout []byte, stderr string, code, wantExit int, wantCode string) {
	t.Helper()
	if code != wantExit {
		t.Fatalf("exit %d want %d stdout %s stderr %s", code, wantExit, stdout, stderr)
	}
	if wantExit <= 1 && stderr != "" {
		t.Fatalf("stderr %q", stderr)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout, &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != wantCode || body.Error.Message != contractMessage(wantCode) {
		t.Fatalf("error %#v want %s", body.Error, wantCode)
	}
}
