package engine

import (
	"go/ast"
	"reflect"
	"strings"
)

func ExtractTag(comments, tagName string) string {
	v := ExtractTagFromComments(strings.Split(comments, "\n"), tagName)
	v = strings.TrimSpace(v)
	v = strings.Trim(v, `"`)
	return v
}

// ExtractTags returns the comma-separated values of an annotation, such as
// "@tags: a, b", without the spaces around them. An annotation that is absent
// gives one empty value.
func ExtractTags(comments, tagName string) []string {
	vs := strings.Split(ExtractTag(comments, tagName), ",")
	for i := range vs {
		vs[i] = strings.TrimSpace(vs[i])
	}
	return vs
}

func ExtractTagFromComments(comments []string, tagName string) string {
	for i := 0; i < len(comments); i++ {
		line := comments[i]
		if strings.HasPrefix(line, "@"+tagName+":") {
			// Capture first line content
			content := strings.TrimSpace(strings.TrimPrefix(line, "@"+tagName+":"))
			// Permission/tag values are comma-separated single-line metadata. Do
			// not absorb following comment lines (which may be YAML-looking text).
			if tagName == "permission" || tagName == "tags" || tagName == "path" || tagName == "method" {
				return content
			}
			// Capture continuation lines until next tag
			j := i + 1
			for j < len(comments) {
				next := strings.TrimSpace(comments[j])
				if strings.HasPrefix(next, "@") {
					break
				}
				if content != "" {
					content += "\n"
				}
				// Preserve empty lines and original content
				if next == "" {
					content += ""
				} else {
					content += next
				}
				j++
			}
			return content
		}
	}
	return ""
}

// extractDescription extracts the description text from comments.
func extractDescription(docs, comments *ast.CommentGroup) (out string) {
	ignores := []string{"@generic:"}
	d1, d2 := extractCommentGroupText(docs, ignores...), extractCommentGroupText(comments, ignores...)
	return strings.Join(filter([]string{strings.TrimSpace(d1), strings.TrimSpace(d2)}, func(s string) bool {
		return s != ""
	}), "\n")
}

// extractDescriptionRaw is extractDescription without the ignored annotations.
func extractDescriptionRaw(docs, comments *ast.CommentGroup) (out string) {
	d1, d2 := extractCommentGroupText(docs), extractCommentGroupText(comments)
	return strings.Join(filter([]string{strings.TrimSpace(d1), strings.TrimSpace(d2)}, func(s string) bool {
		return s != ""
	}), "\n")
}

func extractTagValueFromDocComments(docs, comments *ast.CommentGroup, tagName string) (out []string) {
	desc := extractDescriptionRaw(docs, comments)
	return filter(strings.Split(ExtractTag(desc, tagName), ","), func(s string) bool {
		return s != ""
	})
}

// extractComments returns the text of a comment group, cleaned up.
func extractComments(commentGroup *ast.CommentGroup) string {
	return extractDescription(nil, commentGroup)
}

func getValueFromTag(field *ast.Field, k string) string {
	if field.Tag == nil {
		return ""
	}
	tagStr := strings.Trim(field.Tag.Value, "`") // remove the backticks
	tag := reflect.StructTag(tagStr)
	return tag.Get(k) // read the desc tag directly
}

// parseParamComment parses the @param annotation of a parameter, which is
// "@param:<in> <name> <type> <required> [\"<description>\"]". The parameter is
// found by its name or, as an int, by its position.
func (m *Method) parseParamComment(doc string, identifier interface{}) *ParamSpec {
	// identifier can be a position index or a field name
	var target string
	switch v := identifier.(type) {
	case int:
		target = generateParamName(v)
	case string:
		target = v
	default:
		return nil
	}

	lines := strings.Split(doc, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "@param:") {
			continue
		}

		// @param:<in> <name> <type> <required> ["<description>"]
		parts := splitParamAnnotation(line)
		if parts == nil || parts[1] != target {
			continue
		}

		description := ""
		if len(parts) == 5 {
			description = strings.Trim(parts[4], `"`)
		}
		types := m.parseStringType(parts[2])
		if len(types) == 0 {
			continue
		}
		return &ParamSpec{
			In:    parts[0],
			Name:  parts[1],
			Types: types,
			// A parameter of the path is part of the route: it is always there.
			Required:    parts[3] == "required" || parts[0] == "path",
			Description: description,
		}
	}
	return nil
}

