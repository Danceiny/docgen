package engine

import (
	"reflect"
	"testing"
)

func TestFilterKeepsOrderAndNeverReturnsNil(t *testing.T) {
	even := func(n int) bool { return n%2 == 0 }
	if got := filter([]int{1, 2, 3, 4, 6}, even); !reflect.DeepEqual(got, []int{2, 4, 6}) {
		t.Errorf("filter = %v", got)
	}
	for name, in := range map[string][]int{"no match": {1, 3}, "empty": {}, "nil": nil} {
		got := filter(in, even)
		if got == nil || len(got) != 0 {
			t.Errorf("%s: filter = %#v, want a non-nil empty slice", name, got)
		}
	}
}

func TestFilterReturnsANewSlice(t *testing.T) {
	in := []string{"a", "", "b"}
	out := filter(in, func(s string) bool { return s != "" })
	out[0] = "changed"
	if in[0] != "a" {
		t.Fatal("filter must not alias its input")
	}
}

func TestStringPtr(t *testing.T) {
	if stringPtr("") != nil {
		t.Error("the empty string is nil")
	}
	s := "text"
	p := stringPtr(s)
	if p == nil || *p != "text" {
		t.Fatalf("stringPtr = %v", p)
	}
	*p = "changed"
	if s != "text" {
		t.Error("stringPtr must point at a copy")
	}
}

func TestDedupeKeepsTheFirstOfEachInOrder(t *testing.T) {
	got := dedupe([]string{"b", "a", "b", "c", "a"})
	if !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("dedupe = %v, want the first of each in order", got)
	}
}

func TestDedupeReturnsAnEmptyInputAsItIs(t *testing.T) {
	if got := dedupe([]string(nil)); got != nil {
		t.Fatalf("dedupe(nil) = %#v, want nil", got)
	}
	empty := []string{}
	if got := dedupe(empty); got == nil || len(got) != 0 {
		t.Fatalf("dedupe(empty) = %#v", got)
	}
}

func TestDedupeDoesNotChangeItsInput(t *testing.T) {
	in := []string{"a", "a", "b"}
	_ = dedupe(in)
	if !reflect.DeepEqual(in, []string{"a", "a", "b"}) {
		t.Fatalf("input changed: %v", in)
	}
}
