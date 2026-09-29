package candidate

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/mako10k/agmemx/modules/domain"
	"github.com/mako10k/agmemx/modules/model"
)

type scopeFixture struct {
	records []Record
	lexical *lexicalFixture
}

func (s scopeFixture) Open(_ context.Context, _ domain.DomainPath) (Snapshot, error) {
	return snapshotFixture{s}, nil
}

type snapshotFixture struct{ scopeFixture }

func (s snapshotFixture) Records(_ context.Context) ([]Record, error) {
	return s.records, nil
}

func (s snapshotFixture) Frequencies(ctx context.Context, ids []model.RecordID, terms []string) (map[model.RecordID]map[string]int, error) {
	return s.lexical.Frequencies(ctx, ids, terms)
}

func (snapshotFixture) Close() error { return nil }

type lexicalFixture struct {
	counts map[model.RecordID]map[string]int
	calls  int
	after  func()
}

func (l *lexicalFixture) Frequencies(_ context.Context, _ []model.RecordID, _ []string) (map[model.RecordID]map[string]int, error) {
	l.calls++
	if l.after != nil {
		l.after()
	}
	return l.counts, nil
}

type vectorFixture struct {
	byHash map[model.BodyHash][]float64
	calls  int
	keys   []string
}

func (v *vectorFixture) Vector(_ context.Context, key string, hash model.BodyHash) ([]float64, bool, error) {
	v.calls++
	v.keys = append(v.keys, key)
	value, found := v.byHash[hash]
	return value, found, nil
}

type queryFixture struct {
	vector []float64
	err    error
	calls  int
	keys   []string
	after  func()
}

func (q *queryFixture) EmbedQuery(_ context.Context, key, _ string) ([]float64, error) {
	q.calls++
	q.keys = append(q.keys, key)
	if q.after != nil {
		q.after()
	}
	return q.vector, q.err
}

