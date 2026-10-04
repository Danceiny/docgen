package pipeline

import "testing"

func TestPatternsAreLiteralExceptForStar(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"shop/config", "shop/config", true},
		{"shop/config", "shop/configs", false},
		{"shop/config", "xshop/config", false},
		{"*/domain", "trade/domain", true},
		{"*/domain", "trade/orders/domain", true}, // "*" runs over slashes
		{"*/domain", "domain", false},
		{"v1.2/api", "v1.2/api", true},
		{"v1.2/api", "v1x2/api", false}, // a dot is a dot, not any character
		{"a+b/c", "a+b/c", true},
		{"a+b/c", "aab/c", false},
		{"(a|b)/x", "(a|b)/x", true},
		{"(a|b)/x", "a/x", false},
		{"[x]/y", "[x]/y", true},
		{"[x]/y", "x/y", false},
		{"*", "anything/at/all", true},
	} {
		got := matchesAnyPattern(tc.path, compilePatterns([]string{tc.pattern}))
		if got != tc.want {
			t.Errorf("pattern %q on %q = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestAnyOfSeveralPatternsMatches(t *testing.T) {
	patterns := compilePatterns([]string{"a/b", "c/*"})
	if !matchesAnyPattern("a/b", patterns) || !matchesAnyPattern("c/d/e", patterns) || matchesAnyPattern("x/y", patterns) {
		t.Fatal("matchesAnyPattern must accept a path that matches any pattern and no other")
	}
	if matchesAnyPattern("a/b", nil) {
		t.Fatal("no patterns match nothing")
	}
}

func TestNoPatternCanFailToCompile(t *testing.T) {
	// Every character but "*" is quoted, so no input is a regular expression.
	for _, p := range []string{"(", "[", "\\", "a**b", "?", "{1", "*(*", ""} {
		compilePatterns([]string{p}) // must not panic
	}
}
