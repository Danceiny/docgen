// Package errcat reads the catalog of named errors that a module's @response
// annotations refer to.
//
// The catalog is a JSON file: an array of objects with the fields name, code,
// message and httpCode, in the order the module registers them,
//
//	[
//	  {"name": "NotFound", "code": 40401, "message": "not found", "httpCode": 404}
//	]
//
// docgen documents each entry as a component, answers an annotation such as
// "@response:404,NotFound,no such order" from it, and corrects the status code
// of an annotation to the HTTP status of the error it names. The file is data so
// that docgen needs no knowledge of the code that defines the errors; a module
// exports its registry with a small program of its own.
package errcat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// Entry is one named error.
type Entry struct {
	// Name identifies the error in annotations and in its component key. It is an
	// identifier: letters, digits and underscores, not starting with a digit.
	Name string `json:"name"`
	// Code is the business status code the error carries in a response body.
	Code int32 `json:"code"`
	// Message is the message the error carries in a response body.
	Message string `json:"message"`
	// HTTPCode is the HTTP status the error is answered with, from 100 to 599.
	HTTPCode int32 `json:"httpCode"`
}

// Catalog is a validated list of errors, looked up by name.
type Catalog struct {
	entries []Entry
	byName  map[string]int
}

// Load reads and validates the catalog at path.
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read error catalog: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes and validates a catalog. The decoder is strict, like the
// configuration's: an unknown field is an error. Every problem with the entries
// is reported at once.
func Parse(data []byte) (*Catalog, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		return nil, errors.New("decode error catalog: want a JSON array of errors")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var entries []Entry
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode error catalog: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("decode error catalog: unexpected data after the array")
	}

	c := &Catalog{entries: entries, byName: make(map[string]int, len(entries))}
	var problems []error
	for i, e := range entries {
		at := fmt.Sprintf("errors[%d]", i)
		switch {
		case e.Name == "":
			problems = append(problems, fmt.Errorf("%s.name: required", at))
		case !isIdentifier(e.Name):
			problems = append(problems, fmt.Errorf("%s.name: %q is not an identifier", at, e.Name))
		default:
			if first, dup := c.byName[e.Name]; dup {
				problems = append(problems, fmt.Errorf("%s.name: %q is already errors[%d]", at, e.Name, first))
			}
			c.byName[e.Name] = i
		}
		if e.HTTPCode < 100 || e.HTTPCode > 599 {
			problems = append(problems, fmt.Errorf("%s.httpCode: want 100 to 599, got %d", at, e.HTTPCode))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return nil, err
	}
	return c, nil
}

// Entries returns the errors in the order of the file. The slice is the
// catalog's own and must not be changed. A nil catalog has none.
func (c *Catalog) Entries() []Entry {
	if c == nil {
		return nil
	}
	return c.entries
}

// Find returns the error a key names: the part after its last dot, so that
// "NotFound", "errors.NotFound" and a component key such as "shop.errors.NotFound"
// all name the same error. A nil catalog finds nothing.
func (c *Catalog) Find(key string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	name := key[strings.LastIndexByte(key, '.')+1:]
	i, ok := c.byName[name]
	if !ok {
		return Entry{}, false
	}
	return c.entries[i], true
}

func isIdentifier(s string) bool {
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}
