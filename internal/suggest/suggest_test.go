package suggest

import "testing"

func TestDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"a", "", 1}, {"modles", "models", 2}, {"abc", "abc", 0}, {"kitten", "sitting", 3}} {
		if got := Distance(tc.a, tc.b); got != tc.want {
			t.Errorf("Distance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestClosestWithin(t *testing.T) {
	words := []string{"path", "tags"}
	if got := ClosestWithin("auth", words, 1); got != "" {
		t.Errorf("auth is two edits from path, and was taken for it: %q", got)
	}
	if got := ClosestWithin("auth", words, 2); got != "path" {
		t.Errorf("with two edits auth is path: %q", got)
	}
	if got := ClosestWithin("Tags", words, 1); got != "tags" {
		t.Errorf("a capital is one edit: %q", got)
	}
}

func TestClosest(t *testing.T) {
	words := []string{"response", "desc", "tags"}
	for _, tc := range []struct{ word, want string }{
		{"respone", "response"}, // a letter missing
		{"discription", ""},     // too far from everything
		{"tag", "tags"},
		{"response", ""}, // it is one of them
		{"zzzzzz", ""},
	} {
		if got := Closest(tc.word, words); got != tc.want {
			t.Errorf("Closest(%q) = %q, want %q", tc.word, got, tc.want)
		}
	}
}
