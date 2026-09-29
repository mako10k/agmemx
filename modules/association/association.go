// Package association adds one-hop memory context and budgeted presentation to ranked candidates.
// R: Assemble living graph hints and statistics without changing candidate selection or rank.
package association

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/mako10k/agmemx/modules/domain"
	"github.com/mako10k/agmemx/modules/model"
)

var (
	ErrInvalidInput         = errors.New("invalid association input")
	ErrInconsistentSnapshot = errors.New("inconsistent association snapshot")
	ErrBudgetTooSmall       = errors.New("budget too small")
)

const DefaultBudget = 4096

// Candidate is already ranked by the candidate module. Association never reorders it.
type Candidate struct {
	ID        model.RecordID
	Score     float64
	ScoreKind string // cosine or lexical
}

// Node is the graph projection of a living or logically deleted record.
type Node struct {
	ID            model.RecordID
	Kind          model.RecordKind
	BodyHash      model.BodyHash
	Domain        domain.DomainPath
	DisplayName   string
	Reference     *model.Reference
	Reason        *model.RecordReason
	ReferenceBase domain.DomainPath
	Deleted       bool
}

// Relation is the graph projection of a public relation.
type Relation struct {
	Value        model.Relation
	EvidenceBase domain.DomainPath
	Deleted      bool
}

type InternalKind string

const (
	AboutLink  InternalKind = "about"
	ReasonLink InternalKind = "reason"
)

// InternalLink describes one stored about or reason reference. From identifies
// the owning record, or the source endpoint of an owning relation.
type InternalLink struct {
	From          model.RecordID
	To            model.RecordID
	Kind          InternalKind
	OwnerRelation model.RelationID
	Deleted       bool
}

// GraphReader must read one immutable revision and return incident links in
// both directions. It may include deleted values, which association filters.
type GraphReader interface {
	Node(context.Context, model.RecordID) (Node, bool, error)
	RelationByID(context.Context, model.RelationID) (Relation, bool, error)
	Relations(context.Context, model.RecordID) ([]Relation, error)
	InternalLinks(context.Context, model.RecordID) ([]InternalLink, error)
}

type BodyReader interface {
	Body(context.Context, model.BodyHash) (string, error)
}

// Resolution preserves the original source and range even when unresolved.
type Resolution struct {
	Source           string  `json:"source"`
	Start            int     `json:"start"`
	End              int     `json:"end"`
	Resolution       string  `json:"resolution"`
	UnresolvedReason *string `json:"unresolved_reason"`
	Context          *string `json:"context"`
}

type ReferenceResolver interface {
	Resolve(context.Context, model.Reference, domain.DomainPath) (Resolution, error)
}

// Renderer belongs to the consumer's presentation boundary. It must serialize
// the complete success result, including its own budget_used field.
type Renderer func(Result) ([]byte, error)

type Builder struct {
	Graph      GraphReader
	Bodies     BodyReader
	References ReferenceResolver
}

type DirectionCount struct {
	Incoming int `json:"incoming"`
	Outgoing int `json:"outgoing"`
}

type Stats struct {
	IncomingReferences int                       `json:"incoming_references"`
	RelationCounts     map[string]DirectionCount `json:"relation_counts"`
	ExternalReference  string                    `json:"external_reference_state"`
}

type Reason struct {
	Kind string  `json:"kind"`
	ID   *string `json:"id,omitempty"`
	Text *string `json:"text,omitempty"`
}

type Hint struct {
	SourceKind         string      `json:"source_kind"`
	RecordID           *string     `json:"record_id"`
	RelationID         *string     `json:"relation_id"`
	RelationKind       *string     `json:"relation_kind"`
	Direction          string      `json:"direction"`
	Domain             *string     `json:"domain"`
	Reason             *Reason     `json:"reason"`
	EvidenceReference  *Resolution `json:"evidence_reference"`
	EvidenceBaseDomain *string     `json:"evidence_base_domain"`
	ReasonPresent      bool        `json:"reason_present"`
	EvidencePresent    bool        `json:"evidence_present"`
	Summary            *string     `json:"summary"`
}

type CandidateView struct {
	RecordID  string  `json:"record_id"`
	Kind      string  `json:"kind"`
	Score     float64 `json:"score"`
	ScoreKind string  `json:"score_kind"`
	Summary   string  `json:"summary"`
	Stats     Stats   `json:"stats"`
	Hints     []Hint  `json:"hints"`
}

