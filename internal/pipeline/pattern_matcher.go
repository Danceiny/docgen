package pipeline

import (
	"regexp"
	"strings"

	"github.com/Danceiny/docgen/internal/engine"
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

// matchesAnyPattern reports whether the path matches one of the patterns.
func matchesAnyPattern(path string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
