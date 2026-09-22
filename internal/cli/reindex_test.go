package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"agmemx/internal/embed"
)

func TestReindexRewritesSubtreeWithoutTouchingFixtureCache(t *testing.T) {
	lay := newEmbedLayout(t, map[string][]float64{"alpha": {1, 0}, "beta": {0, 1}})
	alpha := create(t, lay.fixtureArgs(lay.root), lay.env, lay.root, "observe", observeBody("alpha"))
	beta := create(t, lay.fixtureArgs(lay.child), lay.env, lay.root, "observe", observeBody("beta"))
	before := snapshotTree(t, namespaceDir(lay.cache, "fixture", "", "fixture-model"))

	var sawBeta bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model, input, raw := readEmbedBody(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api/embed" {
			t.Errorf("ollama %s %s", r.Method, r.URL.Path)
		}
		if _, ok := r.Header["Authorization"]; ok {
			t.Errorf("ollama authorization %q", r.Header.Get("Authorization"))
		}
		if model != "nomic-embed-text" || string(raw) != `{"model":"nomic-embed-text","input":"`+input+`"}` {
			t.Errorf("body %s", raw)
		}
		vec := []float64{1, 0}
		if input == "beta" {
			sawBeta = true
			vec = []float64{0, 1}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float64{vec}})
	}))
	defer srv.Close()

	base := srv.URL + "/"
	out, errOut, code := run(t, append(lay.hostedArgs(lay.root, "ollama", "nomic-embed-text", base, ""), "reindex"), []byte(`{}`), lay.env, lay.root)
	got := assertReindex(t, out, errOut, code, "ollama", "nomic-embed-text", 2)
	if !sawBeta || got.Count != 2 {
		t.Fatalf("reindex did not walk the subtree: beta %v %#v", sawBeta, got)
	}
	sameSnap(t, before, snapshotTree(t, namespaceDir(lay.cache, "fixture", "", "fixture-model")))
	trimmed := strings.TrimRight(base, "/")
	if _, err := os.Stat(namespaceDir(lay.cache, "ollama", trimmed, "nomic-embed-text")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(namespaceDir(lay.cache, "ollama", base, "nomic-embed-text")); !os.IsNotExist(err) {
		t.Fatal("cache key kept the trailing slash")
	}
	names := childNames(t, filepath.Join(lay.cache, "agmemx"))
	for _, name := range names {
		if strings.HasPrefix(name, "stage-") {
			t.Fatalf("stage left behind: %v", names)
		}
	}

	found := search(t, lay.hostedArgs(lay.root, "ollama", "nomic-embed-text", base, ""), lay.env, lay.root, `{"query":"alpha"}`)
	seen := map[string]bool{}
	for _, item := range found.Observations {
		seen[item.ID] = true
	}
	if !seen[alpha.ID] || !seen[beta.ID] {
		t.Fatalf("search %#v", found.Observations)
	}

	alphaPath := vectorFile(lay.cache, "ollama", trimmed, "nomic-embed-text", "alpha")
	parentVec := snapshotTree(t, alphaPath)
	out, errOut, code = run(t, append(lay.hostedArgs(lay.child, "ollama", "nomic-embed-text", base, ""), "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReindex(t, out, errOut, code, "ollama", "nomic-embed-text", 1)
	sameSnap(t, parentVec, snapshotTree(t, alphaPath))
	found = search(t, lay.hostedArgs(lay.root, "ollama", "nomic-embed-text", trimmed, ""), lay.env, lay.root, `{"query":"alpha"}`)
	seen = map[string]bool{}
	for _, item := range found.Observations {
		seen[item.ID] = true
	}
	if !seen[alpha.ID] || !seen[beta.ID] {
		t.Fatalf("parent vector dropped %#v", found.Observations)
	}
}

func TestReindexFailurePreservesFixtureCache(t *testing.T) {
	for _, mode := range []string{"rejected", "timeout", "refused"} {
		t.Run(mode, func(t *testing.T) {
			lay := newEmbedLayout(t, map[string][]float64{"alpha": {1, 0}, "beta": {0, 1}})
			alpha := create(t, lay.fixtureArgs(lay.root), lay.env, lay.root, "observe", observeBody("alpha"))
			beta := create(t, lay.fixtureArgs(lay.child), lay.env, lay.root, "observe", observeBody("beta"))
			ns := namespaceDir(lay.cache, "fixture", "", "fixture-model")
			before := snapshotTree(t, ns)
			namesBefore := childNames(t, filepath.Join(lay.cache, "agmemx"))
			base, cleanup := failureServer(t, mode)
			defer cleanup()
			out, errOut, code := run(t, append(lay.hostedArgs(lay.root, "ollama", "nomic", base, ""), "reindex"), []byte(`{}`), lay.env, lay.root)
			want := "embed_unreachable"
			if mode == "rejected" {
				want = "embed_rejected"
			}
			assertReject(t, out, errOut, code, 1, want)
			sameSnap(t, before, snapshotTree(t, ns))
			if got := childNames(t, filepath.Join(lay.cache, "agmemx")); strings.Join(got, ",") != strings.Join(namesBefore, ",") {
				t.Fatalf("cache dirs %v want %v", got, namesBefore)
			}
			found := search(t, lay.fixtureArgs(lay.root), lay.env, lay.root, `{"query":"alpha"}`)
			seen := map[string]bool{}
			for _, item := range found.Observations {
				seen[item.ID] = true
			}
			if !seen[alpha.ID] || !seen[beta.ID] {
				t.Fatalf("fixture cache unusable %#v", found.Observations)
			}
		})
	}
}

func TestReindexDimensionMismatchPreservesNamespace(t *testing.T) {
	lay := newEmbedLayout(t, map[string][]float64{"alpha": {1, 0}})
	dim := 2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := []float64{1, 0}
		if dim == 3 {
			vec = []float64{1, 0, 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float64{vec}})
	}))
	defer srv.Close()
	args := lay.hostedArgs(lay.root, "ollama", "nomic", srv.URL, "")
	_ = create(t, args, lay.env, lay.root, "observe", observeBody("alpha"))
	ns := namespaceDir(lay.cache, "ollama", srv.URL, "nomic")
	before := snapshotTree(t, ns)
	dim = 3
	out, errOut, code := run(t, append(args, "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReject(t, out, errOut, code, 1, "embed_dimension_mismatch")
	sameSnap(t, before, snapshotTree(t, ns))
	vec, ok, err := embed.Get(lay.cache, embed.Key{Provider: "ollama", BaseURL: srv.URL, Model: "nomic", Text: "alpha"})
	if err != nil || !ok || len(vec) != 2 {
		t.Fatalf("stored vector %v ok %v err %v", vec, ok, err)
	}
}

func TestOpenAIReindexAuthAndUnsetKey(t *testing.T) {
	lay := newEmbedLayout(t, map[string][]float64{"alpha": {1, 0}})
	_ = create(t, lay.fixtureArgs(lay.root), lay.env, lay.root, "observe", observeBody("alpha"))
	ns := namespaceDir(lay.cache, "fixture", "", "fixture-model")
	before := snapshotTree(t, ns)

	t.Setenv("OPENAI_API_KEY", "wrong")
	t.Setenv("XAI_API_KEY", "right")
	var auth, path string
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		var err error
		raw, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"data":[{"embedding":[0.5,0.25]}]}`)
	}))
	defer srv.Close()
	args := lay.hostedArgs(lay.root, "openai", "text-embedding-3-small", srv.URL+"/", "XAI_API_KEY")
	out, errOut, code := run(t, append(args, "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReindex(t, out, errOut, code, "openai", "text-embedding-3-small", 1)
	if path != "/embeddings" || auth != "Bearer right" {
		t.Fatalf("path %s auth %s", path, auth)
	}
	if string(raw) != `{"model":"text-embedding-3-small","input":"alpha"}` {
		t.Fatalf("body %s", raw)
	}
	sameSnap(t, before, snapshotTree(t, ns))
	if _, err := os.Stat(namespaceDir(lay.cache, "openai", srv.URL, "text-embedding-3-small")); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OPENAI_API_KEY", "sk-default")
	defaultArgs := lay.hostedArgs(lay.root, "openai", "text-embedding-3-small", srv.URL, "")
	out, errOut, code = run(t, append(defaultArgs, "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReindex(t, out, errOut, code, "openai", "text-embedding-3-small", 1)
	if auth != "Bearer sk-default" {
		t.Fatalf("default auth %s", auth)
	}

	called := false
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer dead.Close()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	out, errOut, code = run(t, append(lay.hostedArgs(lay.root, "openai", "text-embedding-3-small", dead.URL, ""), "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReject(t, out, errOut, code, 1, "embed_provider_unset")
	if called {
		t.Fatal("missing key contacted the provider")
	}
	sameSnap(t, before, snapshotTree(t, ns))
}

func TestReindexRejectsUnknownFieldAndEmptySubtree(t *testing.T) {
	lay := newEmbedLayout(t, map[string][]float64{"alpha": {1, 0}})
	out, errOut, code := run(t, append(lay.fixtureArgs(lay.root), "reindex"), []byte(`{"extra":1}`), lay.env, lay.root)
	assertReject(t, out, errOut, code, 2, "unknown_field")

	out, errOut, code = run(t, append(lay.fixtureArgs(lay.root), "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReindex(t, out, errOut, code, "fixture", "fixture-model", 0)

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("empty reindex contacted the provider")
	}))
	url := srv.URL
	srv.Close()
	out, errOut, code = run(t, append(lay.hostedArgs(lay.root, "ollama", "nomic", url, ""), "reindex"), []byte(`{}`), lay.env, lay.root)
	assertReindex(t, out, errOut, code, "ollama", "nomic", 0)

	out, errOut, code = run(t, []string{"--dir", lay.root, "--embed-provider", "ollama", "reindex"}, []byte(`{}`), lay.env, lay.root)
	assertReject(t, out, errOut, code, 1, "embed_provider_unset")
}

func TestHostedObserveRejectsWithoutWriting(t *testing.T) {
	lay := newEmbedLayout(t, map[string][]float64{"alpha": {1, 0}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"embeddings":[]}`)
	}))
	defer srv.Close()
	before := objectCount(t, lay.data)
	out, errOut, code := run(t, append(lay.hostedArgs(lay.root, "ollama", "nomic", srv.URL, ""), "observe"), []byte(observeBody("alpha")), lay.env, lay.root)
	assertReject(t, out, errOut, code, 1, "embed_rejected")
	if objectCount(t, lay.data) != before {
		t.Fatal("rejected embed wrote an object")
	}
	closed := httptest.NewServer(http.NewServeMux())
	closedURL := closed.URL
	closed.Close()
	out, errOut, code = run(t, append(lay.hostedArgs(lay.root, "ollama", "nomic", closedURL, ""), "observe"), []byte(observeBody("alpha")), lay.env, lay.root)
	assertReject(t, out, errOut, code, 1, "embed_unreachable")
	if objectCount(t, lay.data) != before {
		t.Fatal("unreachable embed wrote an object")
	}
	if names := childNames(t, filepath.Join(lay.cache, "agmemx")); len(names) != 0 {
		t.Fatalf("cache left after rejection: %v", names)
	}
}

