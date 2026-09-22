package cli

import (
	"encoding/json"
	"io"

	"agmemx/internal/store"
	"agmemx/internal/xdg"
)

func handleRelate(stdout, stderr io.Writer, roots xdg.Roots, _ options, _ string, body map[string]json.RawMessage) int {
	if rej := unknownKeys(body, "kind", "from", "to"); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	kind, rej := jsonString(body["kind"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	if kind != "next" {
		writeReject(stdout, reject("relation_kind_invalid", 1))
		return 1
	}
	from, rej := relateEndpoint(body["from"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	to, rej := relateEndpoint(body["to"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	if !idHex(from) || !idHex(to) || from == to {
		writeReject(stdout, reject("relation_endpoints_invalid", 1))
		return 1
	}
	fromDomain, fromOK, err := store.ObjectDomain(roots.Data, from)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	toDomain, toOK, err := store.ObjectDomain(roots.Data, to)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	if !fromOK || !toOK {
		writeReject(stdout, reject("relation_endpoints_invalid", 1))
		return 1
	}
	if fromDomain != toDomain {
		writeReject(stdout, reject("cross_domain_next", 1))
		return 1
	}
	if err := store.PutNext(roots.Data, fromDomain, from, to); err != nil {
		fmtErr(stderr, err)
		return 2
	}
	writeOK(stdout, struct {
		Kind   string `json:"kind"`
		From   string `json:"from"`
		To     string `json:"to"`
		Domain string `json:"domain"`
	}{Kind: "next", From: from, To: to, Domain: fromDomain})
	return 0
}

func relateEndpoint(raw json.RawMessage) (string, *rejection) {
	if len(raw) == 0 {
		return "", reject("relation_endpoints_invalid", 1)
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return "", reject("invalid_type", 2)
	}
	if id == "" {
		return "", reject("relation_endpoints_invalid", 1)
	}
	return id, nil
}

func idHex(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}