type Omitted struct {
	Candidates int  `json:"candidates"`
	Hints      int  `json:"hints"`
	Summaries  int  `json:"summaries"`
	Truncated  bool `json:"truncated"`
}

type Result struct {
	Mode       string          `json:"mode"`
	Total      int             `json:"total"`
	Candidates []CandidateView `json:"candidates"`
	Omitted    Omitted         `json:"omitted"`
	BudgetUsed int             `json:"budget_used"`
}

type prepared struct {
	view    CandidateView
	summary string
	hints   []Hint
	weight  float64
}

// Build retains the supplied candidate order and total. The returned bytes
// are exactly those measured against budget; callers must emit these bytes.
func (b Builder) Build(ctx context.Context, mode string, total int, candidates []Candidate, budget int, render Renderer) (Result, []byte, error) {
	if b.Graph == nil || b.Bodies == nil || render == nil || (mode != "text" && mode != "embedding") || total < len(candidates) || budget < 0 {
		return Result{}, nil, ErrInvalidInput
	}
	if budget == 0 {
		budget = DefaultBudget
	}
	preparedValues := make([]prepared, 0, len(candidates))
	seen := make(map[model.RecordID]bool, len(candidates))
	for _, candidate := range candidates {
		if !candidate.ID.Valid() || seen[candidate.ID] || math.IsNaN(candidate.Score) || math.IsInf(candidate.Score, 0) || (candidate.ScoreKind != "cosine" && candidate.ScoreKind != "lexical") {
			return Result{}, nil, ErrInvalidInput
		}
		seen[candidate.ID] = true
		value, err := b.prepare(ctx, candidate)
		if err != nil {
			return Result{}, nil, err
		}
		preparedValues = append(preparedValues, value)
	}
	state := selection{mode: mode, total: total, values: preparedValues, included: 0, hints: make([]map[int]bool, len(preparedValues)), summaries: make([]string, len(preparedValues)), hintSummaries: make([]map[int]string, len(preparedValues)), hintReasons: make([]map[int]string, len(preparedValues)), hintContexts: make([]map[int]string, len(preparedValues))}
	if len(preparedValues) > 0 {
		state.included = 1
	}
	result, data, err := state.measure(render)
	if err != nil {
		return Result{}, nil, err
	}
	if len(data) > budget {
		return Result{}, nil, ErrBudgetTooSmall
	}
	for {
		choice, found, err := state.bestFitting(ctx, budget, render)
		if err != nil {
			return Result{}, nil, err
		}
		if !found {
			break
		}
		state = choice
	}
	result, data, err = state.measure(render)
	if err != nil {
		return Result{}, nil, err
	}
	if len(data) > budget {
		return Result{}, nil, ErrBudgetTooSmall
	}
	return result, data, nil
}