type embedLayout struct {
	data, cache, root, child, fixture string
	env                               []string
}

func newEmbedLayout(t *testing.T, vectors map[string][]float64) embedLayout {
	t.Helper()
	data := t.TempDir()
	cache := t.TempDir()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, child} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abcd"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, vectors)
	return embedLayout{
		data: data, cache: cache, root: root, child: child, fixture: fixture,
		env: []string{
			"HOME=" + data,
			"XDG_DATA_HOME=" + data,
			"XDG_CACHE_HOME=" + cache,
			"XDG_STATE_HOME=" + t.TempDir(),
		},
	}
}

func (l embedLayout) fixtureArgs(dir string) []string {
	return []string{"--dir", dir, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", l.fixture}
}

func (l embedLayout) hostedArgs(dir, provider, model, base, keyEnv string) []string {
	args := []string{"--dir", dir, "--embed-provider", provider, "--embed-model", model, "--embed-base-url", base}
	if keyEnv != "" {
		args = append(args, "--embed-api-key-env", keyEnv)
	}
	return args
}

func observeBody(text string) string {
	return `{"text":"` + text + `","reference":{"source":"a.txt","start":0,"end":1}}`
}

func namespaceDir(cache, provider, base, model string) string {
	sum := sha256.Sum256([]byte(provider + "\n" + base + "\n" + model))
	return filepath.Join(cache, "agmemx", hex.EncodeToString(sum[:]))
}

func vectorFile(cache, provider, base, model, text string) string {
	sum := sha256.Sum256([]byte(text))
	return filepath.Join(namespaceDir(cache, provider, base, model), hex.EncodeToString(sum[:])+".json")
}

type fileSnap struct {
	mode os.FileMode
	mod  time.Time
	body []byte
	dir  bool
}

func snapshotTree(t *testing.T, root string) map[string]fileSnap {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]fileSnap{}
	if !info.IsDir() {
		body, err := os.ReadFile(root)
		if err != nil {
			t.Fatal(err)
		}
		out["."] = fileSnap{mode: info.Mode(), mod: info.ModTime(), body: body}
		return out
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		meta, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snap := fileSnap{mode: meta.Mode(), mod: meta.ModTime(), dir: d.IsDir()}
		if !d.IsDir() {
			snap.body, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		out[rel] = snap
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameSnap(t *testing.T, before, after map[string]fileSnap) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("cache entries %d want %d", len(after), len(before))
	}
	for name, snap := range before {
		got, ok := after[name]
		if !ok || got.dir != snap.dir || got.mode != snap.mode || !got.mod.Equal(snap.mod) || !bytes.Equal(got.body, snap.body) {
			t.Fatalf("cache entry %s changed", name)
		}
	}
}

func childNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func failureServer(t *testing.T, mode string) (string, func()) {
	t.Helper()
	switch mode {
	case "timeout":
		prev := embed.Client
		embed.Client = &http.Client{Timeout: 100 * time.Millisecond}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(400 * time.Millisecond)
		}))
		return srv.URL, func() {
			embed.Client = prev
			srv.Close()
		}
	case "refused":
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()
		return url, func() {}
	default:
		n := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n++
			if n == 1 {
				_, _ = io.WriteString(w, `{"embeddings":[[1,0]]}`)
				return
			}
			_, _ = io.WriteString(w, `{}`)
		}))
		return srv.URL, srv.Close
	}
}

