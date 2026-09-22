package cli

import (
	"encoding/json"
	"io"
	"path/filepath"

	"agmemx/internal/domain"
	"agmemx/internal/record"
	"agmemx/internal/store"
	"agmemx/internal/xdg"
)

func handleDomainAttach(stdout, stderr io.Writer, roots xdg.Roots, _ options, resolved string, body map[string]json.RawMessage) int {
	if rej := unknownKeys(body, "id", "domain"); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	id, rej := jsonString(body["id"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	rawDomain, rej := jsonString(body["domain"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	target, ok := canonicalAttachDomain(rawDomain)
	if !ok {
		writeReject(stdout, reject("domain_invalid", 1))
		return 1
	}
	members, err := store.SubtreeMembership(roots.Data, resolved)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	from, ok := members[id]
	if !ok {
		writeReject(stdout, reject("object_not_found", 1))
		return 1
	}
	obj, err := loadObject(roots.Data, id)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	sum, err := record.ContentSHA256(obj)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	if err := store.Relocate(roots.Data, from, target, id); err != nil {
		fmtErr(stderr, err)
		return 2
	}
	writeOK(stdout, struct {
		ID            string `json:"id"`
		Domain        string `json:"domain"`
		ContentSHA256 string `json:"content_sha256"`
	}{ID: id, Domain: target, ContentSHA256: sum})
	return 0
}

func canonicalAttachDomain(raw string) (string, bool) {
	if !filepath.IsAbs(raw) {
		return "", false
	}
	resolved, kind := domain.Resolve("", raw)
	if kind != domain.OK {
		return "", false
	}
	return resolved, true
}
