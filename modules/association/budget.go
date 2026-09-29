package association

import (
	"context"
	"fmt"
	"unicode/utf8"
)

type selection struct {
	mode          string
	total         int
	values        []prepared
	included      int
	hints         []map[int]bool
	summaries     []string
	hintSummaries []map[int]string
	hintReasons   []map[int]string
	hintContexts  []map[int]string
}

func (s selection) copy() selection {
	next := s
	next.summaries = append([]string(nil), s.summaries...)
	next.hints = make([]map[int]bool, len(s.hints))
	next.hintSummaries = make([]map[int]string, len(s.hintSummaries))
	next.hintReasons = make([]map[int]string, len(s.hintReasons))
	next.hintContexts = make([]map[int]string, len(s.hintContexts))
	for i := range s.hints {
		next.hints[i] = make(map[int]bool, len(s.hints[i]))
		for j, included := range s.hints[i] {
			next.hints[i][j] = included
		}
		next.hintSummaries[i] = make(map[int]string, len(s.hintSummaries[i]))
		for j, summary := range s.hintSummaries[i] {
			next.hintSummaries[i][j] = summary
		}
		next.hintReasons[i] = make(map[int]string, len(s.hintReasons[i]))
		for j, reason := range s.hintReasons[i] {
			next.hintReasons[i][j] = reason
		}
		next.hintContexts[i] = make(map[int]string, len(s.hintContexts[i]))
		for j, context := range s.hintContexts[i] {
			next.hintContexts[i][j] = context
		}
	}
	return next
}

func (s selection) result() Result {
	result := Result{Mode: s.mode, Total: s.total, Candidates: make([]CandidateView, 0, s.included)}
	result.Omitted.Candidates = s.total - s.included
	for i, item := range s.values {
		if i >= s.included {
			result.Omitted.Hints += len(item.hints)
			if item.summary != "" {
				result.Omitted.Summaries++
			}
			for _, hint := range item.hints {
				if hint.Summary != nil && *hint.Summary != "" {
					result.Omitted.Summaries++
				}
				if hint.Reason != nil && hint.Reason.Text != nil && *hint.Reason.Text != "" {
					result.Omitted.Summaries++
				}
				if hint.EvidenceReference != nil && hint.EvidenceReference.Context != nil && *hint.EvidenceReference.Context != "" {
					result.Omitted.Summaries++
				}
			}
			continue
		}
		view := item.view
		view.Summary = s.summaries[i]
		view.Hints = make([]Hint, 0, len(s.hints[i]))
		if item.summary != "" && view.Summary != item.summary {
			result.Omitted.Summaries++
		}
		for j, hint := range item.hints {
			if !s.hints[i][j] {
				result.Omitted.Hints++
				if hint.Summary != nil && *hint.Summary != "" {
					result.Omitted.Summaries++
				}
				if hint.Reason != nil && hint.Reason.Text != nil && *hint.Reason.Text != "" {
					result.Omitted.Summaries++
				}
				if hint.EvidenceReference != nil && hint.EvidenceReference.Context != nil && *hint.EvidenceReference.Context != "" {
					result.Omitted.Summaries++
				}
				continue
			}
			fullSummary := hint.Summary
			hint.Summary = nil
			if fullSummary != nil && *fullSummary != "" {
				if value := s.hintSummaries[i][j]; value != "" {
					hint.Summary = &value
				}
				if hint.Summary == nil || *hint.Summary != *fullSummary {
					result.Omitted.Summaries++
				}
			}
			if hint.Reason != nil && hint.Reason.Text != nil && *hint.Reason.Text != "" {
				fullText := *hint.Reason.Text
				copyReason := *hint.Reason
				copyReason.Text = nil
				if value := s.hintReasons[i][j]; value != "" {
					copyReason.Text = &value
				}
				hint.Reason = &copyReason
				if copyReason.Text == nil || *copyReason.Text != fullText {
					result.Omitted.Summaries++
				}
			}
			if hint.EvidenceReference != nil && hint.EvidenceReference.Context != nil && *hint.EvidenceReference.Context != "" {
				fullContext := *hint.EvidenceReference.Context
				copyReference := *hint.EvidenceReference
				copyReference.Context = nil
				if value := s.hintContexts[i][j]; value != "" {
					copyReference.Context = &value
				}
				hint.EvidenceReference = &copyReference
				if copyReference.Context == nil || *copyReference.Context != fullContext {
					result.Omitted.Summaries++
				}
			}
			view.Hints = append(view.Hints, hint)
		}
		result.Candidates = append(result.Candidates, view)
	}
	result.Omitted.Truncated = result.Omitted.Candidates > 0 || result.Omitted.Hints > 0 || result.Omitted.Summaries > 0
	return result
}

