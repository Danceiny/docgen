package pipeline

import (
	"reflect"
	"testing"
)

func TestFilterKeepsOrderAndNeverReturnsNil(t *testing.T) {
	notEmpty := func(s string) bool { return s != "" }
	if got := filter([]string{"a", "", "b"}, notEmpty); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("filter = %v", got)
	}
	if got := filter(nil, notEmpty); got == nil || len(got) != 0 {
		t.Errorf("filter(nil) = %#v, want a non-nil empty slice", got)
	}
}

func TestDedupeKeepsTheFirstOfEach(t *testing.T) {
	got := dedupe([]string{"b", "a", "b", "c", "a"})
	if !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Errorf("dedupe = %v", got)
	}
}

func TestDedupeReturnsAnEmptyInputAsItIs(t *testing.T) {
	if got := dedupe([]string(nil)); got != nil {
		t.Errorf("dedupe(nil) = %#v, want nil", got)
	}
	empty := []string{}
	if got := dedupe(empty); got == nil || len(got) != 0 {
		t.Errorf("dedupe(empty) = %#v, want the empty slice", got)
	}
}

func TestDedupeDoesNotChangeItsInput(t *testing.T) {
	in := []string{"a", "a", "b"}
	_ = dedupe(in)
	if !reflect.DeepEqual(in, []string{"a", "a", "b"}) {
		t.Fatalf("input changed: %v", in)
	}
}
