package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"agmemx/internal/embed"
	"agmemx/internal/record"
	"agmemx/internal/search"
	"agmemx/internal/store"
	"agmemx/internal/xdg"
)

func handleObserve(stdout, stderr io.Writer, roots xdg.Roots, opts options, resolved string, body map[string]json.RawMessage) int {
	if rej := unknownKeys(body, "text", "reference", "interval", "reason"); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	if _, ok := body["reason"]; ok {
		writeReject(stdout, reject("observation_reason_forbidden", 1))
		return 1
	}
	text, rej := requiredText(body, "text")
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	ref, rej := parseReference(resolved, body["reference"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	interval, rej := parseInterval(body, "interval")
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	obj := record.Object{Kind: "observation", Domain: resolved, Text: text, Interval: interval, Reference: &ref}
	return commit(stdout, stderr, roots, opts, obj)
}

func handleBelieve(stdout, stderr io.Writer, roots xdg.Roots, opts options, resolved string, body map[string]json.RawMessage) int {
	if rej := unknownKeys(body, "text", "reason", "about", "interval"); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	text, rej := requiredText(body, "text")
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	interval, rej := parseInterval(body, "interval")
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	reason, rej := parseReason(roots.Data, resolved, body)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	about, rej := parseAbout(roots.Data, resolved, body)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	obj := record.Object{Kind: "belief", Domain: resolved, Text: text, Interval: interval, Reason: reason, About: about}
	return commit(stdout, stderr, roots, opts, obj)
}

func handleSearch(stdout, stderr io.Writer, roots xdg.Roots, opts options, resolved string, body map[string]json.RawMessage) int {
	if rej := unknownKeys(body, "query", "limit"); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	query, rej := requiredText(body, "query")
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	limit, rej := parseLimit(body["limit"])
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	queryVec, rej := embedText(roots.Cache, opts, query)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	ids, err := store.IDsInSubtree(roots.Data, resolved)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	members, err := store.SubtreeMembership(roots.Data, resolved)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	type loaded struct {
		obj   record.Object
		score float64
	}
	var hits []loaded
	for _, id := range ids {
		obj, err := loadObject(roots.Data, id)
		if err != nil {
			fmtErr(stderr, err)
			return 2
		}
		if dom, ok := members[id]; ok {
			obj.Domain = dom
		}
		vec, ok, err := embed.Get(roots.Cache, cacheKey(opts, obj.Text))
		if err != nil {
			fmtErr(stderr, err)
			return 2
		}
		if !ok {
			writeReject(stdout, reject("embed_cache_missing", 1))
			return 1
		}
		if len(vec) != len(queryVec) {
			writeReject(stdout, reject("embed_dimension_mismatch", 1))
			return 1
		}
		hits = append(hits, loaded{obj: obj, score: search.Round6(search.Cosine(queryVec, vec))})
	}
	ranked := make([]search.Hit, len(hits))
	for i := range hits {
		ranked[i] = search.Hit{ID: hits[i].obj.ID, Kind: hits[i].obj.Kind, Score: hits[i].score}
	}
	search.Order(ranked)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	byID := map[string]loaded{}
	for _, hit := range hits {
		byID[hit.obj.ID] = hit
	}
	var beliefs, observations []any
	for _, hit := range ranked {
		item := byID[hit.ID]
		if item.obj.Kind == "belief" {
			beliefs = append(beliefs, beliefView(item.obj, item.score))
		} else {
			observations = append(observations, observationView(item.obj, item.score))
		}
	}
	if beliefs == nil {
		beliefs = []any{}
	}
	if observations == nil {
		observations = []any{}
	}
	writeOK(stdout, map[string]any{"beliefs": beliefs, "observations": observations})
	return 0
}

func commit(stdout, stderr io.Writer, roots xdg.Roots, opts options, obj record.Object) int {
	vector, rej := embedText(roots.Cache, opts, obj.Text)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	sum, err := record.ContentSHA256(obj)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	id, err := newID(roots.Data)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	obj.ID = id
	if obj.Kind == "belief" && obj.About == nil {
		obj.About = []string{}
	}
	body, err := marshalObject(obj)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	key := cacheKey(opts, obj.Text)
	if err := embed.Put(roots.Cache, key, vector); err != nil {
		if errors.Is(err, embed.ErrDimension) {
			writeReject(stdout, reject("embed_dimension_mismatch", 1))
			return 1
		}
		fmtErr(stderr, err)
		return 2
	}
	if err := store.Commit(roots.Data, obj.Domain, id, body); err != nil {
		_ = embed.Delete(roots.Cache, key)
		fmtErr(stderr, err)
		return 2
	}
	writeOK(stdout, struct {
		ID            string `json:"id"`
		Kind          string `json:"kind"`
		Domain        string `json:"domain"`
		ContentSHA256 string `json:"content_sha256"`
	}{ID: id, Kind: obj.Kind, Domain: obj.Domain, ContentSHA256: sum})
	return 0
}

func unknownKeys(body map[string]json.RawMessage, allowed ...string) *rejection {
	ok := map[string]struct{}{}
	for _, key := range allowed {
		ok[key] = struct{}{}
	}
	for key := range body {
		if _, allowed := ok[key]; !allowed {
			return reject("unknown_field", 2)
		}
	}
	return nil
}

func requiredText(body map[string]json.RawMessage, key string) (string, *rejection) {
	raw, ok := body[key]
	if !ok {
		return "", reject("missing_field", 2)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", reject("invalid_type", 2)
	}
	if text == "" {
		return "", reject("text_empty", 1)
	}
	if utf8.RuneCountInString(text) > 8192 {
		return "", reject("text_too_long", 1)
	}
	return text, nil
}

func parseReference(domainName string, raw json.RawMessage) (record.Reference, *rejection) {
	if len(raw) == 0 {
		return record.Reference{}, reject("missing_field", 2)
	}
	fields, rej := objectFields(raw)
	if rej != nil {
		return record.Reference{}, rej
	}
	if rej := unknownKeys(fields, "source", "start", "end"); rej != nil {
		return record.Reference{}, rej
	}
	sourceRaw, ok := fields["source"]
	if !ok {
		return record.Reference{}, reject("missing_field", 2)
	}
	var source string
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return record.Reference{}, reject("invalid_type", 2)
	}
	start, rej := parseIndex(fields["start"])
	if rej != nil {
		return record.Reference{}, rej
	}
	end, rej := parseIndex(fields["end"])
	if rej != nil {
		return record.Reference{}, rej
	}
	full, rej := sourcePath(domainName, source)
	if rej != nil {
		return record.Reference{}, rej
	}
	file, err := os.ReadFile(full)
	if err != nil {
		return record.Reference{}, reject("reference_not_found", 1)
	}
	if !utf8.Valid(file) {
		return record.Reference{}, reject("reference_encoding", 1)
	}
	length := utf8.RuneCount(file)
	if start < 0 || end <= start || end > length {
		return record.Reference{}, reject("reference_span_invalid", 1)
	}
	return record.Reference{Source: filepath.ToSlash(filepath.Clean(source)), Start: start, End: end}, nil
}

func sourcePath(domainName, source string) (string, *rejection) {
	if source == "" || filepath.IsAbs(source) {
		return "", reject("reference_escapes_domain", 1)
	}
	clean := filepath.Clean(source)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", reject("reference_escapes_domain", 1)
	}
	full := filepath.Join(domainName, clean)
	rel, err := filepath.Rel(domainName, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", reject("reference_escapes_domain", 1)
	}
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", reject("reference_not_found", 1)
	}
	return full, nil
}

