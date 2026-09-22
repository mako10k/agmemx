package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestDomainAttachMovesIndexOnly(t *testing.T) {
	data := t.TempDir()
	cache := t.TempDir()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "a.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	link := filepath.Join(t.TempDir(), "via")
	if err := os.Symlink(dest, link); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{
		"観測した":   {1, 0},
		"残す":     {0, 1},
		"次も移す":   {0.2, 0.8},
		"移動先の理由": {0.5, 0.5},
		"移動先の対象": {0.4, 0.6},
		"矛盾を探す":  {1, 0},
	})
	env := []string{
		"HOME=" + data,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + cache,
		"XDG_STATE_HOME=" + t.TempDir(),
	}
	childArgs := []string{"--dir", child, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	moved := create(t, childArgs, env, root, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":0,"end":2}}`)
	stay := create(t, childArgs, env, root, "observe", `{"text":"残す","reference":{"source":"a.txt","start":0,"end":1}}`)
	objectPath := filepath.Join(data, "agmemx", "objects", moved.ID+".json")
	before, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	stayBefore, err := os.ReadFile(filepath.Join(data, "agmemx", "objects", stay.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	cacheBefore := snapshotFiles(t, cache)
	if objectCount(t, data) != 2 {
		t.Fatalf("objects %d", objectCount(t, data))
	}

	out, errOut, code := run(t, []string{"--dir", root, "domain-attach"}, mustJSON(t, map[string]string{
		"id":     moved.ID,
		"domain": link,
	}), env, root)
	if code != 0 {
		t.Fatalf("attach %d %s %s", code, out, errOut)
	}
	if errOut != "" {
		t.Fatalf("stderr %q", errOut)
	}
	var got struct {
		ID            string `json:"id"`
		Domain        string `json:"domain"`
		ContentSHA256 string `json:"content_sha256"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	wantDomain := canonical(t, dest)
	if got.ID != moved.ID || got.Domain != wantDomain || got.ContentSHA256 != moved.ContentSHA256 {
		t.Fatalf("attach %#v want domain %s hash %s", got, wantDomain, moved.ContentSHA256)
	}
	if got.Domain == link {
		t.Fatal("attach returned the symlink path")
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 3 {
		t.Fatalf("keys %#v", raw)
	}
	after, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("object bytes changed\n%s\n%s", before, after)
	}
	if objectCount(t, data) != 2 {
		t.Fatal("attach copied the object")
	}
	if _, err := os.Stat(indexPath(data, link)); !os.IsNotExist(err) {
		t.Fatal("index was stored under the symlink path")
	}
	if !reflect.DeepEqual(cacheBefore, snapshotFiles(t, cache)) {
		t.Fatal("attach rewrote the embedding cache")
	}

	destIDs := indexIDs(t, data, wantDomain)
	if len(destIDs) != 1 || destIDs[0] != moved.ID {
		t.Fatalf("dest index %v", destIDs)
	}
	childIDs := indexIDs(t, data, moved.Domain)
	if len(childIDs) != 1 || childIDs[0] != stay.ID {
		t.Fatalf("source index %v", childIDs)
	}

	rootArgs := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	destArgs := []string{"--dir", dest, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	fromRoot := placedSearch(t, rootArgs, env, root)
	if placedHas(fromRoot.Observations, moved.ID) || !placedDomain(fromRoot.Observations, stay.ID, stay.Domain) {
		t.Fatalf("root search %#v", fromRoot.Observations)
	}
	fromChild := placedSearch(t, childArgs, env, root)
	if placedHas(fromChild.Observations, moved.ID) || !placedHas(fromChild.Observations, stay.ID) {
		t.Fatalf("child search %#v", fromChild.Observations)
	}
	fromDest := placedSearch(t, destArgs, env, root)
	if !placedDomain(fromDest.Observations, moved.ID, wantDomain) {
		t.Fatalf("dest search %#v", fromDest.Observations)
	}

	next := create(t, childArgs, env, root, "observe", `{"text":"次も移す","reference":{"source":"a.txt","start":1,"end":3}}`)
	nextBefore, err := os.ReadFile(filepath.Join(data, "agmemx", "objects", next.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	out, errOut, code = run(t, []string{"--dir", child, "domain-attach"}, mustJSON(t, map[string]string{
		"id":     next.ID,
		"domain": wantDomain,
	}), env, root)
	if code != 0 || errOut != "" {
		t.Fatalf("second attach %d %s %s", code, out, errOut)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.ContentSHA256 != next.ContentSHA256 || got.Domain != wantDomain {
		t.Fatalf("second %#v", got)
	}
	nextAfter, err := os.ReadFile(filepath.Join(data, "agmemx", "objects", next.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(nextBefore, nextAfter) || !bytes.Equal(before, mustRead(t, objectPath)) {
		t.Fatal("second attach rewrote an object")
	}
	gotIDs := indexIDs(t, data, wantDomain)
	wantIDs := []string{moved.ID, next.ID}
	sort.Strings(wantIDs)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("dest ids %v want %v", gotIDs, wantIDs)
	}
	childIDs = indexIDs(t, data, stay.Domain)
	if len(childIDs) != 1 || childIDs[0] != stay.ID {
		t.Fatalf("source index after second move %v", childIDs)
	}
	if !bytes.Equal(stayBefore, mustRead(t, filepath.Join(data, "agmemx", "objects", stay.ID+".json"))) {
		t.Fatal("stay object changed")
	}

	beforeBelieve := objectCount(t, data)
	rejected, errOut, code := run(t, append(rootArgs, "believe"), []byte(`{"text":"移動先の理由","reason":{"kind":"observation","id":"`+moved.ID+`"}}`), env, root)
	assertAttachReject(t, rejected, errOut, code, 1, "reason_not_found", "理由の対象が存在しない")
	aboutRejected, errOut, code := run(t, append(rootArgs, "believe"), []byte(`{"text":"移動先の対象","about":["`+moved.ID+`"]}`), env, root)
	assertAttachReject(t, aboutRejected, errOut, code, 1, "object_not_found", "対象が存在しない")
	if objectCount(t, data) != beforeBelieve {
		t.Fatal("rejected believe wrote an object")
	}
	reason := create(t, destArgs, env, root, "believe", `{"text":"移動先の理由","reason":{"kind":"observation","id":"`+moved.ID+`"}}`)
	about := create(t, destArgs, env, root, "believe", `{"text":"移動先の対象","about":["`+moved.ID+`"]}`)
	if reason.Domain != wantDomain || about.Domain != wantDomain {
		t.Fatalf("reason %s about %s", reason.Domain, about.Domain)
	}
	if !bytes.Equal(before, mustRead(t, objectPath)) {
		t.Fatal("object bytes changed after reason checks")
	}
}

func TestDomainAttachRejectionsDoNotMove(t *testing.T) {
	data := t.TempDir()
	root := t.TempDir()
	other := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "a.txt"), []byte("あいうえ"), 0o644); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeFixture(t, fixture, map[string][]float64{"観測した": {1, 0}})
	env := []string{
		"HOME=" + data,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + t.TempDir(),
		"XDG_STATE_HOME=" + t.TempDir(),
	}
	args := []string{"--dir", root, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}
	obs := create(t, args, env, root, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":0,"end":1}}`)
	otherObs := create(t, []string{"--dir", other, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}, env, root, "observe", `{"text":"観測した","reference":{"source":"a.txt","start":0,"end":1}}`)
	wantDomain := canonical(t, dest)
	rel, err := filepath.Rel(filepath.Dir(dest), dest)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		body    any
		code    string
		exit    int
		message string
		cwd     string
	}{
		{name: "unknown", body: map[string]any{"id": obs.ID, "domain": wantDomain, "extra": true}, code: "unknown_field", exit: 2, message: "未知のフィールドがある"},
		{name: "missing id", body: map[string]any{"domain": wantDomain}, code: "missing_field", exit: 2, message: "必須フィールドがない"},
		{name: "missing domain", body: map[string]any{"id": obs.ID}, code: "missing_field", exit: 2, message: "必須フィールドがない"},
		{name: "id type", body: map[string]any{"id": 1, "domain": wantDomain}, code: "invalid_type", exit: 2, message: "フィールドの型が契約と違う"},
		{name: "domain type", body: map[string]any{"id": obs.ID, "domain": 1}, code: "invalid_type", exit: 2, message: "フィールドの型が契約と違う"},
		{name: "relative", body: map[string]any{"id": obs.ID, "domain": rel}, code: "domain_invalid", exit: 1, message: "付け替え先が正規化できるディレクトリではない", cwd: filepath.Dir(dest)},
		{name: "missing path", body: map[string]any{"id": obs.ID, "domain": filepath.Join(dest, "missing")}, code: "domain_invalid", exit: 1, message: "付け替え先が正規化できるディレクトリではない"},
		{name: "file", body: map[string]any{"id": obs.ID, "domain": filePath}, code: "domain_invalid", exit: 1, message: "付け替え先が正規化できるディレクトリではない"},
		{name: "root", body: map[string]any{"id": obs.ID, "domain": "/"}, code: "domain_invalid", exit: 1, message: "付け替え先が正規化できるディレクトリではない"},
		{name: "bad id and bad domain", body: map[string]any{"id": "not-there", "domain": "relative"}, code: "domain_invalid", exit: 1, message: "付け替え先が正規化できるディレクトリではない"},
		{name: "unknown id", body: map[string]any{"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "domain": wantDomain}, code: "object_not_found", exit: 1, message: "対象が存在しない"},
		{name: "outside subtree", body: map[string]any{"id": otherObs.ID, "domain": wantDomain}, code: "object_not_found", exit: 1, message: "対象が存在しない"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := snapshotFiles(t, data)
			cwd := root
			if tc.cwd != "" {
				cwd = tc.cwd
			}
			out, errOut, code := run(t, []string{"--dir", root, "domain-attach"}, mustJSON(t, tc.body), env, cwd)
			assertAttachReject(t, out, errOut, code, tc.exit, tc.code, tc.message)
			if !reflect.DeepEqual(before, snapshotFiles(t, data)) {
				t.Fatal("rejection changed the store")
			}
		})
	}
	if _, err := os.Stat(indexPath(data, wantDomain)); !os.IsNotExist(err) {
		t.Fatal("rejection created the destination index")
	}
}

type placedHit struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

type placedBody struct {
	Beliefs      []placedHit `json:"beliefs"`
	Observations []placedHit `json:"observations"`
}

func placedSearch(t *testing.T, args []string, env []string, cwd string) placedBody {
	t.Helper()
	out, errOut, code := run(t, append(append([]string{}, args...), "search"), []byte(`{"query":"矛盾を探す"}`), env, cwd)
	if code != 0 || errOut != "" {
		t.Fatalf("search %d %s %s", code, out, errOut)
	}
	var found placedBody
	if err := json.Unmarshal(out, &found); err != nil {
		t.Fatal(err)
	}
	return found
}

func placedHas(hits []placedHit, id string) bool {
	for _, hit := range hits {
		if hit.ID == id {
			return true
		}
	}
	return false
}

func placedDomain(hits []placedHit, id, domain string) bool {
	for _, hit := range hits {
		if hit.ID == id {
			return hit.Domain == domain
		}
	}
	return false
}

func indexIDs(t *testing.T, dataHome, domain string) []string {
	t.Helper()
	raw, err := os.ReadFile(indexPath(dataHome, domain))
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
	if decoded.Domain != domain {
		t.Fatalf("index domain %s want %s", decoded.Domain, domain)
	}
	if decoded.IDs == nil {
		t.Fatal("ids must be an array")
	}
	return decoded.IDs
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = string(body)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return out
}

func assertAttachReject(t *testing.T, stdout []byte, stderr string, code, wantExit int, wantCode, wantMessage string) {
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
		t.Fatalf("stdout %s: %v", stdout, err)
	}
	if body.Error.Code != wantCode || body.Error.Message != wantMessage {
		t.Fatalf("error %#v want %s %s", body.Error, wantCode, wantMessage)
	}
	var raw map[string]any
	if err := json.Unmarshal(stdout, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 {
		t.Fatalf("extra keys %#v", raw)
	}
}
