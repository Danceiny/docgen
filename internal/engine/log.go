package engine

import (
	"fmt"
	"log/slog"
	"sync/atomic"
)

// diagnostics is the logger the engine reports to. Like Settings it is
// process-wide, because docgen generates one configuration at a time; it is
// atomic so that tests which swap it are not a data race.
var diagnostics atomic.Pointer[slog.Logger]

// SetLogger sets the logger that receives the engine's diagnostics: the types
// it could not resolve, the annotations it could not use, the packages it could
// not load. With nil the engine reports to slog.Default().
//
// The engine never prints on its own; everything it has to say goes through
// this logger, at Warn when the document is probably missing something, at
// Error when the engine met a construct it has no rule for, and at Debug for
// what only helps to follow a generation.
func SetLogger(l *slog.Logger) { diagnostics.Store(l) }

// Logger returns the logger set by SetLogger, or slog.Default(); it is never nil.
// The pipeline reports through it too, so a run has one place its diagnostics go.
func Logger() *slog.Logger {
	if l := diagnostics.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// goType renders the dynamic type of v, the %T of a diagnostic.
func goType(v any) string { return fmt.Sprintf("%T", v) }
