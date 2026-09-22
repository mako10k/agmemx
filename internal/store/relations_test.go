package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPutNextLeavesObjectBodiesUntouched(t *testing.T) {
	data := t.TempDir()
	domain := "/mem/domain"
	from := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	to := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	body := []byte(`{"id":"` + from + `","kind":"observation","domain":"` + domain + `"}` + "\n")
	if err := Commit(data, domain, from, body); err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(LayoutOf(data).Objects, from+".json")
	before, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := ObjectDomain(data, from)
	if err != nil || !ok || got != domain {
		t.Fatalf("domain %q ok %v err %v", got, ok, err)
	}
	missing, ok, err := ObjectDomain(data, to)
	if err != nil || ok || missing != "" {
		t.Fatalf("missing domain %q ok %v err %v", missing, ok, err)
	}

	if err := PutNext(data, domain, from, to); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("object rewritten\n%s\n%s", before, after)
	}
	edges, err := LoadNext(data, domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 || edges[0].Kind != "next" || edges[0].From != from || edges[0].To != to {
		t.Fatalf("edges %#v", edges)
	}
	raw, err := os.ReadFile(RelationPath(data, domain))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"domain":"/mem/domain","next":[{"kind":"next","from":"` + from + `","to":"` + to + `"}]}` + "\n"
	if string(raw) != want {
		t.Fatalf("relation file %q", raw)
	}
	if filepath.Dir(RelationPath(data, domain)) == LayoutOf(data).Objects {
		t.Fatal("relations live in objects")
	}

	if err := PutNext(data, domain, from, to); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(RelationPath(data, domain))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatalf("duplicate edge rewrote %q", again)
	}
	other := "cccccccccccccccccccccccccccccccc"
	if err := PutNext(data, domain, to, other); err != nil {
		t.Fatal(err)
	}
	edges, err = LoadNext(data, domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 || edges[1].From != to || edges[1].To != other {
		t.Fatalf("edges %#v", edges)
	}
	if _, err := os.Stat(filepath.Join(LayoutOf(data).Objects, to+".json")); !os.IsNotExist(err) {
		t.Fatalf("endpoint object created: %v", err)
	}
}

func TestPutNextRejectsIdentityMismatch(t *testing.T) {
	data := t.TempDir()
	domain := "/mem/domain"
	path := RelationPath(data, domain)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"domain":"/other","next":[]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PutNext(data, domain, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("expected identity mismatch")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"domain":"/other","next":[]}`+"\n" {
		t.Fatalf("mismatch rewrote %q", raw)
	}
}

func TestObjectDomainCorrupt(t *testing.T) {
	data := t.TempDir()
	id := "dddddddddddddddddddddddddddddddd"
	dir := LayoutOf(data).Objects
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ObjectDomain(data, id); err == nil {
		t.Fatal("expected corrupt object error")
	}
}
