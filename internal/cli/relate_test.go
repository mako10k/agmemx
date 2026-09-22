package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRelateSameDomainWithoutEmbedProvider(t *testing.T) {
	data := t.TempDir()
	home := t.TempDir()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	other := t.TempDir()
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "b.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{"発話の記録": {1, 0}})
	env := []string{
		"HOME=" + home,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + t.TempDir(),
		"XDG_STATE_HOME=" + t.TempDir(),
	}
	args := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	childArgs := []string{"--dir", child, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	first := create(t, args, env, root, "observe", `{"text":"発話の記録","reference":{"source":"a.txt","start":0,"end":1}}`)
	second := create(t, args, env, root, "observe", `{"text":"発話の記録","reference":{"source":"a.txt","start":1,"end":2}}`)
	third := create(t, childArgs, env, root, "observe", `{"text":"発話の記録","reference":{"source":"b.txt","start":0,"end":1}}`)
	fourth := create(t, childArgs, env, root, "observe", `{"text":"発話の記録","reference":{"source":"b.txt","start":1,"end":2}}`)
	if first.Domain != canonical(t, root) || third.Domain != canonical(t, child) || first.Domain == third.Domain {
		t.Fatalf("domains %s %s", first.Domain, third.Domain)
	}
	objects := snapshotObjects(t, data)

	rejected := []struct {
		in   string
		exit int
		code string
		msg  string
	}{
		{`{"kind":"next","from":"` + first.ID + `","to":"` + second.ID + `","extra":1}`, 2, "unknown_field", "未知のフィールドがある"},
		{`{"from":"` + first.ID + `","to":"` + second.ID + `"}`, 2, "missing_field", "必須フィールドがない"},
		{`{"kind":"prev","from":"` + first.ID + `","to":"` + second.ID + `"}`, 1, "relation_kind_invalid", "順序の種別が契約にない"},
		{`{"kind":1,"from":"` + first.ID + `","to":"` + second.ID + `"}`, 2, "invalid_type", "フィールドの型が契約と違う"},
		{`{"kind":"next","from":"` + first.ID + `"}`, 1, "relation_endpoints_invalid", "順序の両端が使えない"},
		{`{"kind":"next","from":"","to":"` + second.ID + `"}`, 1, "relation_endpoints_invalid", "順序の両端が使えない"},
		{`{"kind":"next","from":"` + first.ID + `","to":"` + first.ID + `"}`, 1, "relation_endpoints_invalid", "順序の両端が使えない"},
		{`{"kind":"next","from":"` + first.ID + `","to":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`, 1, "relation_endpoints_invalid", "順序の両端が使えない"},
		{`{"kind":"next","from":"` + first.ID + `","to":1}`, 2, "invalid_type", "フィールドの型が契約と違う"},
		{`{"kind":"next","from":"` + first.ID + `","to":"` + third.ID + `"}`, 1, "cross_domain_next", "次発話は同一ドメインに限る"},
	}
	for _, tc := range rejected {
		out, errOut, code := run(t, []string{"relate"}, []byte(tc.in), env, other)
		assertRelateReject(t, out, errOut, code, tc.exit, tc.code, tc.msg)
	}
	if _, err := os.Stat(filepath.Join(data, "agmemx", "relations")); !os.IsNotExist(err) {
		t.Fatalf("rejection created relations: %v", err)
	}
	if diff := objectDiff(objects, snapshotObjects(t, data)); diff != "" {
		t.Fatal(diff)
	}

	out, errOut, code := run(t, []string{"--dir", other, "relate"}, []byte(`{"kind":"next","from":"`+first.ID+`","to":"`+second.ID+`"}`), env, other)
	if code != 0 || errOut != "" {
		t.Fatalf("code %d stderr %q stdout %s", code, errOut, out)
	}
	want := `{"kind":"next","from":"` + first.ID + `","to":"` + second.ID + `","domain":"` + first.Domain + `"}` + "\n"
	if string(out) != want {
		t.Fatalf("stdout %q", out)
	}
	out, errOut, code = run(t, []string{"--dir", root, "relate"}, []byte(`{"kind":"next","from":"`+third.ID+`","to":"`+fourth.ID+`"}`), env, root)
	if code != 0 || errOut != "" {
		t.Fatalf("child code %d stderr %q stdout %s", code, errOut, out)
	}
	wantChild := `{"kind":"next","from":"` + third.ID + `","to":"` + fourth.ID + `","domain":"` + third.Domain + `"}` + "\n"
	if string(out) != wantChild {
		t.Fatalf("child stdout %q", out)
	}
	if diff := objectDiff(objects, snapshotObjects(t, data)); diff != "" {
		t.Fatal(diff)
	}
	assertNextFile(t, data, first.Domain, []nextEdge{{Kind: "next", From: first.ID, To: second.ID}})
	assertNextFile(t, data, third.Domain, []nextEdge{{Kind: "next", From: third.ID, To: fourth.ID}})

	out, errOut, code = run(t, []string{"relate"}, []byte(`{"kind":"next","from":"`+second.ID+`","to":"`+third.ID+`"}`), env, other)
	assertRelateReject(t, out, errOut, code, 1, "cross_domain_next", "次発話は同一ドメインに限る")
	assertNextFile(t, data, first.Domain, []nextEdge{{Kind: "next", From: first.ID, To: second.ID}})
	if _, err := os.Stat(relationFile(data, filepath.Join(other))); !os.IsNotExist(err) {
		t.Fatalf("unrelated dir got a relation file: %v", err)
	}
}

type nextEdge struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

func assertRelateReject(t *testing.T, stdout []byte, stderr string, code, wantExit int, wantCode, wantMessage string) {
	t.Helper()
	if code != wantExit {
		t.Fatalf("exit %d want %d stdout %s stderr %s", code, wantExit, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr %q", stderr)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(stdout, &raw); err != nil {
		t.Fatalf("stdout %s: %v", stdout, err)
	}
	if len(raw) != 1 || raw["error"] == nil {
		t.Fatalf("body %s", stdout)
	}
	var errFields map[string]any
	if err := json.Unmarshal(raw["error"], &errFields); err != nil {
		t.Fatal(err)
	}
	if len(errFields) != 2 {
		t.Fatalf("error keys %#v", errFields)
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
	if body.Error.Code != wantCode || body.Error.Message != wantMessage {
		t.Fatalf("error %#v want %s %s", body.Error, wantCode, wantMessage)
	}
}

func snapshotObjects(t *testing.T, data string) map[string][]byte {
	t.Helper()
	dir := filepath.Join(data, "agmemx", "objects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = raw
	}
	return out
}

func objectDiff(before, after map[string][]byte) string {
	if len(before) != len(after) {
		return "object count changed"
	}
	for name, raw := range before {
		if !bytes.Equal(raw, after[name]) {
			return "object " + name + " rewritten"
		}
	}
	return ""
}

func relationFile(data, domain string) string {
	sum := sha256.Sum256([]byte(domain))
	return filepath.Join(data, "agmemx", "relations", hex.EncodeToString(sum[:])+".json")
}

func assertNextFile(t *testing.T, data, domain string, want []nextEdge) {
	t.Helper()
	raw, err := os.ReadFile(relationFile(data, domain))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Domain string     `json:"domain"`
		Next   []nextEdge `json:"next"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Domain != domain || len(decoded.Next) != len(want) {
		t.Fatalf("file %#v want %#v", decoded, want)
	}
	for i := range want {
		if decoded.Next[i] != want[i] {
			t.Fatalf("edge %#v want %#v", decoded.Next[i], want[i])
		}
	}
	if filepath.Dir(relationFile(data, domain)) == filepath.Join(data, "agmemx", "objects") {
		t.Fatal("relation stored in objects")
	}
}
