package engine

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

// recorder is a slog.Handler that keeps every record, whatever its level.
type recorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *recorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
	return nil
}

func (r *recorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *recorder) WithGroup(string) slog.Handler      { return r }

// attrsOf renders the attributes of a record by key.
func attrsOf(rec slog.Record) map[string]string {
	out := map[string]string{}
	rec.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.String()
		return true
	})
	return out
}

// captureLogs sends the engine's diagnostics to a recorder for one test.
func captureLogs(t *testing.T) *recorder {
	t.Helper()
	r := &recorder{}
	SetLogger(slog.New(r))
	t.Cleanup(func() { SetLogger(nil) })
	return r
}

func TestLoggerFallsBackToTheSlogDefault(t *testing.T) {
	t.Cleanup(func() { SetLogger(nil) })

	SetLogger(nil)
	if Logger() != slog.Default() {
		t.Fatal("with no logger set the engine reports to slog.Default()")
	}

	l := slog.New(&recorder{})
	SetLogger(l)
	if Logger() != l {
		t.Fatal("Logger() is not the logger SetLogger was given")
	}

	SetLogger(nil)
	if Logger() != slog.Default() {
		t.Fatal("SetLogger(nil) did not restore the default")
	}
}

// What the engine says has a level that tells the reader how much it matters:
// Warn when the document is probably missing something, Error for a construct
// the engine has no rule for, Debug for what only helps to follow a run.
func TestDiagnosticsReachTheLoggerAtTheirLevel(t *testing.T) {
	tests := []struct {
		name  string
		do    func()
		level slog.Level
		msg   string
		attrs map[string]string
	}{
		{
			name:  "an enum whose underlying type has no schema",
			do:    func() { generateEnumSchemaFromEntry(nil, "Mystery") },
			level: slog.LevelWarn,
			msg:   "enum underlying type has no schema, using object",
			attrs: map[string]string{"underlyingType": "Mystery"},
		},
		{
			name: "a selector expression the parser cannot qualify",
			do: func() {
				sel := &ast.SelectorExpr{X: &ast.ParenExpr{X: ast.NewIdent("x")}, Sel: ast.NewIdent("Widget")}
				(&TypeParser{}).parseSelector(sel, &ParseContext{})
			},
			level: slog.LevelWarn,
			msg:   "selector expression has an unsupported qualifier",
			attrs: map[string]string{"selector": "Widget"},
		},
		{
			name: "an expression the key generator has no rule for",
			do: func() {
				(&TypeParser{pkg: &packages.Package{}}).generateTypeKeyUncached(&ast.BadExpr{})
			},
			level: slog.LevelError,
			msg:   "expression has no key rule",
			attrs: map[string]string{"expr": "*ast.BadExpr"},
		},
		{
			name: "a type hidden by its annotation, which only someone following the run wants to see",
			do: func() {
				file, err := parser.ParseFile(token.NewFileSet(), "p.go", "package p\n\n//apidoc:hidden\ntype Secret struct{}\n", parser.ParseComments)
				if err != nil {
					t.Fatal(err)
				}
				spec := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
				shouldHideType(&packages.Package{Syntax: []*ast.File{file}}, spec, Audience{Name: "internal"})
			},
			level: slog.LevelDebug,
			msg:   "type is hidden by its visibility annotation",
			attrs: map[string]string{"type": "Secret"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := captureLogs(t)
			tc.do()

			if len(rec.records) != 1 {
				t.Fatalf("got %d records, want exactly one: %v", len(rec.records), rec.records)
			}
			got := rec.records[0]
			if got.Level != tc.level {
				t.Errorf("level = %v, want %v", got.Level, tc.level)
			}
			if got.Message != tc.msg {
				t.Errorf("message = %q, want %q", got.Message, tc.msg)
			}
			attrs := attrsOf(got)
			for k, want := range tc.attrs {
				if attrs[k] != want {
					t.Errorf("attribute %s = %q, want %q (all: %v)", k, attrs[k], want, attrs)
				}
			}
		})
	}
}

// The engine is a library to its command: it must not print on its own,
// because the command decides where diagnostics go and how loud they are. The
// scan reads the source, so a new use of fmt's print functions or of a log
// package in any file of the package fails here instead of landing in someone's
// terminal.
func TestEngineReportsOnlyThroughItsLogger(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files found: %v", err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			if path := strings.Trim(imp.Path.Value, `"`); path == "log" {
				t.Errorf("%s imports %q; report through Logger() instead", name, path)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case pkg.Name == "fmt" && (sel.Sel.Name == "Print" || sel.Sel.Name == "Printf" || sel.Sel.Name == "Println"):
				t.Errorf("%s:%d calls fmt.%s; report through Logger() instead", name, fset.Position(sel.Pos()).Line, sel.Sel.Name)
			case pkg.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr"):
				t.Errorf("%s:%d writes to os.%s; report through Logger() instead", name, fset.Position(sel.Pos()).Line, sel.Sel.Name)
			}
			return true
		})
	}
}

// The same warning about the same place is told once in a run, not once for each
// document that is built.
func TestAWarningAboutAPlaceIsToldOncePerRun(t *testing.T) {
	logs := captureLogs(t)
	Configure(Settings{})
	for i := 0; i < 3; i++ {
		warnAt("something is wrong here", "method", "Get", "at", "s.go:10")
	}
	warnAt("something is wrong here", "method", "Get", "at", "s.go:20")
	warnAt("something else is wrong here", "method", "Get", "at", "s.go:10")
	if got := len(logs.records); got != 3 {
		t.Fatalf("%d warnings, want 3: one for each different thing", got)
	}
	Configure(Settings{})
	warnAt("something is wrong here", "method", "Get", "at", "s.go:10")
	if got := len(logs.records); got != 4 {
		t.Fatalf("%d warnings: a new run tells again", got)
	}
}