func (b Builder) prepare(ctx context.Context, candidate Candidate) (prepared, error) {
	if err := ctx.Err(); err != nil {
		return prepared{}, err
	}
	node, found, err := b.Graph.Node(ctx, candidate.ID)
	if err != nil {
		return prepared{}, err
	}
	if !found || node.Deleted || node.ID != candidate.ID || !node.Kind.Valid() || !node.Domain.Valid() {
		return prepared{}, ErrInconsistentSnapshot
	}
	stats := Stats{RelationCounts: map[string]DirectionCount{"next": {}, "change": {}, "end": {}}, ExternalReference: "none"}
	contextSnippet := ""
	if node.Reference != nil {
		if b.References == nil || !node.ReferenceBase.Valid() {
			return prepared{}, ErrInconsistentSnapshot
		}
		resolution, err := b.References.Resolve(ctx, *node.Reference, node.ReferenceBase)
		if err != nil {
			return prepared{}, err
		}
		if resolution.Resolution != "resolved" && resolution.Resolution != "unresolved" {
			return prepared{}, ErrInconsistentSnapshot
		}
		stats.ExternalReference = resolution.Resolution
		if resolution.Context != nil && resolution.Resolution == "resolved" {
			contextSnippet = truncateRunes(*resolution.Context, 40)
		}
	}
	text, err := b.Bodies.Body(ctx, node.BodyHash)
	if err != nil {
		return prepared{}, err
	}
	if !utf8.ValidString(text) {
		return prepared{}, ErrInconsistentSnapshot
	}
	summary := snippet(node.DisplayName, text)
	if contextSnippet != "" {
		summary += " — " + contextSnippet
	}
	relations, err := b.Graph.Relations(ctx, candidate.ID)
	if err != nil {
		return prepared{}, err
	}
	links, err := b.Graph.InternalLinks(ctx, candidate.ID)
	if err != nil {
		return prepared{}, err
	}
	hints := make([]Hint, 0, len(relations)+len(links))
	seen := make(map[string]bool)
	if node.Reason != nil {
		reason := recordReason(*node.Reason)
		if reason.Kind == "text" {
			hints = append(hints, Hint{SourceKind: "reason", Direction: "outgoing", Reason: reason, ReasonPresent: true})
		} else if reason.ID != nil {
			peerID, _ := node.Reason.ID()
			peer, alive, err := b.liveNode(ctx, peerID)
			if err != nil {
				return prepared{}, err
			}
			if !alive || string(peer.Kind) != string(node.Reason.Kind()) {
				return prepared{}, ErrInconsistentSnapshot
			}
			peerSummary, err := b.nodeSummary(ctx, peer)
			if err != nil {
				return prepared{}, err
			}
			pid, path := peerID.String(), peer.Domain.String()
			hints = append(hints, Hint{SourceKind: "reason", RecordID: &pid, Direction: "outgoing", Domain: &path, Reason: reason, ReasonPresent: true, Summary: &peerSummary})
		}
	}
	for _, relation := range relations {
		if relation.Deleted {
			continue
		}
		value := relation.Value
		if !value.ID().Valid() || !value.Kind().Valid() {
			return prepared{}, ErrInconsistentSnapshot
		}
		key := "relation:" + value.ID().String()
		if seen[key] {
			continue
		}
		seen[key] = true
		peerID, direction := value.To(), "outgoing"
		if value.From() != candidate.ID {
			if value.To() != candidate.ID {
				return prepared{}, ErrInconsistentSnapshot
			}
			peerID, direction = value.From(), "incoming"
		}
		peer, alive, err := b.liveNode(ctx, peerID)
		if err != nil {
			return prepared{}, err
		}
		if !alive {
			continue
		}
		peerSummary, err := b.nodeSummary(ctx, peer)
		if err != nil {
			return prepared{}, err
		}
		counts := stats.RelationCounts[string(value.Kind())]
		if value.From() == candidate.ID {
			counts.Outgoing++
		}
		if value.To() == candidate.ID {
			counts.Incoming++
		}
		stats.RelationCounts[string(value.Kind())] = counts
		rid, kind, pid, path := value.ID().String(), string(value.Kind()), peerID.String(), peer.Domain.String()
		hint := Hint{SourceKind: "relation", RecordID: &pid, RelationID: &rid, RelationKind: &kind, Direction: direction, Domain: &path, Summary: &peerSummary}
		if reason, ok := value.Reason(); ok {
			hint.Reason = relationReason(reason)
			hint.ReasonPresent = true
		}
		if reference, ok := value.EvidenceReference(); ok {
			if b.References == nil || !relation.EvidenceBase.Valid() {
				return prepared{}, ErrInconsistentSnapshot
			}
			resolved, err := b.References.Resolve(ctx, reference, relation.EvidenceBase)
			if err != nil {
				return prepared{}, err
			}
			hint.EvidenceReference = &resolved
			base := relation.EvidenceBase.String()
			hint.EvidenceBaseDomain = &base
			hint.EvidencePresent = true
		}
		hints = append(hints, hint)
	}
	for _, link := range links {
		if link.Deleted {
			continue
		}
		if link.Kind == ReasonLink && !link.OwnerRelation.Valid() && link.From == candidate.ID && node.Reason != nil {
			if reasonID, ok := node.Reason.ID(); ok && reasonID == link.To {
				continue
			}
		}
		if link.OwnerRelation.Valid() {
			owner, found, err := b.Graph.RelationByID(ctx, link.OwnerRelation)
			if err != nil {
				return prepared{}, err
			}
			if !found || owner.Deleted {
				continue
			}
			if owner.Value.ID() != link.OwnerRelation {
				return prepared{}, ErrInconsistentSnapshot
			}
		}
		if link.Kind != AboutLink && link.Kind != ReasonLink {
			return prepared{}, ErrInconsistentSnapshot
		}
		peerID, direction := link.To, "outgoing"
		if link.From != candidate.ID {
			if link.To != candidate.ID {
				return prepared{}, ErrInconsistentSnapshot
			}
			peerID, direction = link.From, "incoming"
		}
		peer, alive, err := b.liveNode(ctx, peerID)
		if err != nil {
			return prepared{}, err
		}
		if !alive {
			continue
		}
		peerSummary, err := b.nodeSummary(ctx, peer)
		if err != nil {
			return prepared{}, err
		}
		key := string(link.Kind) + ":" + link.From.String() + ":" + link.To.String() + ":" + link.OwnerRelation.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		if link.To == candidate.ID && (link.From != candidate.ID || link.OwnerRelation.Valid()) {
			stats.IncomingReferences++
		}
		pid, path := peerID.String(), peer.Domain.String()
		hint := Hint{SourceKind: string(link.Kind), RecordID: &pid, Direction: direction, Domain: &path, Summary: &peerSummary, ReasonPresent: link.Kind == ReasonLink}
		if link.Kind == ReasonLink {
			reasonID := link.To.String()
			hint.Reason = &Reason{Kind: "record", ID: &reasonID}
		}
		hints = append(hints, hint)
	}
	linksCount := 0
	for _, count := range stats.RelationCounts {
		linksCount += count.Incoming + count.Outgoing
	}
	weight := 100*normalized(candidate.Score, candidate.ScoreKind) + math.Min(20, 2*float64(stats.IncomingReferences)) + math.Min(10, float64(linksCount))
	return prepared{view: CandidateView{RecordID: candidate.ID.String(), Kind: string(node.Kind), Score: candidate.Score, ScoreKind: candidate.ScoreKind, Stats: stats, Hints: []Hint{}}, summary: summary, hints: hints, weight: weight}, nil
}

