package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/errcat"
	"github.com/Danceiny/docgen/internal/overlay"
)

// block returns the first fenced code block of the given language that comes
// after the heading, as the README writes them.
func block(t *testing.T, readme, heading, lang string) string {
	t.Helper()
	_, after, ok := strings.Cut(readme, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("the README has no heading %q", heading)
	}
	_, code, ok := strings.Cut(after, "```"+lang+"\n")
	if !ok {
		t.Fatalf("no %s block after %q", lang, heading)
	}
	code, _, ok = strings.Cut(code, "```")
	if !ok {
		t.Fatalf("the %s block after %q is not closed", lang, heading)
	}
	return code
}

// The README shows a configuration, an error catalog and an overlay. They are
// the documentation of those formats, so they have to be valid in them.
func TestReadmeExamplesAreValid(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)

	for _, heading := range []string{"## Quick start", "## Configuration"} {
		if _, err := config.Parse([]byte(block(t, readme, heading, "yaml"))); err != nil {
			t.Errorf("the configuration under %q: %v", heading, err)
		}
	}
	if _, err := errcat.Parse([]byte(block(t, readme, "### The error catalog", "json"))); err != nil {
		t.Errorf("the error catalog: %v", err)
	}
	if _, err := overlay.Parse([]byte(block(t, readme, "### Overlays", "yaml"))); err != nil {
		t.Errorf("the overlay: %v", err)
	}
}
