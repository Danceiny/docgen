package yamlerr

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type inner struct {
	Name string `yaml:"name"`
}

// outer has a field that cannot be a key because it is not exported and one that
// is left out by its tag; Keys must offer neither.
type outer struct {
	Models  []string `yaml:"models"`
	Version int      `yaml:"version"`
	Inner   inner    `yaml:"inner"`
	hidden  int
	Skipped int `yaml:"-"`
}

var _ = outer{hidden: 0}

func decode(t *testing.T, in string) error {
	t.Helper()
	var out outer
	dec := yaml.NewDecoder(bytes.NewReader([]byte(in)))
	dec.KnownFields(true)
	return dec.Decode(&out)
}

func describe(goType string) (string, []string, bool) {
	switch goType {
	case "yamlerr.outer":
		return "the top level", Keys(reflect.TypeOf(outer{})), true
	case "yamlerr.inner":
		return "inner", Keys(reflect.TypeOf(inner{})), true
	}
	return "", nil, false
}

func TestExplainSaysWhichKeyIsUnknownWhereAndWhatThereIs(t *testing.T) {
	err := Explain(decode(t, "modles: [a]\nversion: 1\ninner:\n  nmae: x\nextra: 1\n"), describe)
	if err == nil {
		t.Fatal("no error")
	}
	msg := err.Error()
	for _, want := range []string{
		`line 1: unknown key "modles" in the top level (did you mean "models"?).`,
		`line 4: unknown key "nmae" in inner (did you mean "name"?).`,
		`line 5: unknown key "extra" in the top level. The keys here are: models, version, inner`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the error does not say %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "not found in type") {
		t.Errorf("the name of a Go type is in the error:\n%s", msg)
	}
	if strings.Contains(msg, "hidden") || strings.Contains(msg, "Skipped") {
		t.Errorf("a field that is not a key is listed:\n%s", msg)
	}
}

func TestExplainLeavesOtherErrorsAndUnknownTypesAlone(t *testing.T) {
	err := decode(t, "version: not-a-number\n")
	if got := Explain(err, describe); got == nil || !strings.Contains(got.Error(), "cannot unmarshal") {
		t.Errorf("a type error became %v", got)
	}
	if got := Explain(nil, describe); got != nil {
		t.Errorf("no error became %v", got)
	}
	unknown := decode(t, "nope: 1\n")
	if got := Explain(unknown, func(string) (string, []string, bool) { return "", nil, false }); got == nil || !strings.Contains(got.Error(), "not found in type") {
		t.Errorf("an error about a type that is not described became %v", got)
	}
}
