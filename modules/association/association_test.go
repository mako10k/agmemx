package association

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mako10k/agmemx/modules/domain"
	"github.com/mako10k/agmemx/modules/model"
)

type graphFixture struct {
	nodes     map[model.RecordID]Node
	owners    map[model.RelationID]Relation
	relations map[model.RecordID][]Relation
	links     map[model.RecordID][]InternalLink
}

func (g graphFixture) RelationByID(_ context.Context, id model.RelationID) (Relation, bool, error) {
	relation, ok := g.owners[id]
	return relation, ok, nil
}

func (g graphFixture) Node(_ context.Context, id model.RecordID) (Node, bool, error) {
	node, ok := g.nodes[id]
	return node, ok, nil
}
func (g graphFixture) Relations(_ context.Context, id model.RecordID) ([]Relation, error) {
	return g.relations[id], nil
}
func (g graphFixture) InternalLinks(_ context.Context, id model.RecordID) ([]InternalLink, error) {
	return g.links[id], nil
}

type bodyFixture map[model.BodyHash]string

func (b bodyFixture) Body(_ context.Context, hash model.BodyHash) (string, error) {
	text, ok := b[hash]
	if !ok {
		return "", errors.New("missing body")
	}
	return text, nil
}

// typedReferenceFixture keeps the test port identical to the public contract.
type typedReferenceFixture struct{ result Resolution }

func (r typedReferenceFixture) Resolve(_ context.Context, _ model.Reference, _ domain.DomainPath) (Resolution, error) {
	return r.result, nil
}

