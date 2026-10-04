package main

import (
	"os"
	"reflect"
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

// yamlKeysOf lists the keys of a configuration type and of the types in it.
func yamlKeysOf(t reflect.Type, seen map[reflect.Type]bool, keys map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
		yamlKeysOf(f.Type, seen, keys)
	}
}

// The reference of the configuration is the documentation of the configuration:
// a key that it does not show is a key nobody learns of.
func TestReadmeConfigurationShowsEveryKey(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	reference := block(t, string(data), "## Configuration", "yaml")

	keys := map[string]bool{}
	yamlKeysOf(reflect.TypeOf(config.Config{}), map[reflect.Type]bool{}, keys)
	if len(keys) < 40 {
		t.Fatalf("found only %d keys in the configuration types", len(keys))
	}
	for key := range keys {
		if !strings.Contains(reference, key+":") && !strings.Contains(reference, "{"+key+":") && !strings.Contains(reference, ", "+key+":") {
			t.Errorf("the configuration reference of the README does not show the key %q", key)
		}
	}
}
