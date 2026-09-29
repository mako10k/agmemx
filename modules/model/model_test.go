package model

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

func mustRecordID(t *testing.T, text string) RecordID {
	t.Helper()
	id, err := ParseRecordID(text)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustRelationID(t *testing.T, text string) RelationID {
	t.Helper()
	id, err := ParseRelationID(text)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOpaqueIdentifiersAndHash(t *testing.T) {
	record, err := NewRecordID()
	if err != nil || !record.Valid() {
		t.Fatalf("new record ID: %v", err)
	}
	if parsed, err := ParseRecordID(record.String()); err != nil || parsed != record {
		t.Fatalf("record ID round trip: %v", err)
	}
	relation, err := NewRelationID()
	if err != nil || !relation.Valid() {
		t.Fatalf("new relation ID: %v", err)
	}
	if parsed, err := ParseRelationID(relation.String()); err != nil || parsed != relation {
		t.Fatalf("relation ID round trip: %v", err)
	}
	digest := sha256.Sum256([]byte("本文\n"))
	hash := BodyHashFromDigest(digest)
	if parsed, err := ParseBodyHash(hash.String()); err != nil || parsed != hash {
		t.Fatalf("body hash round trip: %v", err)
	}
	for _, text := range []string{"", strings.Repeat("0", 32), strings.ToUpper(record.String()), record.String() + "0", strings.Repeat("g", 32)} {
		if _, err := ParseRecordID(text); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseRecordID(%q) should reject: %v", text, err)
		}
	}
	if _, err := ParseRelationID(strings.Repeat("0", 32)); !errors.Is(err, ErrInvalid) {
		t.Errorf("zero relation ID accepted: %v", err)
	}
	if _, err := ParseBodyHash(strings.ToUpper(hash.String())); !errors.Is(err, ErrInvalid) {
		t.Errorf("uppercase hash accepted: %v", err)
	}
}

func TestRecordKindsReasonsAndIsolation(t *testing.T) {
	id := mustRecordID(t, "11111111111111111111111111111111")
	target := mustRecordID(t, "22222222222222222222222222222222")
	reason, err := NewIDRecordReason(ReasonObservation, target)
	if err != nil {
		t.Fatal(err)
	}
	name := "仮説"
	about := []RecordID{target}
	belief, err := NewRecord(RecordSpec{ID: id, Kind: Belief, Text: "矛盾する仮説", DisplayName: &name, Reason: &reason, About: about})
	if err != nil {
		t.Fatal(err)
	}
	name = "変更後"
	about[0] = id
	returned := belief.About()
	returned[0] = id
	if got, _ := belief.DisplayName(); got != "仮説" || belief.About()[0] != target {
		t.Fatal("record retained mutable caller or result storage")
	}
	if got, ok := belief.Reason(); !ok || got.Kind() != ReasonObservation {
		t.Fatal("belief reason lost")
	}

	reference, err := NewReference("notes/観測.txt", 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRecord(RecordSpec{ID: id, Kind: Observation, Text: "観測", Reference: &reference}); err != nil {
		t.Fatalf("observation with direct Reference: %v", err)
	}
	invalid := []RecordSpec{
		{ID: id, Kind: Observation, Text: "観測"},
		{ID: id, Kind: Observation, Text: "観測", Reference: &reference, Reason: &reason},
		{ID: id, Kind: Belief, Text: "信念", Reference: &reference},
		{ID: id, Kind: Belief, Text: ""},
		{ID: id, Kind: Belief, Text: string([]byte{0xff})},
		{ID: id, Kind: Belief, Text: "信念", About: []RecordID{target, target}},
		{ID: id, Kind: "fact", Text: "信念"},
		{Kind: Belief, Text: "信念"},
	}
	for index, spec := range invalid {
		if _, err := NewRecord(spec); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid record %d accepted: %v", index, err)
		}
	}
}

func TestIntervalReferenceAndReasonConstraints(t *testing.T) {
	interval, err := NewInterval("2026-09-29T10:00:00+09:00", "2026-09-29T01:30:00Z")
	if err != nil || interval.Start() == "" {
		t.Fatalf("valid interval: %v", err)
	}
	for _, endpoints := range [][2]string{
		{"2026-09-29", "2026-09-30T00:00:00Z"},
		{"2026-09-30T00:00:00Z", "2026-09-29T00:00:00Z"},
	} {
		if _, err := NewInterval(endpoints[0], endpoints[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid interval accepted: %v", err)
		}
	}
	for _, span := range []struct {
		source string
		start  int
		end    int
	}{{"", 0, 1}, {"notes.txt", -1, 1}, {"notes.txt", 2, 1}} {
		if _, err := NewReference(span.source, span.start, span.end); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid Reference accepted: %v", err)
		}
	}
	id := mustRecordID(t, "33333333333333333333333333333333")
	if _, err := NewIDRecordReason(ReasonText, id); !errors.Is(err, ErrInvalid) {
		t.Errorf("record ID reason with text kind accepted: %v", err)
	}
	if _, err := NewTextRecordReason(""); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty text reason accepted: %v", err)
	}
	if _, err := NewIDRelationReason(RecordID{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty relation reason ID accepted: %v", err)
	}
}

func TestRelationDistinguishesHistoryAndRetainsGround(t *testing.T) {
	from := mustRecordID(t, "44444444444444444444444444444444")
	to := mustRecordID(t, "55555555555555555555555555555555")
	id := mustRelationID(t, "66666666666666666666666666666666")
	reason, err := NewTextRelationReason("新しい資料を確認")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := NewReference("notes/source.txt", 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []RelationKind{Next, Change, End} {
		relation, err := NewRelation(RelationSpec{ID: id, Kind: kind, From: from, To: to, Reason: &reason, EvidenceReference: &reference})
		if err != nil {
			t.Fatalf("%s relation: %v", kind, err)
		}
		if relation.Kind() != kind || relation.From() != from || relation.To() != to {
			t.Fatalf("%s direction or kind lost", kind)
		}
		if ground, ok := relation.Reason(); !ok || ground.Kind() != RelationReasonText {
			t.Fatalf("%s relation reason lost", kind)
		}
		if _, ok := relation.EvidenceReference(); !ok {
			t.Fatalf("%s external evidence lost", kind)
		}
	}
	for _, kind := range []RelationKind{Change, End} {
		if _, err := NewRelation(RelationSpec{ID: id, Kind: kind, From: from, To: from}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s accepted one ID for old and new records: %v", kind, err)
		}
	}
	if _, err := NewRelation(RelationSpec{ID: id, Kind: "contradiction", From: from, To: to}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unsupported relation kind accepted: %v", err)
	}
}