func parseIndex(raw json.RawMessage) (int, *rejection) {
	if len(raw) == 0 {
		return 0, reject("missing_field", 2)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var number json.Number
	if err := dec.Decode(&number); err != nil {
		return 0, reject("invalid_type", 2)
	}
	text := number.String()
	if strings.ContainsAny(text, ".eE") {
		return 0, reject("invalid_type", 2)
	}
	value, err := number.Int64()
	if err != nil {
		return 0, reject("invalid_type", 2)
	}
	return int(value), nil
}

func parseInterval(body map[string]json.RawMessage, key string) (*record.Interval, *rejection) {
	raw, ok := body[key]
	if !ok || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	fields, rej := objectFields(raw)
	if rej != nil {
		return nil, rej
	}
	if rej := unknownKeys(fields, "start", "end"); rej != nil {
		return nil, rej
	}
	start, rej := jsonString(fields["start"])
	if rej != nil {
		return nil, rej
	}
	end, rej := jsonString(fields["end"])
	if rej != nil {
		return nil, rej
	}
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return nil, reject("interval_invalid", 1)
	}
	endTime, err := time.Parse(time.RFC3339, end)
	if err != nil || endTime.Before(startTime) {
		return nil, reject("interval_invalid", 1)
	}
	return &record.Interval{Start: start, End: end}, nil
}

