package embed

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// ErrFixture means the fixture file does not match the contract.
var ErrFixture = errors.New("embed fixture invalid")

// FixtureVector returns the vector whose key is text.
func FixtureVector(path, model, text string) ([]float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrFixture
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc struct {
		Model     string               `json:"model"`
		Dimension int                  `json:"dimension"`
		Vectors   map[string][]float64 `json:"vectors"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, ErrFixture
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, ErrFixture
	}
	if doc.Model != model || doc.Dimension <= 0 || doc.Vectors == nil {
		return nil, ErrFixture
	}
	vec, ok := doc.Vectors[text]
	if !ok || len(vec) != doc.Dimension {
		return nil, ErrFixture
	}
	for _, other := range doc.Vectors {
		if len(other) != doc.Dimension {
			return nil, ErrFixture
		}
	}
	return vec, nil
}
