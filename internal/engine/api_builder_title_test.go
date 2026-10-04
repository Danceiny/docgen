package engine

import (
	"strings"
	"testing"
)

// titleWords must give what strings.Title gave, for the names it can be given.
func TestTitleWordsIsWhatStringsTitleWas(t *testing.T) {
	for _, s := range []string{
		"", "pet", "Pet", "userAdmin", "user_admin", "user-admin", "user admin", "a1b", "1abc", "x.y", "über", "été", "привет",
	} {
		if got, want := titleWords(s), strings.Title(s); got != want { //nolint:staticcheck // the point of the test
			t.Errorf("titleWords(%q) = %q, strings.Title gives %q", s, got, want)
		}
	}
}
