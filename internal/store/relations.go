package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// NextEdge is one same-domain next relation stored outside object bodies.
type NextEdge struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

type relationFile struct {
	Domain string     `json:"domain"`
	Next   []NextEdge `json:"next"`
}

// RelationPath is the next-relation file for one canonical domain.
func RelationPath(dataHome, domain string) string {
	sum := sha256.Sum256([]byte(domain))
	return filepath.Join(LayoutOf(dataHome).Root, "relations", hex.EncodeToString(sum[:])+".json")
}

// ObjectDomain returns the domain recorded on the object when it was committed.
// ok is false when the id is absent or the object has no domain.
func ObjectDomain(dataHome, id string) (string, bool, error) {
	raw, err := Load(dataHome, id)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var obj struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", false, err
	}
	if obj.Domain == "" {
		return "", false, nil
	}
	return obj.Domain, true, nil
}

// PutNext records one next edge for domain. Object files are not rewritten.
// A repeated edge is left as the existing record.
func PutNext(dataHome, domain, from, to string) error {
	if domain == "" || from == "" || to == "" || from == to {
		return errors.New("invalid next relation")
	}
	dir := filepath.Join(LayoutOf(dataHome).Root, "relations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := RelationPath(dataHome, domain)
	current := relationFile{Domain: domain, Next: []NextEdge{}}
	if _, err := os.Stat(path); err == nil {
		loaded, readErr := readRelations(path)
		if readErr != nil {
			return readErr
		}
		if loaded.Domain != domain {
			return errors.New("relation index identity mismatch")
		}
		current = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	edge := NextEdge{Kind: "next", From: from, To: to}
	for _, existing := range current.Next {
		if existing == edge {
			return nil
		}
	}
	current.Next = append(current.Next, edge)
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeAtomic(path, body)
}

// LoadNext returns the next edges stored for domain. A missing file is empty.
func LoadNext(dataHome, domain string) ([]NextEdge, error) {
	path := RelationPath(dataHome, domain)
	current, err := readRelations(path)
	if errors.Is(err, os.ErrNotExist) {
		return []NextEdge{}, nil
	}
	if err != nil {
		return nil, err
	}
	if current.Domain != domain {
		return nil, errors.New("relation index identity mismatch")
	}
	return current.Next, nil
}

func readRelations(path string) (relationFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return relationFile{}, err
	}
	var decoded relationFile
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return relationFile{}, err
	}
	if decoded.Next == nil {
		decoded.Next = []NextEdge{}
	}
	return decoded, nil
}
