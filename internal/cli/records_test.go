package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"agmemx/internal/record"
)

func TestRecordsRoundTripThroughSearch(t *testing.T) {
	data := t.TempDir()
	cache := t.TempDir()
	state := t.TempDir()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(root, "notes")
	if err := os.Mkdir(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "a.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "b.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{
		"観測した":         {1, 0},
		"理由なし":         {0, 1},
		"観測が理由":        {0, 1},
		"信念が理由":        {0, 1},
		"文章が理由":        {0, 1},
		"二つの記録は矛盾している": {1, 1},
		"子の観測":         {0.2, 0.8},
		"矛盾を探す":        {1, 1},
	})
	env := []string{
		"HOME=" + data,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + cache,
		"XDG_STATE_HOME=" + state,
	}
	args := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}

	before := objectCount(t, data)
	out, errOut, code := run(t, append(args, "observe"), []byte(`{"text":"観測した","reason":{"kind":"belief","id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"reference":{"source":"notes/a.txt","start":0,"end":1}}`), env, root)
	assertCode(t, out, errOut, code, 1, "observation_reason_forbidden")
	if objectCount(t, data) != before {
		t.Fatal("observation with a reason left an object")
	}

	obs := create(t, args, env, root, "observe", `{"text":"観測した","reference":{"source":"notes/a.txt","start":0,"end":2},"interval":{"start":"2026-01-01T00:00:00Z","end":"2026-01-02T00:00:00Z"}}`)
	if obs.Kind != "observation" || obs.Domain != canonical(t, root) {
		t.Fatalf("observation %#v", obs)
	}
	wantObs, err := record.ContentSHA256(record.Object{
		Kind:      "observation",
		Text:      "観測した",
		Interval:  &record.Interval{Start: "2026-01-01T00:00:00Z", End: "2026-01-02T00:00:00Z"},
		Reference: &record.Reference{Source: "notes/a.txt", Start: 0, End: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if obs.ContentSHA256 != wantObs {
		t.Fatalf("hash %s want %s", obs.ContentSHA256, wantObs)
	}

	none := create(t, args, env, root, "believe", `{"text":"理由なし"}`)
	fromObs := create(t, args, env, root, "believe", `{"text":"観測が理由","reason":{"kind":"observation","id":"`+obs.ID+`"}}`)
	fromBelief := create(t, args, env, root, "believe", `{"text":"信念が理由","reason":{"kind":"belief","id":"`+none.ID+`"}}`)
	fromText := create(t, args, env, root, "believe", `{"text":"文章が理由","reason":{"kind":"text","text":"直接の説明"}}`)
	conflict := create(t, args, env, root, "believe", `{"text":"二つの記録は矛盾している","about":["`+fromText.ID+`","`+fromObs.ID+`"]}`)

	childArgs := []string{"--dir", child, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	childObs := create(t, childArgs, env, root, "observe", `{"text":"子の観測","reference":{"source":"b.txt","start":0,"end":1}}`)
	if childObs.ID == "" {
		t.Fatal("child reference should stay inside the parent directory")
	}

	escaped, errOut, code := run(t, append(childArgs, "believe"), []byte(`{"text":"理由なし","reason":{"kind":"observation","id":"`+obs.ID+`"}}`), env, root)
	assertCode(t, escaped, errOut, code, 1, "reason_not_found")

	found := search(t, args, env, root, `{"query":"矛盾を探す","limit":1}`)
	if len(found.Beliefs) != 1 || found.Beliefs[0].ID != conflict.ID {
		t.Fatalf("beliefs %#v", found.Beliefs)
	}
	got := found.Beliefs[0]
	wantAbout := []string{fromObs.ID, fromText.ID}
	sort.Strings(wantAbout)
	if len(got.About) != 2 || got.About[0] != wantAbout[0] || got.About[1] != wantAbout[1] {
		t.Fatalf("about %#v want %v", got.About, wantAbout)
	}
	if len(found.Observations) != 0 {
		t.Fatalf("observations %#v", found.Observations)
	}

	all := search(t, args, env, root, `{"query":"矛盾を探す"}`)
	var seenObs, seenChild bool
	for _, item := range all.Observations {
		if item.ID == obs.ID {
			seenObs = true
			if item.Reference.Source != "notes/a.txt" || item.Reference.Start != 0 || item.Reference.End != 2 {
				t.Fatalf("reference %#v", item.Reference)
			}
			if item.Interval == nil || item.Interval.Start != "2026-01-01T00:00:00Z" || item.Interval.End != "2026-01-02T00:00:00Z" {
				t.Fatalf("interval %#v", item.Interval)
			}
		}
		if item.ID == childObs.ID {
			seenChild = true
		}
	}
	if !seenObs || !seenChild {
		t.Fatal("parent search did not return both observations")
	}
	reasons := map[string]beliefHit{}
	for _, item := range all.Beliefs {
		reasons[item.Text] = item
	}
	if reasons["理由なし"].Reason != nil {
		t.Fatalf("empty reason %#v", reasons["理由なし"].Reason)
	}
	if reasons["観測が理由"].Reason == nil || reasons["観測が理由"].Reason.Kind != "observation" || reasons["観測が理由"].Reason.ID != obs.ID {
		t.Fatalf("observation reason %#v", reasons["観測が理由"].Reason)
	}
	if reasons["信念が理由"].Reason == nil || reasons["信念が理由"].Reason.Kind != "belief" || reasons["信念が理由"].Reason.ID != none.ID {
		t.Fatalf("belief reason %#v", reasons["信念が理由"].Reason)
	}
	if reasons["文章が理由"].Reason == nil || reasons["文章が理由"].Reason.Kind != "text" || reasons["文章が理由"].Reason.Text != "直接の説明" {
		t.Fatalf("text reason %#v", reasons["文章が理由"].Reason)
	}
	if fromBelief.ContentSHA256 == "" || fromText.ContentSHA256 == "" {
		t.Fatal("missing hashes")
	}

	childFound := search(t, childArgs, env, root, `{"query":"矛盾を探す"}`)
	for _, item := range childFound.Observations {
		if item.ID == obs.ID {
			t.Fatal("child search returned the parent observation")
		}
	}
	for _, item := range childFound.Beliefs {
		if item.ID == conflict.ID {
			t.Fatal("child search returned the parent belief")
		}
	}
}

func TestRecordRejectionsDoNotWrite(t *testing.T) {
	data := t.TempDir()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.txt"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{"観測した": {1, 0}})
	env := []string{"HOME=" + data, "XDG_DATA_HOME=" + data, "XDG_CACHE_HOME=" + t.TempDir(), "XDG_STATE_HOME=" + t.TempDir()}
	base := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	cases := []struct {
		args    []string
		command string
		in      string
		code    string
		exit    int
	}{
		{base, "observe", `{"text":"","reference":{"source":"a.txt","start":0,"end":1}}`, "text_empty", 1},
		{base, "observe", `{"text":"観測した","reference":{"source":"../a.txt","start":0,"end":1}}`, "reference_escapes_domain", 1},
		{base, "observe", `{"text":"観測した","reference":{"source":"missing.txt","start":0,"end":1}}`, "reference_not_found", 1},
		{base, "observe", `{"text":"観測した","reference":{"source":"bad.txt","start":0,"end":1}}`, "reference_encoding", 1},
		{base, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":0,"end":9}}`, "reference_span_invalid", 1},
		{base, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":1.5,"end":2}}`, "invalid_type", 2},
		{[]string{"--dir", root, "--embed-provider", "fixture"}, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":0,"end":1}}`, "embed_provider_unset", 1},
		{base, "observe", `{"text":"未登録","reference":{"source":"a.txt","start":0,"end":1}}`, "embed_fixture_invalid", 1},
		{base, "believe", `{"text":"理由なし","reason":{"kind":"nope"}}`, "reason_kind_invalid", 1},
	}
	for _, tc := range cases {
		args := append(append([]string{}, tc.args...), tc.command)
		out, errOut, code := run(t, args, []byte(tc.in), env, root)
		assertCode(t, out, errOut, code, tc.exit, tc.code)
	}
	if objectCount(t, data) != 0 {
		t.Fatal("rejections wrote an object")
	}

	wide := filepath.Join(t.TempDir(), "wide.json")
	writeFixture(t, wide, map[string][]float64{"別次元": {1, 0, 0}})
	_ = create(t, base, env, root, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":0,"end":1}}`)
	wideArgs := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", wide, "observe"}
	out, errOut, code := run(t, wideArgs, []byte(`{"text":"別次元","reference":{"source":"a.txt","start":0,"end":1}}`), env, root)
	assertCode(t, out, errOut, code, 1, "embed_dimension_mismatch")
	if objectCount(t, data) != 1 {
		t.Fatalf("dimension mismatch wrote a second object: %d", objectCount(t, data))
	}
}

type created struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Domain        string `json:"domain"`
	ContentSHA256 string `json:"content_sha256"`
}

type beliefHit struct {
	About  []string `json:"about"`
	ID     string   `json:"id"`
	Text   string   `json:"text"`
	Reason *struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"reason"`
}

type observationHit struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Interval *struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"interval"`
	Reference struct {
		Source string `json:"source"`
		Start  int    `json:"start"`
		End    int    `json:"end"`
	} `json:"reference"`
}

type searchBody struct {
	Beliefs      []beliefHit      `json:"beliefs"`
	Observations []observationHit `json:"observations"`
}

func create(t *testing.T, args []string, env []string, cwd, command, body string) created {
	t.Helper()
	out, errOut, code := run(t, append(append([]string{}, args...), command), []byte(body), env, cwd)
	if code != 0 {
		t.Fatalf("%s %s: %d %s %s", command, body, code, out, errOut)
	}
	var created created
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatal(err)
	}
	if len(created.ID) != 32 || created.ContentSHA256 == "" {
		t.Fatalf("%#v", created)
	}
	return created
}

func search(t *testing.T, args []string, env []string, cwd, body string) searchBody {
	t.Helper()
	var req struct {
		Query string `json:"query"`
		Limit *int   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	cmd := append(append([]string{}, args...), "search")
	if req.Query != "" {
		cmd = append(cmd, "--query", req.Query)
	}
	if req.Limit != nil {
		cmd = append(cmd, "--limit", strconv.Itoa(*req.Limit))
	}
	out, errOut, code := run(t, cmd, nil, env, cwd)
	if code != 0 {
		t.Fatalf("search %s: %d %s %s", body, code, out, errOut)
	}
	var found searchBody
	if err := json.Unmarshal(out, &found); err != nil {
		t.Fatal(err)
	}
	return found
}

func writeFixture(t *testing.T, path string, vectors map[string][]float64) {
	t.Helper()
	dim := 0
	for _, vec := range vectors {
		dim = len(vec)
		break
	}
	body, err := json.Marshal(map[string]any{"model": "fixture-model", "dimension": dim, "vectors": vectors})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func objectCount(t *testing.T, data string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(data, "agmemx", "objects"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
