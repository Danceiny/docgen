package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// GenerationSession owns the package graph for exactly one generated
// document. Keeping the graph here makes model and API generation share one
// packages.Load result while keeping AST-backed state bounded by the session.
type GenerationSession struct {
	moduleDir string
	packages  []*packages.Package
	imports   map[string]*packages.Package
	resolving map[string]bool
	hidden    map[string]bool // the keys of the types of the module that the document hides
	audience  Audience
	mu        sync.Mutex
}

// SessionOption configures a GenerationSession.
type SessionOption func(*GenerationSession)

// WithAudience sets who the document generated in this session is written for.
func WithAudience(a Audience) SessionOption {
	return func(s *GenerationSession) { s.audience = a }
}

func NewGenerationSession(moduleDir string, opts ...SessionOption) (*GenerationSession, error) {
	abs, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, fmt.Errorf("resolve module dir: %w", err)
	}
	info, err := os.Stat(filepath.Join(abs, "go.mod"))
	if err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("go.mod is a directory")
		}
		return nil, fmt.Errorf("module dir %s has no go.mod: %w; docgen documents the Go module in the current directory, so run it at the root of the module or name that directory with -C", abs, err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedImports | packages.NeedDeps | packages.NeedModule,
		Dir: abs,
	}
	pkgs, loadErr := packages.Load(cfg, "./...")
	if loadErr != nil {
		return nil, fmt.Errorf("load package graph: %w", loadErr)
	}
	if err := validatePackageGraph(pkgs); err != nil {
		return nil, err
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].ID < pkgs[j].ID })
	s := &GenerationSession{moduleDir: abs, packages: pkgs, imports: make(map[string]*packages.Package), resolving: make(map[string]bool), hidden: make(map[string]bool)}
	for _, pkg := range pkgs {
		if pkg.Module != nil && pkg.Module.Main {
			ModuleName = pkg.Module.Path
			break
		}
	}
	for _, opt := range opts {
		opt(s)
	}
	for _, pkg := range pkgs {
		s.index(pkg, make(map[string]bool))
	}
	return s, nil
}

// describePackageErrors lists the first errors of a package, so that a package
// that does not build says why.
func describePackageErrors(errs []packages.Error) string {
	const shown = 3
	var parts []string
	for i, err := range errs {
		if i == shown {
			parts = append(parts, fmt.Sprintf("and %d more", len(errs)-shown))
			break
		}
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

func validatePackageGraph(pkgs []*packages.Package) error {
	seen := make(map[string]bool)
	var walk func(*packages.Package) error
	walk = func(pkg *packages.Package) error {
		if pkg == nil || seen[pkg.ID] {
			return nil
		}
		seen[pkg.ID] = true
		if len(pkg.Errors) != 0 {
			return fmt.Errorf("package %s does not build: %s (docgen reads the module the way go build ./... does: fix the errors, or run go mod download if a module is missing)", pkg.PkgPath, describePackageErrors(pkg.Errors))
		}
		paths := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			if err := walk(pkg.Imports[path]); err != nil {
				return err
			}
		}
		if pkg.IllTyped {
			// No error of its own and none in what it imports: nothing says why.
			return fmt.Errorf("package %s is ill-typed", pkg.PkgPath)
		}
		return nil
	}
	for _, pkg := range pkgs {
		if err := walk(pkg); err != nil {
			return err
		}
	}
	return nil
}

func (s *GenerationSession) Packages() []*packages.Package { return s.packages }

func (s *GenerationSession) ModuleDir() string { return s.moduleDir }

func (s *GenerationSession) ImportedPackage(importPath string) *packages.Package {
	return s.imports[importPath]
}

func (s *GenerationSession) EnterResolving(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolving[key] {
		return false
	}
	s.resolving[key] = true
	return true
}

// markHidden records that the document hides the type with the key.
func (s *GenerationSession) markHidden(key string) {
	s.mu.Lock()
	s.hidden[key] = true
	s.mu.Unlock()
}

// HiddenTypeOf returns the key of a type that the method takes or returns and
// that the document hides, or "". An operation cannot be described without the
// types it uses, so the pipeline leaves it out and says so.
func (s *GenerationSession) HiddenTypeOf(m *Method) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.hidden) == 0 {
		return ""
	}
	for _, p := range m.Params {
		for _, t := range p.Types {
			if t != nil && s.hidden[t.FullKey] {
				return t.FullKey
			}
		}
	}
	for _, r := range m.Responses {
		if r.DataType != nil && s.hidden[r.DataType.FullKey] {
			return r.DataType.FullKey
		}
	}
	return ""
}

func (s *GenerationSession) LeaveResolving(key string) {
	s.mu.Lock()
	delete(s.resolving, key)
	s.mu.Unlock()
}

func (s *GenerationSession) index(pkg *packages.Package, seen map[string]bool) {
	if pkg == nil || seen[pkg.ID] {
		return
	}
	seen[pkg.ID] = true
	if pkg.PkgPath != "" {
		s.imports[pkg.PkgPath] = pkg
	}
	if pkg.ID != "" {
		s.imports[pkg.ID] = pkg
	}
	paths := make([]string, 0, len(pkg.Imports))
	for path := range pkg.Imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		s.index(pkg.Imports[path], seen)
	}
}

// Close drops references to ASTs and clears legacy process caches before the
// next document is generated. Internal is therefore fully closed before
// public generation starts, without changing filtering semantics.
func (s *GenerationSession) Close() {
	s.packages = nil
	s.imports = nil
	ResetGenerationCaches()
}