func parseReason(dataHome, resolved string, body map[string]json.RawMessage) (*record.Reason, *rejection) {
	raw, ok := body["reason"]
	if !ok || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	fields, rej := objectFields(raw)
	if rej != nil {
		return nil, rej
	}
	kindRaw, ok := fields["kind"]
	if !ok {
		return nil, reject("missing_field", 2)
	}
	var kind string
	if err := json.Unmarshal(kindRaw, &kind); err != nil {
		return nil, reject("invalid_type", 2)
	}
	switch kind {
	case "belief", "observation":
		if rej := unknownKeys(fields, "kind", "id"); rej != nil {
			return nil, rej
		}
		id, rej := jsonString(fields["id"])
		if rej != nil {
			return nil, rej
		}
		obj, rej := objectInSubtree(dataHome, resolved, id)
		if rej != nil {
			return nil, rej
		}
		if obj.Kind != kind {
			return nil, reject("reason_not_found", 1)
		}
		return &record.Reason{Kind: kind, ID: id}, nil
	case "text":
		if rej := unknownKeys(fields, "kind", "text"); rej != nil {
			return nil, rej
		}
		text, rej := jsonString(fields["text"])
		if rej != nil {
			return nil, rej
		}
		if text == "" {
			return nil, reject("text_empty", 1)
		}
		if utf8.RuneCountInString(text) > 8192 {
			return nil, reject("text_too_long", 1)
		}
		return &record.Reason{Kind: kind, Text: text}, nil
	default:
		return nil, reject("reason_kind_invalid", 1)
	}
}

func parseAbout(dataHome, resolved string, body map[string]json.RawMessage) ([]string, *rejection) {
	raw, ok := body["about"]
	if !ok {
		return []string{}, nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, reject("invalid_type", 2)
	}
	sorted, unique := record.SortAbout(ids)
	if !unique {
		return nil, reject("invalid_type", 2)
	}
	for _, id := range sorted {
		if _, rej := objectInSubtree(dataHome, resolved, id); rej != nil {
			return nil, reject("object_not_found", 1)
		}
	}
	return sorted, nil
}

func objectInSubtree(dataHome, resolved, id string) (record.Object, *rejection) {
	obj, err := loadObject(dataHome, id)
	if err != nil {
		return record.Object{}, reject("reason_not_found", 1)
	}
	members, err := store.SubtreeMembership(dataHome, resolved)
	if err != nil {
		return record.Object{}, reject("reason_not_found", 1)
	}
	dom, ok := members[id]
	if !ok {
		return record.Object{}, reject("reason_not_found", 1)
	}
	obj.Domain = dom
	return obj, nil
}

func parseLimit(raw json.RawMessage) (int, *rejection) {
	if len(raw) == 0 {
		return 8, nil
	}
	value, rej := parseIndex(raw)
	if rej != nil {
		return 0, rej
	}
	if value < 1 || value > 20 {
		return 0, reject("limit_invalid", 1)
	}
	return value, nil
}

