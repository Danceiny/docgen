package pipeline

import (
	"regexp"
	"slices"
	"strings"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"

	"golang.org/x/tools/go/packages"
)

// getRelativePath returns a package path relative to the module path.
func getRelativePath(fullPath string) string {
	return strings.TrimPrefix(fullPath, engine.ModuleName+"/")
}

// compilePatterns compiles package patterns. In a pattern only "*" is special:
// it matches any run of characters, slashes included, and everything else,
// dots included, matches itself.
func compilePatterns(patterns []string) []*regexp.Regexp {
	res := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		quoted := regexp.QuoteMeta(p)
		res = append(res, regexp.MustCompile("^"+strings.ReplaceAll(quoted, `\*`, ".*")+"$"))
	}
	return res
}

// unmatchedPatterns returns the patterns that match no package of the module.
func unmatchedPatterns(pkgs []*packages.Package, patterns []string) []string {
	var unmatched []string
	for _, pattern := range patterns {
		compiled := compilePatterns([]string{pattern})
		matched := false
		for _, pkg := range pkgs {
			if matchesAnyPattern(getRelativePath(pkg.ID), compiled) {
				matched = true
				break
			}
		}
		if !matched {
			unmatched = append(unmatched, pattern)
		}
	}
	return unmatched
}

// warnAboutPatternsThatMatchNothing says so for each pattern of a document that
// no package of the module matches: it is almost always a mistake, and the
// document is silently empty because of it.
func warnAboutPatternsThatMatchNothing(d config.Doc, pkgs []*packages.Package) {
	for _, field := range []struct {
		name     string
		patterns []string
	}{{"models", d.Models}, {"services", d.Services}} {
		for _, pattern := range unmatchedPatterns(pkgs, field.patterns) {
			msg := "a pattern matches no package of the module; patterns are matched against the package path relative to the module"
			if rest, below := strings.CutPrefix(pattern, "*/"); below {
				if slices.Contains(field.patterns, rest) {
					continue // the list names both layouts, one of them is not the module's
				}
				msg += ", so " + pattern + " does not match the package " + rest + " at the module root; list " + rest + " too"
			} else if slices.Contains(field.patterns, "*/"+pattern) {
				continue // likewise: the module has directories above it, and the root package is the other layout
			}
			engine.Logger().Warn(msg, "document", d.Name, "list", field.name, "pattern", pattern)
		}
	}
}

// matchesAnyPattern reports whether the path matches one of the patterns.
func matchesAnyPattern(path string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