func recordID(t *testing.T, digit byte) model.RecordID {
	t.Helper()
	id, err := model.ParseRecordID(strings.Repeat(string(digit), 32))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func relationID(t *testing.T, digit byte) model.RelationID {
	t.Helper()
	id, err := model.ParseRelationID(strings.Repeat(string(digit), 32))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func path(t *testing.T, value string) domain.DomainPath {
	t.Helper()
	p, err := domain.ParseStoredPath(value)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func node(id model.RecordID, text string, p domain.DomainPath) (Node, model.BodyHash) {
	hash := model.BodyHashFromDigest(sha256.Sum256([]byte(text)))
	return Node{ID: id, Kind: model.Belief, BodyHash: hash, Domain: p}, hash
}

func relation(t *testing.T, id model.RelationID, kind model.RelationKind, from, to model.RecordID) Relation {
	t.Helper()
	value, err := model.NewRelation(model.RelationSpec{ID: id, Kind: kind, From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	return Relation{Value: value}
}

func renderJSON(result Result) ([]byte, error) {
	data, err := json.Marshal(struct {
		OK     bool   `json:"ok"`
		Result Result `json:"result"`
	}{true, result})
	return append(data, '\n'), err
}

func renderText(result Result) ([]byte, error) {
	var out strings.Builder
	fmt.Fprintf(&out, "mode=%s total=%d omitted=%+v used=%d\n", result.Mode, result.Total, result.Omitted, result.BudgetUsed)
	for _, candidate := range result.Candidates {
		fmt.Fprintf(&out, "id=%s score=%g summary=%s stats=%+v\n", candidate.RecordID, candidate.Score, candidate.Summary, candidate.Stats)
		for _, hint := range candidate.Hints {
			fmt.Fprintf(&out, "hint=%s %s %v %v summary=%v\n", hint.SourceKind, hint.Direction, hint.RelationID, hint.RecordID, hint.Summary)
		}
	}
	return []byte(out.String()), nil
}

func TestOneHopDirectionsStatsAndDeletedFiltering(t *testing.T) {
	a, b, c, dead := recordID(t, '1'), recordID(t, '2'), recordID(t, '3'), recordID(t, '4')
	p, other := path(t, "/notes"), path(t, "/other")
	an, ah := node(a, "first", p)
	bn, bh := node(b, "middle", p)
	cn, ch := node(c, "last", other)
	dn, dh := node(dead, "removed", p)
	dn.Deleted = true
	r1 := relation(t, relationID(t, 'a'), model.Next, a, b)
	r2 := relation(t, relationID(t, 'b'), model.Change, b, c)
	r3 := relation(t, relationID(t, 'c'), model.Next, b, dead)
	r4 := relation(t, relationID(t, 'd'), model.End, b, c)
	r4.Deleted = true
	ref, err := model.NewReference("source.md", 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	bn.Reference, bn.ReferenceBase = &ref, p
	g := graphFixture{
		nodes:     map[model.RecordID]Node{a: an, b: bn, c: cn, dead: dn},
		relations: map[model.RecordID][]Relation{b: {r1, r2, r3, r4}},
		links: map[model.RecordID][]InternalLink{b: {
			{From: a, To: b, Kind: AboutLink},
			{From: b, To: c, Kind: ReasonLink},
			{From: dead, To: b, Kind: ReasonLink},
		}},
	}
	ctx := "unresolved"
	build := Builder{Graph: g, Bodies: bodyFixture{ah: "first", bh: "middle", ch: "last", dh: "removed"}, References: typedReferenceFixture{Resolution{Source: "source.md", Start: 0, End: 3, Resolution: ctx}}}
	result, encoded, err := build.Build(context.Background(), "text", 1, []Candidate{{ID: b, Score: 0.7, ScoreKind: "lexical"}}, 10000, renderJSON)
	if err != nil {
		t.Fatal(err)
	}
	if result.BudgetUsed != len(encoded) || result.Total != 1 || len(result.Candidates) != 1 || result.Candidates[0].RecordID != b.String() {
		t.Fatalf("candidate identity or budget changed: %+v", result)
	}
	view := result.Candidates[0]
	if view.Stats.ExternalReference != "unresolved" || view.Stats.IncomingReferences != 1 || view.Stats.RelationCounts["next"] != (DirectionCount{Incoming: 1}) || view.Stats.RelationCounts["change"] != (DirectionCount{Outgoing: 1}) || view.Stats.RelationCounts["end"] != (DirectionCount{}) {
		t.Fatalf("incorrect living statistics: %+v", view.Stats)
	}
	if len(view.Hints) != 4 {
		t.Fatalf("want four living one-hop hints, got %+v", view.Hints)
	}
	var before, after, cross bool
	for _, hint := range view.Hints {
		if hint.RecordID != nil && *hint.RecordID == dead.String() {
			t.Fatal("deleted peer appeared")
		}
		if hint.SourceKind == "relation" && hint.RelationKind != nil && *hint.RelationKind == "next" && hint.Direction == "incoming" {
			before = true
		}
		if hint.SourceKind == "relation" && hint.RelationKind != nil && *hint.RelationKind == "change" && hint.Direction == "outgoing" {
			after = true
			cross = hint.Domain != nil && *hint.Domain == "/other"
		}
	}
	if !before || !after || !cross {
		t.Fatalf("A -> B -> C direction or cross-domain identity lost: %+v", view.Hints)
	}
}

func TestBudgetMeasuresFinalBytesAndKeepsOrder(t *testing.T) {
	a, b := recordID(t, '5'), recordID(t, '6')
	p := path(t, "/notes")
	an, ah := node(a, strings.Repeat("あ", 80), p)
	bn, bh := node(b, "second", p)
	g := graphFixture{nodes: map[model.RecordID]Node{a: an, b: bn}, relations: map[model.RecordID][]Relation{a: {relation(t, relationID(t, 'e'), model.Next, a, b)}}}
	build := Builder{Graph: g, Bodies: bodyFixture{ah: strings.Repeat("あ", 80), bh: "second"}}
	input := []Candidate{{ID: a, Score: 0.1, ScoreKind: "cosine"}, {ID: b, Score: 0.9, ScoreKind: "cosine"}}
	for _, render := range []Renderer{renderJSON, renderText} {
		full, fullBytes, err := build.Build(context.Background(), "embedding", 3, input, 10000, render)
		if err != nil {
			t.Fatal(err)
		}
		if full.BudgetUsed != len(fullBytes) || len(full.Candidates) != 2 || full.Candidates[0].RecordID != a.String() || full.Candidates[1].RecordID != b.String() || full.Omitted.Candidates != 1 {
			t.Fatalf("full result changed rank/total: %+v", full)
		}
		tight, data, err := build.Build(context.Background(), "embedding", 3, input, len(fullBytes)-30, render)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != tight.BudgetUsed || len(data) > len(fullBytes)-30 || !utf8.Valid(data) || !tight.Omitted.Truncated || tight.Candidates[0].RecordID != a.String() {
			t.Fatalf("invalid budgeted output: %+v bytes=%d", tight, len(data))
		}
	}
	if _, _, err := build.Build(context.Background(), "text", 3, input, 1, renderJSON); !errors.Is(err, ErrBudgetTooSmall) {
		t.Fatalf("tiny budget = %v", err)
	}
}

func TestEmptyRangeAndSnapshotFailures(t *testing.T) {
	build := Builder{Graph: graphFixture{}, Bodies: bodyFixture{}}
	result, data, err := build.Build(context.Background(), "text", 0, nil, 4096, renderJSON)
	if err != nil || len(result.Candidates) != 0 || result.BudgetUsed != len(data) {
		t.Fatalf("empty result: %+v, %v", result, err)
	}
	id := recordID(t, '7')
	if _, _, err := build.Build(context.Background(), "text", 1, []Candidate{{ID: id, Score: 0.3, ScoreKind: "lexical"}}, 4096, renderJSON); !errors.Is(err, ErrInconsistentSnapshot) {
		t.Fatalf("missing candidate = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := build.Build(ctx, "text", 1, []Candidate{{ID: id, Score: 0.3, ScoreKind: "lexical"}}, 4096, renderJSON); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled build = %v", err)
	}
}

func TestReasonPayloadAndDeletedRelationOwner(t *testing.T) {
	a, b, c := recordID(t, '8'), recordID(t, '9'), recordID(t, 'a')
	p := path(t, "/notes")
	an, ah := node(a, "first", p)
	bn, bh := node(b, "second", p)
	cn, ch := node(c, "third", p)
	natural, err := model.NewTextRecordReason("見直した理由")
	if err != nil {
		t.Fatal(err)
	}
	bn.Reason = &natural
	ownerID := relationID(t, 'f')
	owner := relation(t, ownerID, model.Change, a, b)
	owner.Deleted = true
	g := graphFixture{
		nodes:  map[model.RecordID]Node{a: an, b: bn, c: cn},
		owners: map[model.RelationID]Relation{ownerID: owner},
		links: map[model.RecordID][]InternalLink{b: {
			{From: a, To: b, Kind: ReasonLink},
			{From: a, To: b, Kind: ReasonLink, OwnerRelation: ownerID},
		}},
	}
	build := Builder{Graph: g, Bodies: bodyFixture{ah: "first", bh: "second", ch: "third"}}
	result, _, err := build.Build(context.Background(), "text", 1, []Candidate{{ID: b, Score: 0.6, ScoreKind: "lexical"}}, 10000, renderJSON)
	if err != nil {
		t.Fatal(err)
	}
	view := result.Candidates[0]
	if view.Stats.IncomingReferences != 1 || len(view.Hints) != 2 {
		t.Fatalf("deleted owner leaked or reasons missing: %+v", view)
	}
	var recordReason, textReason bool
	for _, hint := range view.Hints {
		if hint.SourceKind != "reason" || hint.Reason == nil || !hint.ReasonPresent {
			t.Fatalf("reason payload absent: %+v", hint)
		}
		if hint.Reason.Kind == "record" && hint.Reason.ID != nil && *hint.Reason.ID == b.String() && hint.Direction == "incoming" && hint.RelationID == nil {
			recordReason = true
		}
		if hint.Reason.Kind == "text" && hint.Reason.Text != nil && *hint.Reason.Text == "見直した理由" && hint.Direction == "outgoing" {
			textReason = true
		}
	}
	if !recordReason || !textReason {
		t.Fatalf("reason kinds or directions lost: %+v", view.Hints)
	}
}

func TestLongRelationReasonDoesNotConsumeCoreHint(t *testing.T) {
	a, b := recordID(t, 'b'), recordID(t, 'c')
	p := path(t, "/notes")
	an, ah := node(a, "first", p)
	bn, bh := node(b, "second", p)
	reason, err := model.NewTextRelationReason(strings.Repeat("理由", 1000))
	if err != nil {
		t.Fatal(err)
	}
	relationID := relationID(t, 'e')
	value, err := model.NewRelation(model.RelationSpec{ID: relationID, Kind: model.Change, From: a, To: b, Reason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	g := graphFixture{nodes: map[model.RecordID]Node{a: an, b: bn}, relations: map[model.RecordID][]Relation{a: {{Value: value}}}}
	build := Builder{Graph: g, Bodies: bodyFixture{ah: "first", bh: "second"}}
	result, data, err := build.Build(context.Background(), "text", 1, []Candidate{{ID: a, Score: 0.7, ScoreKind: "lexical"}}, 1000, renderJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 1000 || len(result.Candidates) != 1 || len(result.Candidates[0].Hints) != 1 {
		t.Fatalf("core hint lost under long reason: %+v", result)
	}
	hint := result.Candidates[0].Hints[0]
	if hint.RelationID == nil || *hint.RelationID != relationID.String() || hint.Direction != "outgoing" || !hint.ReasonPresent || hint.Reason == nil || hint.Reason.Kind != "text" {
		t.Fatalf("relation identity, direction, or reason presence lost: %+v", hint)
	}
}

func TestCandidateRecordReasonIsOneOutgoingHint(t *testing.T) {
	a, b := recordID(t, 'd'), recordID(t, 'e')
	p := path(t, "/notes")
	an, ah := node(a, "belief", p)
	bn, bh := node(b, "support", p)
	reason, err := model.NewIDRecordReason(model.ReasonBelief, b)
	if err != nil {
		t.Fatal(err)
	}
	an.Reason = &reason
	g := graphFixture{nodes: map[model.RecordID]Node{a: an, b: bn}, links: map[model.RecordID][]InternalLink{a: {{From: a, To: b, Kind: ReasonLink}}}}
	build := Builder{Graph: g, Bodies: bodyFixture{ah: "belief", bh: "support"}}
	result, _, err := build.Build(context.Background(), "text", 1, []Candidate{{ID: a, Score: 0.5, ScoreKind: "lexical"}}, 10000, renderJSON)
	if err != nil {
		t.Fatal(err)
	}
	hints := result.Candidates[0].Hints
	if len(hints) != 1 || hints[0].SourceKind != "reason" || hints[0].RecordID == nil || *hints[0].RecordID != b.String() || hints[0].Reason == nil || hints[0].Reason.Kind != "record" || hints[0].Reason.ID == nil || *hints[0].Reason.ID != b.String() {
		t.Fatalf("record reason missing or duplicated: %+v", hints)
	}
}