func fixtureID(t *testing.T, digit byte) model.RecordID {
	t.Helper()
	id, err := model.ParseRecordID(strings.Repeat(string(digit), 32))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func fixturePath(t *testing.T, value string) domain.DomainPath {
	t.Helper()
	path, err := domain.ParseStoredPath(value)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureRecord(id model.RecordID, body string, path domain.DomainPath) Record {
	return Record{ID: id, BodyHash: model.BodyHashFromDigest(sha256.Sum256([]byte(body))), Domain: path}
}

func TestLexemesWidthCaseAndCJK(t *testing.T) {
	terms, err := Lexemes("ＦＯＯ foo、東京大学 東 ｶﾞｸ ガク")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"foo", "foo", "東京", "京大", "大学", "東", "ガク", "ガク"}
	if !reflect.DeepEqual(terms, want) {
		t.Fatalf("terms = %q, want %q", terms, want)
	}
	if _, err := Lexemes("\xff"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid UTF-8 accepted: %v", err)
	}
}

func TestTextRankingFrequencyTieLimitAndDeleted(t *testing.T) {
	a, b, c, dead := fixtureID(t, '1'), fixtureID(t, '2'), fixtureID(t, '3'), fixtureID(t, '4')
	p := fixturePath(t, "/notes")
	records := []Record{fixtureRecord(a, "a", p), fixtureRecord(b, "b", p), fixtureRecord(c, "c", p), fixtureRecord(dead, "dead", p)}
	records[3].Deleted = true
	lexical := &lexicalFixture{counts: map[model.RecordID]map[string]int{
		a: {"foo": 2, "bar": 1},
		b: {"foo": 2, "bar": 1},
		c: {"foo": 1},
	}}
	searcher := Searcher{Scope: scopeFixture{records: records, lexical: lexical}}
	result, err := searcher.Search(context.Background(), Request{Scope: p, Query: "ＦＯＯ foo bar", Mode: ModeText, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != ModeText || result.Total != 3 || len(result.Candidates) != 2 || result.Candidates[0].ID != a || result.Candidates[1].ID != b || result.Candidates[0].ScoreKind != "lexical" || math.Abs(result.Candidates[0].Score-1.03) > 1e-10 {
		t.Fatalf("lexical score, ties, or limit incorrect: %+v", result)
	}
	if lexical.calls != 1 {
		t.Fatalf("index queried %d times", lexical.calls)
	}
	if _, err := searcher.Search(context.Background(), Request{Scope: p, Query: "!!!", Mode: ModeText, Limit: 2}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero-term query = %v", err)
	}
}

func TestEmbeddingAllOrNothingAndCurrentHashes(t *testing.T) {
	a, b := fixtureID(t, '5'), fixtureID(t, '6')
	p := fixturePath(t, "/notes")
	ra, rb := fixtureRecord(a, "current-a", p), fixtureRecord(b, "current-b", p)
	old := fixtureRecord(a, "old-a", p)
	lexical := &lexicalFixture{counts: map[model.RecordID]map[string]int{a: {"term": 1}, b: {"term": 2}}}
	vectors := &vectorFixture{byHash: map[model.BodyHash][]float64{old.BodyHash: {1, 0}, ra.BodyHash: {1, 0}}}
	query := &queryFixture{vector: []float64{0, 1}}
	searcher := Searcher{Scope: scopeFixture{records: []Record{ra, rb}, lexical: lexical}, Vectors: vectors, Queries: query}
	request := Request{Scope: p, Query: "term", Mode: ModeAuto, Limit: 10, ProfileKey: "provider|url|model"}
	result, err := searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeText || result.Total != 2 || query.calls != 0 {
		t.Fatalf("missing current hash did not fall back as a whole: %+v, %v, calls=%d", result, err, query.calls)
	}
	request.Mode = ModeEmbedding
	if _, err := searcher.Search(context.Background(), request); !errors.Is(err, ErrEmbeddingUnavailable) {
		t.Fatalf("explicit missing vector = %v", err)
	}
	vectors.byHash[rb.BodyHash] = []float64{0, 1}
	result, err = searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeEmbedding || result.Total != 2 || result.Candidates[0].ID != b || result.Candidates[0].ScoreKind != "cosine" || result.Candidates[0].Score != 1 || result.Candidates[1].ID != a || result.Candidates[1].Score != 0 {
		t.Fatalf("cosine rank wrong: %+v, %v", result, err)
	}
	if query.calls != 1 || !reflect.DeepEqual(query.keys, []string{request.ProfileKey}) {
		t.Fatalf("query profile key mismatch: %+v", query)
	}
	query.err = errors.New("provider unavailable")
	request.Mode = ModeAuto
	result, err = searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeText {
		t.Fatalf("provider failure fallback = %+v, %v", result, err)
	}
	query.err = nil
	vectors.byHash[rb.BodyHash] = []float64{0, 1, 0}
	result, err = searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeText || result.Total != 2 {
		t.Fatalf("mixed vector dimensions did not fall back as a whole: %+v, %v", result, err)
	}
	request.Mode = ModeEmbedding
	if _, err := searcher.Search(context.Background(), request); !errors.Is(err, ErrEmbeddingUnavailable) {
		t.Fatalf("explicit mixed dimensions = %v", err)
	}
	request.Mode, request.ProfileKey = ModeAuto, ""
	result, err = searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeText {
		t.Fatalf("missing profile auto fallback = %+v, %v", result, err)
	}
}

func TestEmptyScopeModeAndCancellation(t *testing.T) {
	p := fixturePath(t, "/notes")
	query := &queryFixture{vector: []float64{1}}
	searcher := Searcher{Scope: scopeFixture{}, Queries: query, Vectors: &vectorFixture{}}
	request := Request{Scope: p, Query: "term", Mode: ModeAuto, Limit: 1, ProfileKey: "profile"}
	result, err := searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeText || result.Total != 0 || len(result.Candidates) != 0 || query.calls != 0 {
		t.Fatalf("empty auto called provider or wrong mode: %+v, %v", result, err)
	}
	request.Query = "!!!"
	if _, err := searcher.Search(context.Background(), request); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty-scope zero-term query = %v", err)
	}
	request.Query = "term"
	request.Mode = ModeEmbedding
	result, err = searcher.Search(context.Background(), request)
	if err != nil || result.Mode != ModeEmbedding || query.calls != 1 {
		t.Fatalf("empty explicit embedding not probed: %+v, %v", result, err)
	}
	query.err = errors.New("offline")
	if _, err := searcher.Search(context.Background(), request); !errors.Is(err, ErrEmbeddingUnavailable) {
		t.Fatalf("empty explicit provider failure = %v", err)
	}
	request.ProfileKey = ""
	if _, err := searcher.Search(context.Background(), request); !errors.Is(err, ErrEmbeddingUnavailable) {
		t.Fatalf("empty explicit profile failure = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := searcher.Search(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search = %v", err)
	}
}

func TestCancellationAfterPortReturns(t *testing.T) {
	id := fixtureID(t, 'a')
	p := fixturePath(t, "/notes")
	record := fixtureRecord(id, "term", p)
	ctx, cancel := context.WithCancel(context.Background())
	lexical := &lexicalFixture{counts: map[model.RecordID]map[string]int{id: {"term": 1}}, after: cancel}
	searcher := Searcher{Scope: scopeFixture{records: []Record{record}, lexical: lexical}}
	if _, err := searcher.Search(ctx, Request{Scope: p, Query: "term", Mode: ModeText, Limit: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("index ignored cancellation = %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	query := &queryFixture{vector: []float64{1}, after: cancel}
	searcher.Queries, searcher.Vectors = query, &vectorFixture{byHash: map[model.BodyHash][]float64{record.BodyHash: {1}}}
	if _, err := searcher.Search(ctx, Request{Scope: p, Query: "term", Mode: ModeEmbedding, Limit: 1, ProfileKey: "profile"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("embedder ignored cancellation = %v", err)
	}
}
