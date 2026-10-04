// Package yamlerr turns the errors of a strict YAML decoder into messages that
// say what to do. The decoder says "field modles not found in type config.Doc":
// a name of a Go type that the person who wrote the file has never seen. Here it
// says which key is unknown, where, and which keys are known.
package yamlerr

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/Danceiny/docgen/internal/suggest"
	"gopkg.in/yaml.v3"
)

var unknownField = regexp.MustCompile(`^line (\d+): field (\S+) not found in type (\S+)$`)

// Describe says where a Go type is in a file and which keys it has.
type Describe func(goType string) (where string, keys []string, ok bool)

// Explain rewrites the unknown-key errors of err, which a decoder with
// KnownFields reports, and leaves every other error as it is.
func Explain(err error, describe Describe) error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return err
	}
	messages := make([]string, len(typeErr.Errors))
	for i, msg := range typeErr.Errors {
		messages[i] = msg
		m := unknownField.FindStringSubmatch(msg)
		if m == nil {
			continue
		}
		line, key, goType := m[1], m[2], m[3]
		where, keys, ok := describe(goType)
		if !ok {
			continue
		}
		explained := fmt.Sprintf("line %s: unknown key %q in %s", line, key, where)
		if guess := suggest.Closest(key, keys); guess != "" {
			explained += fmt.Sprintf(" (did you mean %q?)", guess)
		}
		explained += fmt.Sprintf(". The keys here are: %s", strings.Join(keys, ", "))
		messages[i] = explained
	}
	return errors.New(strings.Join(messages, "\n"))
}

// Keys returns the keys of a struct that a YAML file can have, in the order the
// fields are declared in.
func Keys(t reflect.Type) []string {
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" || t.Field(i).PkgPath != "" {
			continue
		}
		if name == "" {
			name = strings.ToLower(t.Field(i).Name)
		}
		keys = append(keys, name)
	}
	return keys
}