func (s selection) measure(render Renderer) (Result, []byte, error) {
	result := s.result()
	for attempts := 0; attempts < 16; attempts++ {
		data, err := render(result)
		if err != nil {
			return Result{}, nil, err
		}
		if !utf8.Valid(data) {
			return Result{}, nil, fmt.Errorf("%w: renderer returned invalid UTF-8", ErrInvalidInput)
		}
		if len(data) == result.BudgetUsed {
			return result, data, nil
		}
		result.BudgetUsed = len(data)
	}
	return Result{}, nil, fmt.Errorf("%w: renderer budget_used did not stabilize", ErrInvalidInput)
}

type choice struct {
	state  selection
	weight float64
	cost   int
	index  int
	key    string
}

func (s selection) bestFitting(ctx context.Context, budget int, render Renderer) (selection, bool, error) {
	if err := ctx.Err(); err != nil {
		return selection{}, false, err
	}
	_, current, err := s.measure(render)
	if err != nil {
		return selection{}, false, err
	}
	var best choice
	found := false
	consider := func(trial selection, weight float64, index int, key string) error {
		_, data, err := trial.measure(render)
		if err != nil {
			return err
		}
		if len(data) > budget {
			return nil
		}
		cost := len(data) - len(current)
		if cost < 1 {
			cost = 1
		}
		if !found || weight/float64(cost) > best.weight/float64(best.cost) ||
			(weight/float64(cost) == best.weight/float64(best.cost) && (index < best.index || (index == best.index && key < best.key))) {
			best, found = choice{state: trial, weight: weight, cost: cost, index: index, key: key}, true
		}
		return nil
	}
	if s.included < len(s.values) {
		trial := s.copy()
		trial.included++
		if err := consider(trial, s.values[s.included].weight, s.included, "candidate"); err != nil {
			return selection{}, false, err
		}
	}
	for i := 0; i < s.included; i++ {
		item := s.values[i]
		if item.summary != "" && s.summaries[i] == "" {
			trial, ok, err := s.fitSummary(i, -1, item.summary, budget, render)
			if err != nil {
				return selection{}, false, err
			}
			if ok {
				if err := consider(trial, item.weight, i, "summary"); err != nil {
					return selection{}, false, err
				}
			}
		}
		for j, hint := range item.hints {
			if !s.hints[i][j] {
				trial := s.copy()
				trial.hints[i][j] = true
				if err := consider(trial, item.weight+hintWeight(hint), i, stableKey(hint)); err != nil {
					return selection{}, false, err
				}
				continue
			}
			if hint.Summary != nil && *hint.Summary != "" && s.hintSummaries[i][j] == "" {
				trial, ok, err := s.fitSummary(i, j, *hint.Summary, budget, render)
				if err != nil {
					return selection{}, false, err
				}
				if ok {
					if err := consider(trial, item.weight+hintWeight(hint), i, "summary:"+stableKey(hint)); err != nil {
						return selection{}, false, err
					}
				}
			}
			if hint.Reason != nil && hint.Reason.Text != nil && *hint.Reason.Text != "" && s.hintReasons[i][j] == "" {
				trial, ok, err := s.fitString(*hint.Reason.Text, budget, render, func(next *selection, value string) { next.hintReasons[i][j] = value })
				if err != nil {
					return selection{}, false, err
				}
				if ok {
					if err := consider(trial, item.weight+hintWeight(hint), i, "reason:"+stableKey(hint)); err != nil {
						return selection{}, false, err
					}
				}
			}
			if hint.EvidenceReference != nil && hint.EvidenceReference.Context != nil && *hint.EvidenceReference.Context != "" && s.hintContexts[i][j] == "" {
				trial, ok, err := s.fitString(*hint.EvidenceReference.Context, budget, render, func(next *selection, value string) { next.hintContexts[i][j] = value })
				if err != nil {
					return selection{}, false, err
				}
				if ok {
					if err := consider(trial, item.weight+hintWeight(hint), i, "context:"+stableKey(hint)); err != nil {
						return selection{}, false, err
					}
				}
			}
		}
	}
	if !found {
		return s, false, nil
	}
	return best.state, true, nil
}

// fitSummary chooses the longest UTF-8-safe prefix whose actual serialization
// fits. An empty prefix is not selected; omitted counts then explain absence.
func (s selection) fitSummary(i, j int, full string, budget int, render Renderer) (selection, bool, error) {
	return s.fitString(full, budget, render, func(next *selection, value string) {
		if j < 0 {
			next.summaries[i] = value
		} else {
			next.hintSummaries[i][j] = value
		}
	})
}

func (s selection) fitString(full string, budget int, render Renderer, apply func(*selection, string)) (selection, bool, error) {
	runes := []rune(full)
	low, high := 1, len(runes)
	var fitting selection
	found := false
	for low <= high {
		mid := low + (high-low)/2
		trial := s.copy()
		apply(&trial, string(runes[:mid]))
		_, data, err := trial.measure(render)
		if err != nil {
			return selection{}, false, err
		}
		if len(data) <= budget {
			fitting, found = trial, true
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return fitting, found, nil
}
