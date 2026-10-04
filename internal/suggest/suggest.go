// Package suggest finds the word that a mistyped one was probably meant to be.
package suggest

import "strings"

// Closest returns the word of words that differs from word by at most two
// single-character edits, the nearest of them, or "" when there is none or when
// word is one of them.
func Closest(word string, words []string) string {
	return ClosestWithin(word, words, 2)
}

// ClosestWithin is Closest with the number of edits that may separate the words:
// with one, a letter missing or too many or wrong, nothing short of that.
func ClosestWithin(word string, words []string, edits int) string {
	best, bestDistance := "", edits+1
	for _, w := range words {
		if w == word {
			return ""
		}
		if d := Distance(word, w); d < bestDistance {
			best, bestDistance = w, d
		}
	}
	return best
}

// Distance is the number of single-character edits (an insertion, a deletion or
// a substitution) between two strings.
func Distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// Nearest returns the word of words that a mistyped long one, such as the full
// key of a type, was probably meant to be, or "" when none is near enough: one
// that differs only in case, one a few edits away (an eighth of the length, three
// at most), or the only one that ends in the same last dotted segment.
func Nearest(word string, words []string) string {
	for _, w := range words {
		if w == word {
			return ""
		}
	}
	for _, w := range words {
		if strings.EqualFold(w, word) {
			return w
		}
	}
	best, bestDistance := "", max(1, min(len(word)/8, 3))+1
	for _, w := range words {
		if d := Distance(word, w); d < bestDistance {
			best, bestDistance = w, d
		}
	}
	if best != "" {
		return best
	}
	last := word[strings.LastIndex(word, ".")+1:]
	for _, w := range words {
		if w[strings.LastIndex(w, ".")+1:] == last {
			if best != "" {
				return "" // two types of that name: no guess is better than a wrong one
			}
			best = w
		}
	}
	return best
}
