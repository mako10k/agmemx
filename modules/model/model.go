// Package model defines the intrinsic values and invariants of memory records and relations.
// R: Keep memory values valid independently of storage, domains, CLI syntax, and providers.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrInvalid marks a value that violates the memory model's intrinsic rules.
var ErrInvalid = errors.New("invalid memory value")

// RecordID and RelationID are distinct opaque 128-bit identities.
type RecordID struct{ bytes [16]byte }
type RelationID struct{ bytes [16]byte }

// BodyHash identifies the exact UTF-8 bytes of a record's text by SHA-256.
// Computing the digest and storing the bytes belong to the body module.
type BodyHash struct{ bytes [32]byte }

// Revision is a monotonically increasing ledger revision. Zero is the initial state.
type Revision uint64

func NewRecordID() (RecordID, error) {
	bytes, err := randomID()
	return RecordID{bytes: bytes}, err
}

func NewRelationID() (RelationID, error) {
	bytes, err := randomID()
	return RelationID{bytes: bytes}, err
}

func randomID() ([16]byte, error) {
	for {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return [16]byte{}, err
		}
		if bytes != ([16]byte{}) {
			return bytes, nil
		}
	}
}

func ParseRecordID(text string) (RecordID, error) {
	bytes, err := parseHex(text, 16)
	if err != nil {
		return RecordID{}, err
	}
	var id RecordID
	copy(id.bytes[:], bytes)
	if !id.Valid() {
		return RecordID{}, invalid("zero record ID")
	}
	return id, nil
}

func ParseRelationID(text string) (RelationID, error) {
	bytes, err := parseHex(text, 16)
	if err != nil {
		return RelationID{}, err
	}
	var id RelationID
	copy(id.bytes[:], bytes)
	if !id.Valid() {
		return RelationID{}, invalid("zero relation ID")
	}
	return id, nil
}

func ParseBodyHash(text string) (BodyHash, error) {
	bytes, err := parseHex(text, 32)
	if err != nil {
		return BodyHash{}, err
	}
	var hash BodyHash
	copy(hash.bytes[:], bytes)
	return hash, nil
}

// BodyHashFromDigest wraps the SHA-256 bytes produced by the body module.
func BodyHashFromDigest(digest [32]byte) BodyHash { return BodyHash{bytes: digest} }

func (id RecordID) Valid() bool      { return id.bytes != ([16]byte{}) }
func (id RelationID) Valid() bool    { return id.bytes != ([16]byte{}) }
func (id RecordID) String() string   { return hex.EncodeToString(id.bytes[:]) }
func (id RelationID) String() string { return hex.EncodeToString(id.bytes[:]) }
func (hash BodyHash) String() string { return hex.EncodeToString(hash.bytes[:]) }

func parseHex(text string, byteCount int) ([]byte, error) {
	if len(text) != byteCount*2 || strings.ToLower(text) != text {
		return nil, invalid("expected lowercase hex with exact length")
	}
	decoded, err := hex.DecodeString(text)
	if err != nil {
		return nil, invalid("expected lowercase hex")
	}
	return decoded, nil
}

// RecordKind names the two forms of memory without assigning a special contradiction type.
type RecordKind string

const (
	Belief      RecordKind = "belief"
	Observation RecordKind = "observation"
)

func (kind RecordKind) Valid() bool { return kind == Belief || kind == Observation }

// RelationKind keeps chronological order distinct from change and termination.
type RelationKind string

const (
	Next   RelationKind = "next"
	Change RelationKind = "change"
	End    RelationKind = "end"
)

func (kind RelationKind) Valid() bool { return kind == Next || kind == Change || kind == End }

// Interval preserves the caller's RFC3339 endpoints; it does not define their domain meaning.
type Interval struct {
	start string
	end   string
}

func NewInterval(start, end string) (Interval, error) {
	value := Interval{start: start, end: end}
	if err := value.validate(); err != nil {
		return Interval{}, err
	}
	return value, nil
}

func (value Interval) Start() string { return value.start }
func (value Interval) End() string   { return value.end }

func (value Interval) validate() error {
	start, err := time.Parse(time.RFC3339, value.start)
	if err != nil {
		return invalid("interval start must be RFC3339")
	}
	end, err := time.Parse(time.RFC3339, value.end)
	if err != nil {
		return invalid("interval end must be RFC3339")
	}
	if end.Before(start) {
		return invalid("interval end precedes start")
	}
	return nil
}

