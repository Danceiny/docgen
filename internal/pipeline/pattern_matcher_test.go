package pipeline

import (
	"log/slog"
	"reflect"
	"testing"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
	"golang.org/x/tools/go/packages"
)

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

// A pattern that matches no package is the mistake behind most empty documents:
// the sample "*/service" does not match a package service at the module root.
func TestUnmatchedPatternsAreFound(t *testing.T) {
	engine.ModuleName = "example.com/shop"
	t.Cleanup(func() { engine.ModuleName = "" })

	pkgs := []*packages.Package{
		{ID: "example.com/shop/service"}, // at the module root
		{ID: "example.com/shop/orders/domain"},
		{ID: "example.com/shop"}, // the module root itself
	}
	got := unmatchedPatterns(pkgs, []string{"*/service", "service", "*/domain", "nope", "*"})
	if want := []string{"*/service", "nope"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched = %v, want %v", got, want)
	}
}

// A list that names both layouts, */service for a module with directories above
// and service for one without, has one pattern that matches nothing by design.
func TestTheTwinOfAPatternIsNotReportedWhenBothAreListed(t *testing.T) {
	engine.ModuleName = "example.com/shop"
	log := &records{}
	engine.SetLogger(slog.New(log))
	t.Cleanup(func() { engine.ModuleName = ""; engine.SetLogger(nil) })

	pkgs := []*packages.Package{{ID: "example.com/shop/service"}, {ID: "example.com/shop/model"}}
	warnAboutPatternsThatMatchNothing(config.Doc{
		Name:     "internal",
		Models:   []string{"*/model", "model"},
		Services: []string{"*/service", "orders", "*/cron"},
	}, pkgs)

	var got []string
	for _, rec := range log.list {
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == "pattern" {
				got = append(got, a.Value.String())
			}
			return true
		})
	}
	if want := []string{"*/service", "orders", "*/cron"}; !reflect.DeepEqual(got, want) {
		t.Errorf("reported patterns = %v, want %v: */model has model beside it, and nothing else is named in both layouts", got, want)
	}
}
