package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"agmemx/internal/domain"
)

// SubtreeMembership maps each object id listed by a domain index inside root
// to the domain path stored in that index.
func SubtreeMembership(dataHome, root string) (map[string]string, error) {
	indexes, err := readIndexes(dataHome)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, idx := range indexes {
		if !domain.InSubtree(root, idx.Domain) {
			continue
		}
		for _, id := range idx.IDs {
			prev, ok := out[id]
			if !ok || idx.Domain < prev {
				out[id] = idx.Domain
			}
		}
	}
	return out, nil
}

// Relocate moves id from one domain index to another.
// The object file is not read or written.
func Relocate(dataHome, fromDomain, toDomain, id string) error {
	fromPath := IndexPath(dataHome, fromDomain)
	fromIdx, err := readIndex(fromPath)
	if err != nil {
		return err
	}
	if fromIdx.Domain != fromDomain {
		return errors.New("domain index identity mismatch")
	}
	if !containsID(fromIdx.IDs, id) {
		return errors.New("id is not in the source domain index")
	}
	if fromDomain == toDomain {
		return nil
	}
	nextFrom := withoutID(fromIdx.IDs, id)

	toPath := IndexPath(dataHome, toDomain)
	prevTo, existed, err := readIfExists(toPath)
	if err != nil {
		return err
	}
	if err := InitDomain(dataHome, toDomain); err != nil {
		return err
	}
	toIdx, err := readIndex(toPath)
	if err != nil {
		removeNewIndex(toPath, existed)
		return err
	}
	if toIdx.Domain != toDomain {
		removeNewIndex(toPath, existed)
		return errors.New("domain index identity mismatch")
	}
	toBody, err := marshalIndex(indexFile{Domain: toIdx.Domain, IDs: withID(toIdx.IDs, id)})
	if err != nil {
		removeNewIndex(toPath, existed)
		return err
	}
	fromBody, err := marshalIndex(indexFile{Domain: fromIdx.Domain, IDs: nextFrom})
	if err != nil {
		removeNewIndex(toPath, existed)
		return err
	}
	if err := writeAtomic(toPath, toBody); err != nil {
		removeNewIndex(toPath, existed)
		return err
	}
	if err := writeAtomic(fromPath, fromBody); err != nil {
		if existed {
			_ = writeAtomic(toPath, prevTo)
		} else {
			_ = os.Remove(toPath)
		}
		return err
	}
	return nil
}

func readIndexes(dataHome string) ([]indexFile, error) {
	entries, err := os.ReadDir(LayoutOf(dataHome).Domains)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []indexFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		current, err := readIndex(filepath.Join(LayoutOf(dataHome).Domains, entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, current)
	}
	return out, nil
}

func readIfExists(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func removeNewIndex(path string, existed bool) {
	if !existed {
		_ = os.Remove(path)
	}
}

func containsID(ids []string, id string) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

func withoutID(ids []string, id string) []string {
	next := make([]string, 0, len(ids))
	for _, existing := range ids {
		if existing != id {
			next = append(next, existing)
		}
	}
	return next
}

func withID(ids []string, id string) []string {
	if containsID(ids, id) {
		return append([]string{}, ids...)
	}
	next := append(append([]string{}, ids...), id)
	sort.Strings(next)
	return next
}

func marshalIndex(idx indexFile) ([]byte, error) {
	if idx.IDs == nil {
		idx.IDs = []string{}
	}
	body, err := json.Marshal(idx)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
