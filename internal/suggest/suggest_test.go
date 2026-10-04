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

func TestNearestFindsTheKeyThatWasMeant(t *testing.T) {
	keys := []string{
		"example.com.shop.order.domain.Order",
		"example.com.shop.order.domain.OrderLine",
		"example.com.shop.pet.domain.Pet",
		"example.com.shop.store.Pet",
	}
	for _, tc := range []struct{ in, want, why string }{
		{"example.com.shop.order.domain.Ordr", "example.com.shop.order.domain.Order", "a letter missing"},
		{"example.com.shop.order.domain.order", "example.com.shop.order.domain.Order", "a capital"},
		{"example.com.shop.order.domian.OrderLine", "example.com.shop.order.domain.OrderLine", "two letters swapped"},
		{"example.com.shop.order.OrderLine", "example.com.shop.order.domain.OrderLine", "a package left out"},
		{"example.com.shop.pet.domain.Pet", "", "it is one of them"},
		{"example.com.shop.zebra.Pet", "", "two types are called Pet: no guess is better than a wrong one"},
		{"example.com.shop.order.domain.Invoice", "", "nothing is like it"},
		{"other.module.Thing", "", "nothing is like it"},
	} {
		if got := Nearest(tc.in, keys); got != tc.want {
			t.Errorf("Nearest(%q) = %q, want %q (%s)", tc.in, got, tc.want, tc.why)
		}
	}
}