func (b Builder) nodeSummary(ctx context.Context, node Node) (string, error) {
	text, err := b.Bodies.Body(ctx, node.BodyHash)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(text) {
		return "", ErrInconsistentSnapshot
	}
	return snippet(node.DisplayName, text), nil
}

func (b Builder) liveNode(ctx context.Context, id model.RecordID) (Node, bool, error) {
	if !id.Valid() {
		return Node{}, false, ErrInconsistentSnapshot
	}
	node, found, err := b.Graph.Node(ctx, id)
	if err != nil || !found || node.Deleted {
		return Node{}, false, err
	}
	if node.ID != id || !node.Domain.Valid() {
		return Node{}, false, ErrInconsistentSnapshot
	}
	return node, true, nil
}

func normalized(score float64, kind string) float64 {
	if kind == "cosine" {
		return math.Max(0, math.Min(1, (score+1)/2))
	}
	return math.Max(0, math.Min(1, score))
}

func snippet(name, text string) string {
	text = truncateRunes(text, 120)
	if name == "" {
		return text
	}
	return name + " — " + text
}

func truncateRunes(text string, count int) string {
	runes := []rune(text)
	if len(runes) > count {
		return string(runes[:count])
	}
	return text
}

func relationReason(reason model.RelationReason) *Reason {
	value := &Reason{Kind: string(reason.Kind())}
	if id, ok := reason.ID(); ok {
		s := id.String()
		value.ID = &s
	}
	if text, ok := reason.Text(); ok {
		value.Text = &text
	}
	return value
}

func recordReason(reason model.RecordReason) *Reason {
	value := &Reason{Kind: string(reason.Kind())}
	if id, ok := reason.ID(); ok {
		s := id.String()
		value.Kind, value.ID = "record", &s
	}
	if text, ok := reason.Text(); ok {
		value.Text = &text
	}
	return value
}

// stableKey is used only for equal-priority tie breaking, after candidate rank.
func stableKey(h Hint) string {
	if h.RelationID != nil {
		return *h.RelationID
	}
	if h.RecordID != nil {
		return *h.RecordID
	}
	return strings.Join([]string{h.SourceKind, h.Direction}, ":")
}

func hintWeight(h Hint) float64 {
	if h.RelationKind != nil {
		switch *h.RelationKind {
		case "change", "end":
			return 20
		case "next":
			return 10
		}
	}
	return 8
}