// splitParamAnnotation splits a line "@param:<in> <name> <type> <required>
// [\"description\"]" into its parts; a line that is not such an annotation (it
// is prose, or it is short of parts) gives nil.
func splitParamAnnotation(line string) []string {
	rest, ok := strings.CutPrefix(line, "@param:")
	if !ok {
		return nil
	}
	parts := strings.SplitN(strings.TrimSpace(rest), " ", 5)
	if len(parts) < 4 {
		return nil
	}
	return parts
}

// paramAnnotationNames lists the names the @param annotations of a doc comment
// give to parameters.
func paramAnnotationNames(doc string) []string {
	var names []string
	for _, line := range strings.Split(doc, "\n") {
		if parts := splitParamAnnotation(line); parts != nil {
			names = append(names, parts[1])
		}
	}
	return names
}

func trimPrefix(c string, ignores ...string) string {
	for _, ignore := range ignores {
		if v, ok := strings.CutPrefix(c, ignore); ok {
			// what comes before the semicolon is dropped as well
			vs := strings.SplitN(v, ";", 2)
			if len(vs) == 2 {
				return vs[1]
			}
			return v
		}
	}
	return c
}

// extractCommentGroupText returns the text of a comment group, like the Text
// method of go/ast.CommentGroup (from which it is adapted, see
// THIRD_PARTY_NOTICES.md) but leaving out the lines that start with one of the
// ignored prefixes.
// Comment markers (//, /*, and */), the first space of a line comment, and
// leading and trailing empty lines are removed.
// Comment directives like "//line" and "//go:noinline" are also removed.
// Multiple empty lines are reduced to one, and trailing space on lines is trimmed.
// Unless the result is empty, it is newline-terminated.
func extractCommentGroupText(g *ast.CommentGroup, ignores ...string) string {
	if g == nil {
		return ""
	}
	comments := make([]string, len(g.List))
	for i, c := range g.List {
		comments[i] = c.Text
	}

	lines := make([]string, 0, 10) // most comments are less than 10 lines
	for _, c := range comments {
		// Remove comment markers.
		// The parser has given us exactly the comment text.
		switch c[1] {
		case '/':
			//-style comment (no newline at the end)
			c = c[2:]
			if len(c) == 0 {
				// empty line
				break
			}
			if c[0] == ' ' {
				// strip first space - required for Example tests
				c = c[1:]
				c = trimPrefix(c, ignores...) //added
				break
			}
			if isDirective(c) {
				// Ignore //go:noinline, //line, and so on.
				continue
			}
			c = trimPrefix(c, ignores...) //added
		case '*':
			/*-style comment */
			c = c[2 : len(c)-2]
		}

		// Split on newlines.
		cl := strings.Split(c, "\n")

		// Walk lines, stripping trailing white space and adding to list.
		for _, l := range cl {
			lines = append(lines, stripTrailingWhitespace(l))
		}
	}

	// Remove leading blank lines; convert runs of
	// interior blank lines to a single blank line.
	n := 0
	for _, line := range lines {
		if line != "" || n > 0 && lines[n-1] != "" {
			lines[n] = line
			n++
		}
	}
	lines = lines[0:n]

	// Add final "" entry to get trailing newline from Join.
	if n > 0 && lines[n-1] != "" {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func isWhitespace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' }

func stripTrailingWhitespace(s string) string {
	i := len(s)
	for i > 0 && isWhitespace(s[i-1]) {
		i--
	}
	return s[0:i]
}

// isDirective reports whether c is a comment directive.
// This code is also in go/printer.
func isDirective(c string) bool {
	// "//line " is a line directive.
	// "//extern " is for gccgo.
	// "//export " is for cgo.
	// (The // has been removed.)
	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "extern ") || strings.HasPrefix(c, "export ") {
		return true
	}

	// "//[a-z0-9]+:[a-z0-9]"
	// (The // has been removed.)
	colon := strings.Index(c, ":")
	if colon <= 0 || colon+1 >= len(c) {
		return false
	}
	for i := 0; i <= colon+1; i++ {
		if i == colon {
			continue
		}
		b := c[i]
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') {
			return false
		}
	}
	return true
}
