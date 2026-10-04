package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// A run is quiet about detail by default and shows it with -v; warnings and
// errors are always shown.
func TestNewLoggerShowsDebugOnlyWhenVerbose(t *testing.T) {
	for _, tc := range []struct {
		verbose   bool
		wantDebug bool
	}{
		{verbose: false, wantDebug: false},
		{verbose: true, wantDebug: true},
	} {
		var out bytes.Buffer
		l := newLogger(&out, tc.verbose)
		l.Debug("detail of the run")
		l.Warn("a warning")
		l.Error("an error")

		got := out.String()
		if has := strings.Contains(got, "detail of the run"); has != tc.wantDebug {
			t.Errorf("verbose=%v: debug line present = %v, want %v\n%s", tc.verbose, has, tc.wantDebug, got)
		}
		for _, line := range []string{"a warning", "an error"} {
			if !strings.Contains(got, line) {
				t.Errorf("verbose=%v: %q is missing\n%s", tc.verbose, line, got)
			}
		}
	}
}

func TestVersionStringNamesTheToolAndTheToolchain(t *testing.T) {
	got := versionString()
	if !strings.HasPrefix(got, "docgen ") || !strings.Contains(got, runtime.Version()) {
		t.Fatalf("version = %q", got)
	}
}
