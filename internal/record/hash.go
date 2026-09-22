// Package record builds the canonical content of beliefs and observations.
package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// Interval is a caller-supplied period stored verbatim.
type Interval struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Reference is a span in a file under the resolved directory.
type Reference struct {
	Source string `json:"source"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
}

// Reason is a belief's optional ground.
type Reason struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
}

// Object is one stored belief or observation.
type Object struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Domain    string     `json:"domain"`
	Text      string     `json:"text"`
	Interval  *Interval  `json:"interval"`
	Reference *Reference `json:"reference,omitempty"`
	Reason    *Reason    `json:"reason,omitempty"`
	About     []string   `json:"about,omitempty"`
}

// ContentSHA256 is the hash of the canonical content, excluding id and domain.
func ContentSHA256(obj Object) (string, error) {
	body, err := contentJSON(obj)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func contentJSON(obj Object) ([]byte, error) {
	payload := map[string]any{
		"text":     obj.Text,
		"interval": intervalValue(obj.Interval),
	}
	if obj.Kind == "observation" {
		payload["reference"] = map[string]any{
			"source": obj.Reference.Source,
			"start":  obj.Reference.Start,
			"end":    obj.Reference.End,
		}
	} else {
		payload["about"] = append([]string(nil), obj.About...)
		payload["reason"] = reasonValue(obj.Reason)
	}
	return json.Marshal(payload)
}

func intervalValue(interval *Interval) any {
	if interval == nil {
		return nil
	}
	return map[string]string{"start": interval.Start, "end": interval.End}
}

func reasonValue(reason *Reason) any {
	if reason == nil {
		return nil
	}
	out := map[string]string{"kind": reason.Kind}
	if reason.Kind == "text" {
		out["text"] = reason.Text
	} else {
		out["id"] = reason.ID
	}
	return out
}

// SortAbout returns a sorted copy with duplicates removed. The boolean is false when a duplicate was present.
func SortAbout(ids []string) ([]string, bool) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return nil, false
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, true
}
