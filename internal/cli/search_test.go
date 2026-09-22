package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"agmemx/internal/embed"
)

func TestSearchSliceUsesEmbeddingCache(t *testing.T) {
	data := t.TempDir()
	cache := t.TempDir()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	sibling := t.TempDir()
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, child, sibling} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("FILEBODY-NOT-MEMORY"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	vectors := map[string][]float64{"探す": {1, 0}}
	for i := 0; i < 10; i++ {
		vectors[itemText(i)] = []float64{1, float64(i)}
	}
	vectors["兄弟"] = []float64{1, 0}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, vectors)
	env := []string{
		"HOME=" + data,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + cache,
		"XDG_STATE_HOME=" + t.TempDir(),
	}
	args := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	var created []created
	for i := 0; i < 10; i++ {
		command := "believe"
		body := `{"text":"` + itemText(i) + `"}`
		if i%2 == 0 {
			command = "observe"
			body = `{"text":"` + itemText(i) + `","reference":{"source":"a.txt","start":0,"end":4}}`
		}
		created = append(created, create(t, args, env, root, command, body))
	}
	create(t, []string{"--dir", sibling, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}, env, root, "believe", `{"text":"兄弟"}`)

	out, errOut, code := run(t, append(append([]string{}, args...), "search", "--query", "探す"), nil, env, root)
	if code != 0 || errOut != "" {
		t.Fatalf("search %d %s %s", code, out, errOut)
	}
	if bytesContain(out, []byte("FILEBODY-NOT-MEMORY")) || bytesContain(out, []byte("兄弟")) {
		t.Fatalf("slice leaked file text or a sibling: %s", out)
	}
	var raw struct {
		Beliefs      []map[string]json.RawMessage `json:"beliefs"`
		Observations []map[string]json.RawMessage `json:"observations"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Beliefs)+len(raw.Observations) != 8 {
		t.Fatalf("default limit returned %d", len(raw.Beliefs)+len(raw.Observations))
	}
	for _, item := range raw.Observations {
		if keys := mapKeys(item); !sameKeys(keys, "domain", "id", "interval", "kind", "reference", "score", "text") {
			t.Fatalf("observation keys %v", keys)
		}
		var ref map[string]json.RawMessage
		if err := json.Unmarshal(item["reference"], &ref); err != nil {
			t.Fatal(err)
		}
		if !sameKeys(mapKeys(ref), "end", "source", "start") {
			t.Fatalf("reference keys %v", mapKeys(ref))
		}
	}
	if !regexp.MustCompile(`"score":[0-9]+(\.[0-9]{1,6})?`).Match(out) {
		t.Fatalf("score is not a decimal of at most six places: %s", out)
	}
	if !bytesContain(out, []byte(`"score":0.707107`)) {
		t.Fatalf("missing rounded score: %s", out)
	}

	limited := search(t, args, env, root, `{"query":"探す","limit":2}`)
	if len(limited.Beliefs) != 1 || len(limited.Observations) != 1 {
		t.Fatalf("split %#v %#v", limited.Beliefs, limited.Observations)
	}
	if limited.Observations[0].ID != created[0].ID || limited.Beliefs[0].ID != created[1].ID {
		t.Fatalf("order obs %s belief %s", limited.Observations[0].ID, limited.Beliefs[0].ID)
	}

	for _, tc := range []struct {
		args []string
		exit int
		code string
	}{
		{[]string{"--query", "探す", "--limit", "0"}, 1, "limit_invalid"},
		{[]string{"--query", "探す", "--limit", "21"}, 1, "limit_invalid"},
		{[]string{"--query", "探す", "--limit", "1.5"}, 2, "invalid_type"},
	} {
		out, errOut, code = run(t, append(append(append([]string{}, args...), "search"), tc.args...), nil, env, root)
		assertCode(t, out, errOut, code, tc.exit, tc.code)
	}

	if err := embed.Delete(cache, embed.Key{Provider: "fixture", Model: "fixture-model", Text: itemText(0)}); err != nil {
		t.Fatal(err)
	}
	out, errOut, code = run(t, append(append([]string{}, args...), "search", "--query", "探す", "--limit", "3"), nil, env, root)
	assertCode(t, out, errOut, code, 1, "embed_cache_missing")
}

func itemText(i int) string {
	return "m" + string(rune('0'+i))
}

func mapKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func sameKeys(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, key := range got {
		seen[key]++
	}
	for _, key := range want {
		seen[key]--
		if seen[key] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func bytesContain(haystack, needle []byte) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (string(haystack) != "" && contains(string(haystack), string(needle))))
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})())
}