type reindexBody struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Count    int    `json:"count"`
}

func assertReindex(t *testing.T, stdout []byte, stderr string, code int, provider, model string, count int) reindexBody {
	t.Helper()
	if code != 0 || stderr != "" {
		t.Fatalf("code %d stderr %q stdout %s", code, stderr, stdout)
	}
	var got reindexBody
	if err := json.Unmarshal(stdout, &got); err != nil {
		t.Fatal(err)
	}
	if got.Provider != provider || got.Model != model || got.Count != count {
		t.Fatalf("reindex %#v", got)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(stdout, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 3 {
		t.Fatalf("keys %#v", raw)
	}
	return got
}

func assertReject(t *testing.T, stdout []byte, stderr string, code, wantExit int, wantCode string) {
	t.Helper()
	msgs := map[string]string{
		"unknown_field":            "未知のフィールドがある",
		"embed_provider_unset":     "埋め込みプロバイダまたはモデルが未設定である",
		"embed_unreachable":        "埋め込みプロバイダへ接続できない",
		"embed_rejected":           "埋め込みプロバイダがベクトルを返さなかった",
		"embed_dimension_mismatch": "埋め込みの次元が索引と一致しない",
	}
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
		t.Fatalf("stdout %s: %v", stdout, err)
	}
	if body.Error.Code != wantCode || body.Error.Message != msgs[wantCode] {
		t.Fatalf("error %#v want %s", body.Error, wantCode)
	}
}

func readEmbedBody(t *testing.T, r *http.Request) (string, string, []byte) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
		return "", "", nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var req struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}
	if err := dec.Decode(&req); err != nil {
		t.Errorf("body %s: %v", raw, err)
	}
	return req.Model, req.Input, raw
}
