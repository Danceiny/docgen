package pipeline

import (
	"log/slog"
	"testing"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
)

// A run reports to the logger it was given, and to slog's default when it was
// given none. A configuration with no documents makes Run set up the engine and
// return, which is all this test needs of it.
func TestRunHandsItsLoggerToTheEngine(t *testing.T) {
	oldRoot := rootDir
	t.Cleanup(func() {
		rootDir = oldRoot
		engine.SetLogger(nil)
	})

	l := slog.New(slog.DiscardHandler)
	if err := Run(Options{Dir: t.TempDir(), Config: &config.Config{}, Logger: l}); err != nil {
		t.Fatal(err)
	}
	if engine.Logger() != l {
		t.Fatal("Run did not install Options.Logger as the engine's logger")
	}

	if err := Run(Options{Dir: t.TempDir(), Config: &config.Config{}}); err != nil {
		t.Fatal(err)
	}
	if engine.Logger() != slog.Default() {
		t.Fatal("with no Options.Logger the engine must report to slog.Default(), not keep the previous run's logger")
	}
}
