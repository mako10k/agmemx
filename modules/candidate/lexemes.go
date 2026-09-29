package candidate

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

// Lexemes applies the same width/case normalization to queries and indexed
// record text. Callers index both body text and display name with this function.
// Repeated tokens are retained so the index can store their frequencies.
func Lexemes(text string) ([]string, error) {
	if !utf8.ValidString(text) {
		return nil, ErrInvalidInput
	}
	normalized := norm.NFC.String(strings.ToLower(width.Fold.String(text)))
	var result []string
	var run []rune
	runCJK := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		if runCJK && len(run) > 1 {
			for i := 0; i+1 < len(run); i++ {
				result = append(result, string(run[i:i+2]))
			}
		} else {
			result = append(result, string(run))
		}
		run = run[:0]
	}
	for _, r := range normalized {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			flush()
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r) {
			flush()
			continue
		}
		isCJK := unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
		if len(run) > 0 && runCJK != isCJK {
			flush()
		}
		runCJK = isCJK
		run = append(run, r)
	}
	flush()
	return result, nil
}