func objectFields(raw json.RawMessage) (map[string]json.RawMessage, *rejection) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, reject("invalid_type", 2)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, reject("invalid_type", 2)
	}
	return fields, nil
}

func jsonString(raw json.RawMessage) (string, *rejection) {
	if len(raw) == 0 {
		return "", reject("missing_field", 2)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", reject("invalid_type", 2)
	}
	return text, nil
}

func newID(dataHome string) (string, error) {
	for {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		id := hex.EncodeToString(buf)
		if !store.Exists(dataHome, id) {
			return id, nil
		}
	}
}

func loadObject(dataHome, id string) (record.Object, error) {
	raw, err := store.Load(dataHome, id)
	if err != nil {
		return record.Object{}, err
	}
	var obj record.Object
	if err := json.Unmarshal(raw, &obj); err != nil {
		return record.Object{}, err
	}
	return obj, nil
}

func marshalObject(obj record.Object) ([]byte, error) {
	payload := map[string]any{
		"id":       obj.ID,
		"kind":     obj.Kind,
		"domain":   obj.Domain,
		"text":     obj.Text,
		"interval": intervalJSON(obj.Interval),
	}
	if obj.Kind == "observation" {
		payload["reference"] = map[string]any{
			"source": obj.Reference.Source,
			"start":  obj.Reference.Start,
			"end":    obj.Reference.End,
		}
	} else {
		payload["about"] = obj.About
		payload["reason"] = reasonJSON(obj.Reason)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func intervalJSON(interval *record.Interval) any {
	if interval == nil {
		return nil
	}
	return map[string]string{"start": interval.Start, "end": interval.End}
}

func reasonJSON(reason *record.Reason) any {
	if reason == nil {
		return nil
	}
	out := map[string]string{"kind": reason.Kind}
	if reason.Kind == "text" {
		out["text"] = reason.Text
	} else {
		out["id"] = reason.ID
	}
	return out
}

// scoreJSON is a decimal with at most six digits after the point.
type scoreJSON float64

func (s scoreJSON) MarshalJSON() ([]byte, error) {
	rounded := search.Round6(float64(s))
	text := strconv.FormatFloat(rounded, 'f', 6, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		text = "0"
	}
	return []byte(text), nil
}

type observationViewBody struct {
	Domain    string    `json:"domain"`
	ID        string    `json:"id"`
	Interval  any       `json:"interval"`
	Kind      string    `json:"kind"`
	Reference any       `json:"reference"`
	Score     scoreJSON `json:"score"`
	Text      string    `json:"text"`
}

type beliefViewBody struct {
	About    []string  `json:"about"`
	Domain   string    `json:"domain"`
	ID       string    `json:"id"`
	Interval any       `json:"interval"`
	Kind     string    `json:"kind"`
	Reason   any       `json:"reason"`
	Score    scoreJSON `json:"score"`
	Text     string    `json:"text"`
}

func observationView(obj record.Object, score float64) observationViewBody {
	return observationViewBody{
		Domain:    obj.Domain,
		ID:        obj.ID,
		Interval:  intervalJSON(obj.Interval),
		Kind:      obj.Kind,
		Reference: map[string]any{"source": obj.Reference.Source, "start": obj.Reference.Start, "end": obj.Reference.End},
		Score:     scoreJSON(score),
		Text:      obj.Text,
	}
}

func beliefView(obj record.Object, score float64) beliefViewBody {
	about := obj.About
	if about == nil {
		about = []string{}
	}
	return beliefViewBody{
		About:    about,
		Domain:   obj.Domain,
		ID:       obj.ID,
		Interval: intervalJSON(obj.Interval),
		Kind:     obj.Kind,
		Reason:   reasonJSON(obj.Reason),
		Score:    scoreJSON(score),
		Text:     obj.Text,
	}
}

func fmtErr(stderr io.Writer, err error) {
	io.WriteString(stderr, "agmemx: "+err.Error()+"\n")
}
