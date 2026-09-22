// Package store keeps object bodies separate from the domain index.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"agmemx/internal/domain"
)

// Layout is the on-disk split under the XDG data directory.
type Layout struct {
	Root    string
	Objects string
	Domains string
}

// LayoutOf returns the objects directory and the domain index directory.
func LayoutOf(dataHome string) Layout {
	root := filepath.Join(dataHome, "agmemx")
	return Layout{
		Root:    root,
		Objects: filepath.Join(root, "objects"),
		Domains: filepath.Join(root, "domains"),
	}
}

// IndexPath is the domain index file for one canonical directory.
func IndexPath(dataHome, domain string) string {
	sum := sha256.Sum256([]byte(domain))
	return filepath.Join(LayoutOf(dataHome).Domains, hex.EncodeToString(sum[:])+".json")
}

type indexFile struct {
	Domain string   `json:"domain"`
	IDs    []string `json:"ids"`
}

// InitDomain creates an empty domain index and the objects directory.
// A second call for the same domain leaves the index bytes unchanged.
func InitDomain(dataHome, domain string) error {
	layout := LayoutOf(dataHome)
	if err := os.MkdirAll(layout.Objects, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(layout.Domains, 0o700); err != nil {
		return err
	}
	path := IndexPath(dataHome, domain)
	if _, err := os.Stat(path); err == nil {
		current, readErr := readIndex(path)
		if readErr != nil {
			return readErr
		}
		if current.Domain != domain {
			return errors.New("domain index identity mismatch")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, err := json.Marshal(indexFile{Domain: domain, IDs: []string{}})
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeAtomic(path, body)
}

func readIndex(path string) (indexFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return indexFile{}, err
	}
	var decoded indexFile
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return indexFile{}, err
	}
	if decoded.IDs == nil {
		decoded.IDs = []string{}
	}
	return decoded, nil
}

func writeAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// Commit stores one object body and adds its id to the domain index.
// The body is removed again when the index update fails.
func Commit(dataHome, domainName, id string, body []byte) error {
	if err := InitDomain(dataHome, domainName); err != nil {
		return err
	}
	path := filepath.Join(LayoutOf(dataHome).Objects, id+".json")
	if err := writeAtomic(path, body); err != nil {
		return err
	}
	if err := appendID(dataHome, domainName, id); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// Load reads one object body.
func Load(dataHome, id string) ([]byte, error) {
	return os.ReadFile(filepath.Join(LayoutOf(dataHome).Objects, id+".json"))
}

// Exists reports whether an object file is present.
func Exists(dataHome, id string) bool {
	_, err := os.Stat(filepath.Join(LayoutOf(dataHome).Objects, id+".json"))
	return err == nil
}

// IDsInSubtree returns object ids whose domain is root or inside root.
func IDsInSubtree(dataHome, root string) ([]string, error) {
	entries, err := os.ReadDir(LayoutOf(dataHome).Domains)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		current, err := readIndex(filepath.Join(LayoutOf(dataHome).Domains, entry.Name()))
		if err != nil {
			return nil, err
		}
		if domain.InSubtree(root, current.Domain) {
			ids = append(ids, current.IDs...)
		}
	}
	return ids, nil
}

func appendID(dataHome, domainName, id string) error {
	path := IndexPath(dataHome, domainName)
	current, err := readIndex(path)
	if err != nil {
		return err
	}
	for _, existing := range current.IDs {
		if existing == id {
			return nil
		}
	}
	current.IDs = append(current.IDs, id)
	sort.Strings(current.IDs)
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeAtomic(path, body)
}
