// Package candidate ranks living records by one selected search method.
// R: Choose embedding or lexical retrieval for a fixed read scope and rank its candidates.
package candidate

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mako10k/agmemx/modules/domain"
	"github.com/mako10k/agmemx/modules/model"
)

var (
	ErrInvalidInput         = errors.New("invalid candidate input")
	ErrEmbeddingUnavailable = errors.New("embedding unavailable")
	ErrInconsistentSnapshot = errors.New("inconsistent candidate snapshot")
)

type Mode string

const (
	ModeAuto      Mode = "auto"
	ModeEmbedding Mode = "embedding"
	ModeText      Mode = "text"
)

// Record is the current body identity of a record in one immutable read image.
type Record struct {
	ID       model.RecordID
	BodyHash model.BodyHash
	Domain   domain.DomainPath
	Deleted  bool
}

// ScopedReader opens one immutable ledger read image for the entire search.
type ScopedReader interface {
	Open(context.Context, domain.DomainPath) (Snapshot, error)
}

// LexicalReader reads frequencies from the opened image and may omit zeros.
type LexicalReader interface {
	Frequencies(context.Context, []model.RecordID, []string) (map[model.RecordID]map[string]int, error)
}

// Snapshot binds scoped records and lexical index reads to one ledger revision.
// Vector lookup is separate but always keyed by these records' current hashes.
type Snapshot interface {
	LexicalReader
	Records(context.Context) ([]Record, error)
	Close() error
}

// VectorReader addresses vectors by the resolved profile key and exact current
// body hash. Missing is distinct from an I/O failure.
type VectorReader interface {
	Vector(context.Context, string, model.BodyHash) ([]float64, bool, error)
}

// QueryEmbedder uses the same resolved profile key as VectorReader.
type QueryEmbedder interface {
	EmbedQuery(context.Context, string, string) ([]float64, error)
}

// Request.ProfileKey is supplied by the profile owner after validation and
// normalization. An empty key means no usable profile was selected.
type Request struct {
	Scope      domain.DomainPath
	Query      string
	Mode       Mode
	Limit      int
	ProfileKey string
}

type Candidate struct {
	ID        model.RecordID
	Score     float64
	ScoreKind string // cosine or lexical
}

type Result struct {
	Mode       Mode
	Total      int
	Candidates []Candidate
}

type Searcher struct {
	Scope   ScopedReader
	Vectors VectorReader
	Queries QueryEmbedder
}