// Reference identifies a half-open Unicode-scalar range in an external source.
// The reference module owns path resolution and checking the range against source content.
type Reference struct {
	source string
	start  int
	end    int
}

func NewReference(source string, start, end int) (Reference, error) {
	value := Reference{source: source, start: start, end: end}
	if err := value.validate(); err != nil {
		return Reference{}, err
	}
	return value, nil
}

func (value Reference) Source() string { return value.source }
func (value Reference) Start() int     { return value.start }
func (value Reference) End() int       { return value.end }

func (value Reference) validate() error {
	if value.source == "" || !utf8.ValidString(value.source) {
		return invalid("reference source must be nonempty UTF-8")
	}
	if value.start < 0 || value.end < value.start {
		return invalid("reference range must be a nonnegative half-open range")
	}
	return nil
}

// RecordReason is either natural text or the identity and kind of a memory node.
type RecordReason struct {
	kind RecordReasonKind
	id   RecordID
	text string
}

type RecordReasonKind string

const (
	ReasonText        RecordReasonKind = "text"
	ReasonBelief      RecordReasonKind = "belief"
	ReasonObservation RecordReasonKind = "observation"
)

func NewTextRecordReason(text string) (RecordReason, error) {
	value := RecordReason{kind: ReasonText, text: text}
	return value, value.validate()
}

func NewIDRecordReason(kind RecordReasonKind, id RecordID) (RecordReason, error) {
	value := RecordReason{kind: kind, id: id}
	return value, value.validate()
}

func (value RecordReason) Kind() RecordReasonKind { return value.kind }
func (value RecordReason) ID() (RecordID, bool) {
	return value.id, value.kind == ReasonBelief || value.kind == ReasonObservation
}
func (value RecordReason) Text() (string, bool) { return value.text, value.kind == ReasonText }

func (value RecordReason) validate() error {
	switch value.kind {
	case ReasonText:
		if value.text == "" || !utf8.ValidString(value.text) || value.id.Valid() {
			return invalid("text record reason needs text only")
		}
	case ReasonBelief, ReasonObservation:
		if !value.id.Valid() || value.text != "" {
			return invalid("record reason needs an ID only")
		}
	default:
		return invalid("unknown record reason kind")
	}
	return nil
}

// RelationReason is either natural text or a memory-node identity.
type RelationReason struct {
	kind RelationReasonKind
	id   RecordID
	text string
}

type RelationReasonKind string

const (
	RelationReasonText   RelationReasonKind = "text"
	RelationReasonRecord RelationReasonKind = "record"
)

func NewTextRelationReason(text string) (RelationReason, error) {
	value := RelationReason{kind: RelationReasonText, text: text}
	return value, value.validate()
}

func NewIDRelationReason(id RecordID) (RelationReason, error) {
	value := RelationReason{kind: RelationReasonRecord, id: id}
	return value, value.validate()
}

func (value RelationReason) Kind() RelationReasonKind { return value.kind }
func (value RelationReason) ID() (RecordID, bool) {
	return value.id, value.kind == RelationReasonRecord
}
func (value RelationReason) Text() (string, bool) {
	return value.text, value.kind == RelationReasonText
}

func (value RelationReason) validate() error {
	switch value.kind {
	case RelationReasonText:
		if value.text == "" || !utf8.ValidString(value.text) || value.id.Valid() {
			return invalid("text relation reason needs text only")
		}
	case RelationReasonRecord:
		if !value.id.Valid() || value.text != "" {
			return invalid("relation reason needs an ID only")
		}
	default:
		return invalid("unknown relation reason kind")
	}
	return nil
}

// RecordSpec is input to NewRecord. NewRecord copies its optional values and about IDs.
type RecordSpec struct {
	ID          RecordID
	Kind        RecordKind
	Text        string
	DisplayName *string
	Interval    *Interval
	Reference   *Reference
	Reason      *RecordReason
	About       []RecordID
}

// Record is a validated memory value, independent of membership and storage layout.
type Record struct {
	id          RecordID
	kind        RecordKind
	text        string
	displayName *string
	interval    *Interval
	reference   *Reference
	reason      *RecordReason
	about       []RecordID
}

