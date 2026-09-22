// Package embed stores regenerable vectors outside the memory objects.
package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Key identifies one vector cache namespace and one text.
type Key struct {
	Provider string
	BaseURL  string
	Model    string
	Text     string
}

// ErrDimension means a vector length differs from the namespace already stored.
var ErrDimension = errors.New("embed dimension mismatch")

type stored struct {
	Vector []float64 `json:"vector"`
}

// Put writes one vector. The namespace dimension is fixed by the first vector.
func Put(cacheHome string, key Key, vector []float64) error {
	return writeVector(namespaceDir(cacheHome, key), key, vector)
}

// StagePut writes a vector into an unpublished stage for key's namespace.
// A dimension already published for that namespace is enforced.
// The published namespace is not modified.
func StagePut(cacheHome string, key Key, vector []float64) error {
	dir := stageDir(cacheHome, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := seedDimension(cacheHome, key, dir); err != nil {
		return err
	}
	return writeVector(dir, key, vector)
}

// PublishStage installs a completed stage into the namespace and removes the stage.
// A missing stage publishes nothing. An existing namespace keeps vectors that were not staged.
func PublishStage(cacheHome string, key Key) error {
	stage := stageDir(cacheHome, key)
	if _, err := os.Stat(stage); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	dest := namespaceDir(cacheHome, key)
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		return os.Rename(stage, dest)
	} else if err != nil {
		return err
	}
	return mergeStage(stage, dest)
}

// DropStage removes an unpublished stage. A missing stage is not an error.
// The published namespace is left in place.
func DropStage(cacheHome string, key Key) error {
	return os.RemoveAll(stageDir(cacheHome, key))
}

func writeVector(dir string, key Key, vector []float64) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dimPath := filepath.Join(dir, "dimension")
	rawDim := strconv.Itoa(len(vector))
	existing, err := os.ReadFile(dimPath)
	if err == nil {
		if string(existing) != rawDim {
			return ErrDimension
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err := os.WriteFile(dimPath, []byte(rawDim), 0o600); err != nil {
		return err
	}
	body, err := json.Marshal(stored{Vector: vector})
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeAtomic(filepath.Join(dir, textName(key.Text)), body)
}

func seedDimension(cacheHome string, key Key, stage string) error {
	destDim := filepath.Join(stage, "dimension")
	if _, err := os.Stat(destDim); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(namespaceDir(cacheHome, key), "dimension"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(destDim, raw, 0o600)
}

func mergeStage(stage, dest string) error {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(stage, entry.Name()))
		if err != nil {
			return err
		}
		if err := writeAtomic(filepath.Join(dest, entry.Name()), raw); err != nil {
			return err
		}
	}
	return os.RemoveAll(stage)
}

// Get returns a stored vector.
func Get(cacheHome string, key Key) ([]float64, bool, error) {
	raw, err := os.ReadFile(filepath.Join(namespaceDir(cacheHome, key), textName(key.Text)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var decoded stored
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false, err
	}
	return decoded.Vector, true, nil
}

// Delete removes one cached vector. A missing vector is not an error.
func Delete(cacheHome string, key Key) error {
	err := os.Remove(filepath.Join(namespaceDir(cacheHome, key), textName(key.Text)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func namespaceDir(cacheHome string, key Key) string {
	sum := sha256.Sum256([]byte(key.Provider + "\n" + key.BaseURL + "\n" + key.Model))
	return filepath.Join(cacheHome, "agmemx", hex.EncodeToString(sum[:]))
}

func stageDir(cacheHome string, key Key) string {
	sum := sha256.Sum256([]byte(key.Provider + "\n" + key.BaseURL + "\n" + key.Model))
	return filepath.Join(cacheHome, "agmemx", "stage-"+hex.EncodeToString(sum[:]))
}

func textName(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:]) + ".json"
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
