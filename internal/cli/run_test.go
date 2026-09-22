package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agmemx/internal/cli"
)

func TestInitCreatesEmptyDomainSeparateFromObjects(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	out, errOut, code := run(t, []string{"--dir", dir, "init"}, []byte("{}\n"), envWithData(data), dir)
	if code != 0 {
		t.Fatalf("code %d out %s err %s", code, out, errOut)
	}
	if errOut != "" {
		t.Fatalf("stderr %q", errOut)
	}
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	wantDomain := canonical(t, dir)
	if len(body) != 1 || body["domain"] != wantDomain {
		t.Fatalf("body %#v want %s", body, wantDomain)
	}
	objects := filepath.Join(data, "agmemx", "objects")
	entries, err := os.ReadDir(objects)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("objects not empty: %v", entries)
	}
	index := indexPath(data, wantDomain)
	raw, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Domain string   `json:"domain"`
		IDs    []string `json:"ids"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Domain != wantDomain || len(decoded.IDs) != 0 {
		t.Fatalf("index %#v", decoded)
	}
	if filepath.Dir(index) == objects {
		t.Fatal("domain index lives in objects")
	}

	out2, _, code2 := run(t, []string{"--dir", dir, "init"}, []byte("{}"), envWithData(data), dir)
	if code2 != 0 {
		t.Fatalf("second code %d %s", code2, out2)
	}
	raw2, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, raw2) {
		t.Fatalf("index changed\n%s\n%s", raw, raw2)
	}
}

func TestDefaultDataHomeAndEmptyOverride(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "XDG_DATA_HOME=", "XDG_CACHE_HOME=", "XDG_STATE_HOME="}
	_, errOut, code := run(t, []string{"--dir", dir, "init"}, []byte("{}"), env, dir)
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errOut)
	}
	if _, err := os.Stat(indexPath(filepath.Join(home, ".local", "share"), canonical(t, dir))); err != nil {
		t.Fatal(err)
	}
}

func TestRelativeXDGRejectedBeforeDirectoryAndBody(t *testing.T) {
	data := t.TempDir()
	out, errOut, code := run(t, []string{"--dir", "/no/such/agmemx-dir", "init"}, []byte("{"), []string{
		"HOME=" + t.TempDir(),
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=relative-cache",
		"XDG_STATE_HOME=" + t.TempDir(),
	}, t.TempDir())
	assertCode(t, out, errOut, code, 1, "xdg_relative")
	if _, err := os.Stat(filepath.Join(data, "agmemx")); !os.IsNotExist(err) {
		t.Fatalf("store created despite relative cache: %v", err)
	}

	out, errOut, code = run(t, []string{"init"}, []byte("{}"), []string{
		"HOME=" + t.TempDir(),
		"XDG_DATA_HOME=rel",
		"XDG_CACHE_HOME=" + t.TempDir(),
		"XDG_STATE_HOME=" + t.TempDir(),
	}, t.TempDir())
	assertCode(t, out, errOut, code, 1, "xdg_relative")

	out, errOut, code = run(t, []string{"init"}, []byte("{}"), []string{
		"HOME=" + t.TempDir(),
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + t.TempDir(),
		"XDG_STATE_HOME=rel",
	}, t.TempDir())
	assertCode(t, out, errOut, code, 1, "xdg_relative")
}

func TestDirResolution(t *testing.T) {
	data := t.TempDir()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := run(t, []string{"--dir", "link", "init"}, []byte("{}"), envWithData(data), base)
	if code != 0 {
		t.Fatalf("symlink code %d %s %s", code, out, errOut)
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if body.Domain != canonical(t, real) {
		t.Fatalf("domain %s want %s", body.Domain, canonical(t, real))
	}

	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code = run(t, []string{"--dir", file, "init"}, []byte("{}"), envWithData(data), base)
	assertCode(t, out, errOut, code, 1, "dir_not_directory")
	out, errOut, code = run(t, []string{"--dir", filepath.Join(base, "missing"), "init"}, []byte("{}"), envWithData(data), base)
	assertCode(t, out, errOut, code, 1, "dir_not_found")
	out, errOut, code = run(t, []string{"--dir", "/", "init"}, []byte("{}"), envWithData(data), base)
	assertCode(t, out, errOut, code, 1, "dir_is_root")
	entries, err := os.ReadDir(filepath.Join(data, "agmemx", "domains"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("domains after rejections: %d", len(entries))
	}
}

func TestInitRejectsMalformedInputWithoutWriting(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	cases := []struct {
		args []string
		in   string
		exit int
		code string
	}{
		{[]string{"init"}, `{"extra":1}`, 2, "unknown_field"},
		{[]string{"init"}, `{`, 2, "invalid_json"},
		{[]string{"init"}, `[]`, 2, "invalid_json"},
		{[]string{"init"}, `{}{}`, 2, "invalid_json"},
		{[]string{"nope"}, `{}`, 2, "invalid_command"},
		{[]string{"--unknown", "init"}, `{}`, 2, "invalid_flag"},
		{[]string{"--embed-provider", "other", "init"}, `{}`, 2, "invalid_flag"},
	}
	for _, tc := range cases {
		out, errOut, code := run(t, tc.args, []byte(tc.in), envWithData(data), dir)
		assertCode(t, out, errOut, code, tc.exit, tc.code)
	}
	if _, err := os.Stat(filepath.Join(data, "agmemx")); !os.IsNotExist(err) {
		t.Fatalf("store written on rejection: %v", err)
	}
}

func TestBinaryInitUsesStartupDirectory(t *testing.T) {
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "agmemx")
	build := exec.Command("go", "build", "-o", bin, "./cmd/agmemx")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	dir := t.TempDir()
	data := t.TempDir()
	cmd := exec.Command(bin, "init")
	cmd.Dir = dir
	cmd.Env = envWithData(data)
	cmd.Stdin = strings.NewReader("{}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v stdout %s stderr %s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.String())
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Domain != canonical(t, dir) {
		t.Fatalf("domain %s", body.Domain)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func TestStartupCwdIsUsedWhenDirOmitted(t *testing.T) {
	data := t.TempDir()
	cwd := t.TempDir()
	out, _, code := run(t, []string{"init"}, []byte("{}"), envWithData(data), cwd)
	if code != 0 {
		t.Fatalf("code %d %s", code, out)
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if body.Domain != canonical(t, cwd) {
		t.Fatalf("domain %s cwd %s", body.Domain, canonical(t, cwd))
	}
}

func run(t *testing.T, args []string, in []byte, env []string, cwd string) ([]byte, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, bytes.NewReader(in), &stdout, &stderr, env, cwd)
	return stdout.Bytes(), stderr.String(), code
}

func envWithData(data string) []string {
	return []string{
		"HOME=" + data,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + data,
		"XDG_STATE_HOME=" + data,
	}
}

func canonical(t *testing.T, path string) string {
	t.Helper()
	eval, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(eval)
}

func indexPath(dataHome, domain string) string {
	sum := sha256.Sum256([]byte(domain))
	return filepath.Join(dataHome, "agmemx", "domains", hex.EncodeToString(sum[:])+".json")
}

func contractMessage(code string) string {
	return map[string]string{
		"xdg_relative":                 "XDG のパスが相対パスである",
		"dir_not_found":                "解決ディレクトリが存在しない",
		"dir_not_directory":            "解決先がディレクトリではない",
		"dir_is_root":                  "解決ディレクトリがファイルシステムのルートである",
		"unknown_field":                "未知のフィールドがある",
		"invalid_json":                 "入力は1つの JSON オブジェクトである",
		"invalid_command":              "未知のコマンドである",
		"invalid_flag":                 "未知のフラグがある",
		"invalid_type":                 "フィールドの型が契約と違う",
		"text_empty":                   "本文が空である",
		"observation_reason_forbidden": "観測の理由は受け付けない",
		"reason_not_found":             "理由の対象が存在しない",
		"cross_domain_next":            "次発話は同一ドメインに限る",
		"reason_kind_invalid":          "信念の理由種別が契約にない",
		"reference_escapes_domain":     "Reference の源が解決ディレクトリの外である",
		"reference_not_found":          "Reference の源ファイルが存在しない",
		"reference_encoding":           "Reference の源が UTF-8 ではない",
		"reference_span_invalid":       "Reference の範囲が源の外である",
		"embed_provider_unset":         "埋め込みプロバイダまたはモデルが未設定である",
		"embed_fixture_invalid":        "フィクスチャのベクトル定義が契約と違う",
		"embed_dimension_mismatch":     "埋め込みの次元が索引と一致しない",
		"embed_cache_missing":          "検索対象の埋め込みキャッシュが欠けている",
		"limit_invalid":                "検索上限が 1 以上 20 以下ではない",
	}[code]
}

func assertCode(t *testing.T, stdout []byte, stderr string, code, wantExit int, wantCode string) {
	t.Helper()
	if code != wantExit {
		t.Fatalf("exit %d want %d stdout %s", code, wantExit, stdout)
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
	if body.Error.Code != wantCode || body.Error.Message != contractMessage(wantCode) {
		t.Fatalf("error %#v want %s", body.Error, wantCode)
	}
	var raw map[string]any
	if err := json.Unmarshal(stdout, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 {
		t.Fatalf("extra keys %#v", raw)
	}
}