func NewRecord(spec RecordSpec) (Record, error) {
	if !spec.ID.Valid() || !spec.Kind.Valid() {
		return Record{}, invalid("record ID or kind")
	}
	if spec.Text == "" || !utf8.ValidString(spec.Text) {
		return Record{}, invalid("record text must be nonempty UTF-8")
	}
	if spec.DisplayName != nil && !utf8.ValidString(*spec.DisplayName) {
		return Record{}, invalid("display name must be UTF-8")
	}
	if spec.Interval != nil {
		if err := spec.Interval.validate(); err != nil {
			return Record{}, err
		}
	}
	if spec.Reference != nil {
		if err := spec.Reference.validate(); err != nil {
			return Record{}, err
		}
	}
	if spec.Reason != nil {
		if err := spec.Reason.validate(); err != nil {
			return Record{}, err
		}
	}
	if spec.Kind == Observation {
		if spec.Reference == nil || spec.Reason != nil {
			return Record{}, invalid("observation needs a direct Reference and no belief reason")
		}
	} else if spec.Reference != nil {
		return Record{}, invalid("belief cannot have a direct Reference")
	}
	seen := make(map[RecordID]struct{}, len(spec.About))
	for _, id := range spec.About {
		if !id.Valid() {
			return Record{}, invalid("about contains a zero ID")
		}
		if _, ok := seen[id]; ok {
			return Record{}, invalid("about contains a duplicate ID")
		}
		seen[id] = struct{}{}
	}
	return Record{
		id: spec.ID, kind: spec.Kind, text: spec.Text,
		displayName: clone(spec.DisplayName), interval: clone(spec.Interval),
		reference: clone(spec.Reference), reason: clone(spec.Reason),
		about: append([]RecordID{}, spec.About...),
	}, nil
}

func (value Record) ID() RecordID      { return value.id }
func (value Record) Kind() RecordKind  { return value.kind }
func (value Record) Text() string      { return value.text }
func (value Record) About() []RecordID { return append([]RecordID{}, value.about...) }
func (value Record) DisplayName() (string, bool) {
	if value.displayName == nil {
		return "", false
	}
	return *value.displayName, true
}
func (value Record) Interval() (Interval, bool) {
	if value.interval == nil {
		return Interval{}, false
	}
	return *value.interval, true
}
func (value Record) Reference() (Reference, bool) {
	if value.reference == nil {
		return Reference{}, false
	}
	return *value.reference, true
}
func (value Record) Reason() (RecordReason, bool) {
	if value.reason == nil {
		return RecordReason{}, false
	}
	return *value.reason, true
}

// RelationSpec is input to NewRelation.
type RelationSpec struct {
	ID                RelationID
	Kind              RelationKind
	From              RecordID
	To                RecordID
	Reason            *RelationReason
	EvidenceReference *Reference
}

// Relation is a validated directed connection between two memory records.
type Relation struct {
	id                RelationID
	kind              RelationKind
	from              RecordID
	to                RecordID
	reason            *RelationReason
	evidenceReference *Reference
}

func NewRelation(spec RelationSpec) (Relation, error) {
	if !spec.ID.Valid() || !spec.Kind.Valid() || !spec.From.Valid() || !spec.To.Valid() {
		return Relation{}, invalid("relation ID, kind, or endpoint")
	}
	if (spec.Kind == Change || spec.Kind == End) && spec.From == spec.To {
		return Relation{}, invalid("change and end need a new record ID")
	}
	if spec.Reason != nil {
		if err := spec.Reason.validate(); err != nil {
			return Relation{}, err
		}
	}
	if spec.EvidenceReference != nil {
		if err := spec.EvidenceReference.validate(); err != nil {
			return Relation{}, err
		}
	}
	return Relation{
		id: spec.ID, kind: spec.Kind, from: spec.From, to: spec.To,
		reason: clone(spec.Reason), evidenceReference: clone(spec.EvidenceReference),
	}, nil
}

func (value Relation) ID() RelationID     { return value.id }
func (value Relation) Kind() RelationKind { return value.kind }
func (value Relation) From() RecordID     { return value.from }
func (value Relation) To() RecordID       { return value.to }
func (value Relation) Reason() (RelationReason, bool) {
	if value.reason == nil {
		return RelationReason{}, false
	}
	return *value.reason, true
}
func (value Relation) EvidenceReference() (Reference, bool) {
	if value.evidenceReference == nil {
		return Reference{}, false
	}
	return *value.evidenceReference, true
}

func clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalid, message) }