// Search fixes the living scope before ranking. Embedding is used only when
// every body has a valid vector and the query vector succeeds; otherwise auto
// searches the entire scope lexically and explicit embedding returns an error.
func (s Searcher) Search(ctx context.Context, request Request) (Result, error) {
	if s.Scope == nil || !request.Scope.Valid() || request.Limit < 1 || !utf8.ValidString(request.Query) || strings.TrimSpace(request.Query) == "" {
		return Result{}, ErrInvalidInput
	}
	if request.Mode == "" {
		request.Mode = ModeAuto
	}
	if request.Mode != ModeAuto && request.Mode != ModeEmbedding && request.Mode != ModeText {
		return Result{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	snapshot, err := s.Scope.Open(ctx, request.Scope)
	if err != nil {
		return Result{}, err
	}
	if snapshot == nil {
		return Result{}, ErrInconsistentSnapshot
	}
	defer snapshot.Close()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	records, err := snapshot.Records(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	living := make([]Record, 0, len(records))
	seen := make(map[model.RecordID]bool, len(records))
	for _, record := range records {
		if !record.ID.Valid() || record.BodyHash == (model.BodyHash{}) || !record.Domain.Valid() || !request.Scope.Contains(record.Domain) || seen[record.ID] {
			return Result{}, ErrInconsistentSnapshot
		}
		seen[record.ID] = true
		if !record.Deleted {
			living = append(living, record)
		}
	}
	if len(living) == 0 && request.Mode != ModeEmbedding {
		terms, err := Lexemes(request.Query)
		if err != nil || len(terms) == 0 {
			return Result{}, ErrInvalidInput
		}
		return Result{Mode: ModeText, Candidates: []Candidate{}}, nil
	}
	if request.Mode != ModeText {
		result, available, err := s.embedding(ctx, request, living)
		if err != nil {
			return Result{}, err
		}
		if available {
			return result, nil
		}
		if request.Mode == ModeEmbedding {
			return Result{}, ErrEmbeddingUnavailable
		}
	}
	return s.text(ctx, request, living, snapshot)
}

func (s Searcher) embedding(ctx context.Context, request Request, records []Record) (Result, bool, error) {
	if request.ProfileKey == "" || s.Vectors == nil || s.Queries == nil {
		return Result{}, false, nil
	}
	vectors := make([][]float64, len(records))
	for i, record := range records {
		if err := ctx.Err(); err != nil {
			return Result{}, false, err
		}
		value, found, err := s.Vectors.Vector(ctx, request.ProfileKey, record.BodyHash)
		if ctx.Err() != nil {
			return Result{}, false, ctx.Err()
		}
		if err != nil {
			return Result{}, false, err
		}
		if !found || !validVector(value) {
			return Result{}, false, nil
		}
		vectors[i] = value
	}
	query, err := s.Queries.EmbedQuery(ctx, request.ProfileKey, request.Query)
	if ctx.Err() != nil {
		return Result{}, false, ctx.Err()
	}
	if err != nil {
		return Result{}, false, nil
	}
	if !validVector(query) {
		return Result{}, false, nil
	}
	result := Result{Mode: ModeEmbedding, Candidates: make([]Candidate, 0, len(records))}
	for i, record := range records {
		if len(vectors[i]) != len(query) {
			return Result{}, false, nil
		}
		score := cosine(query, vectors[i])
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return Result{}, false, nil
		}
		result.Candidates = append(result.Candidates, Candidate{ID: record.ID, Score: score, ScoreKind: "cosine"})
	}
	result.Total = len(result.Candidates)
	rankAndLimit(&result, request.Limit)
	return result, true, nil
}

func (s Searcher) text(ctx context.Context, request Request, records []Record, snapshot Snapshot) (Result, error) {
	terms, err := Lexemes(request.Query)
	if err != nil || len(terms) == 0 {
		return Result{}, ErrInvalidInput
	}
	unique := make(map[string]bool, len(terms))
	for _, term := range terms {
		unique[term] = true
	}
	queryTerms := make([]string, 0, len(unique))
	for term := range unique {
		queryTerms = append(queryTerms, term)
	}
	sort.Strings(queryTerms)
	ids := make([]model.RecordID, 0, len(records))
	allowed := make(map[model.RecordID]bool, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
		allowed[record.ID] = true
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	counts, err := snapshot.Frequencies(ctx, ids, queryTerms)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result := Result{Mode: ModeText, Candidates: []Candidate{}}
	for id, frequencies := range counts {
		if !allowed[id] {
			return Result{}, ErrInconsistentSnapshot
		}
		matched, totalFrequency := 0, 0
		for term, frequency := range frequencies {
			if !unique[term] || frequency < 0 {
				return Result{}, ErrInconsistentSnapshot
			}
			if frequency > 0 {
				matched++
				if frequency >= 10-totalFrequency {
					totalFrequency = 10
				} else {
					totalFrequency += frequency
				}
			}
		}
		if matched > 0 {
			score := float64(matched)/float64(len(queryTerms)) + math.Min(0.1, float64(totalFrequency)/100)
			result.Candidates = append(result.Candidates, Candidate{ID: id, Score: score, ScoreKind: "lexical"})
		}
	}
	result.Total = len(result.Candidates)
	rankAndLimit(&result, request.Limit)
	return result, nil
}

func rankAndLimit(result *Result, limit int) {
	sort.Slice(result.Candidates, func(i, j int) bool {
		left, right := result.Candidates[i], result.Candidates[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		return left.ID.String() < right.ID.String()
	})
	if len(result.Candidates) > limit {
		result.Candidates = result.Candidates[:limit]
	}
}

func validVector(vector []float64) bool {
	if len(vector) == 0 {
		return false
	}
	nonzero := false
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		if value != 0 {
			nonzero = true
		}
	}
	return nonzero
}

func cosine(left, right []float64) float64 {
	var leftMax, rightMax float64
	for i := range left {
		leftMax = math.Max(leftMax, math.Abs(left[i]))
		rightMax = math.Max(rightMax, math.Abs(right[i]))
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		a, b := left[i]/leftMax, right[i]/rightMax
		dot += a * b
		leftNorm += a * a
		rightNorm += b * b
	}
	return math.Max(-1, math.Min(1, dot/(math.Sqrt(leftNorm)*math.Sqrt(rightNorm))))
}
