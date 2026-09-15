package matcher

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Normalize converts a string into a comparable form: it is Unicode-normalized
// to NFC (so macOS NFD exports still match NFC library metadata), lowercased,
// and every rune that is not a letter or a digit is collapsed into a single
// space. Letters from non-latin scripts are preserved so Japanese, Cyrillic and
// other titles still match.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	for _, r := range strings.ToLower(strings.TrimSpace(norm.NFC.String(s))) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSpace && b.Len() > 0 {
				b.WriteRune(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
			continue
		}
		pendingSpace = true
	}
	return b.String()
}

// Similarity returns a 0..1 ratio derived from the Levenshtein distance of two
// already-normalized strings.
func Similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	if a == "" || b == "" {
		return 0
	}
	aRunes, bRunes := []rune(a), []rune(b)
	longest := len(aRunes)
	if len(bRunes) > longest {
		longest = len(bRunes)
	}
	return 1 - float64(levenshtein(aRunes, bRunes))/float64(longest)
}

// levenshtein computes the edit distance between two rune slices using two rows.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min3(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
