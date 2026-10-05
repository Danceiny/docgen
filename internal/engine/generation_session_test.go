package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestValidatePackageGraphFailsClosed(t *testing.T) {
	t.Run("package error", func(t *testing.T) {
		err := validatePackageGraph([]*packages.Package{{ID: "broken", PkgPath: "example.com/broken", Errors: []packages.Error{{Msg: "syntax error"}}}})
		if err == nil {
			t.Fatal("package load errors must fail closed")
		}
	})
	t.Run("ill typed", func(t *testing.T) {
		err := validatePackageGraph([]*packages.Package{{ID: "ill", PkgPath: "example.com/ill", IllTyped: true}})
		if err == nil {
			t.Fatal("ill-typed packages must fail closed")
		}
	})
}

// A package that does not build says why: the first errors are in the message,
// at their positions, and so is the package of a dependency that is the cause.
func TestValidatePackageGraphSaysWhy(t *testing.T) {
	broken := &packages.Package{ID: "dep", PkgPath: "example.com/dep", IllTyped: true, Errors: []packages.Error{
		{Pos: "dep.go:3:5", Msg: "undefined: x"},
		{Pos: "dep.go:4:5", Msg: "undefined: y"},
		{Pos: "dep.go:5:5", Msg: "undefined: z"},
		{Pos: "dep.go:6:5", Msg: "undefined: w"},
	}}
	user := &packages.Package{ID: "user", PkgPath: "example.com/user", IllTyped: true,
		Imports: map[string]*packages.Package{"example.com/dep": broken}}

	err := validatePackageGraph([]*packages.Package{user})
	if err == nil {
		t.Fatal("a package that imports one that does not build must fail closed")
	}
	for _, want := range []string{"example.com/dep", "dep.go:3:5", "undefined: x", "undefined: z", "and 1 more"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "undefined: w") {
		t.Errorf("the error lists more than the first errors: %q", err)
	}
}

func TestAGenerationSessionSaysWhereToRunTheToolWhenThereIsNoModule(t *testing.T) {
	_, err := NewGenerationSession(t.TempDir())
	if err == nil {
		t.Fatal("a directory without go.mod was loaded")
	}
	for _, want := range []string{"has no go.mod", "root of the module", "-C"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say %q: %v", want, err)
		}
	}
}

// The sessions of the documents of one module share its package graph, and
// closing one does not take it from the others.
func TestSessionsOfOneModuleShareItsPackages(t *testing.T) {
	withSettings(t, Settings{}, ModuleName) // a session names the module it loads, for good
	const dir = "../pipeline/testdata/ondemand"
	module, err := LoadModule(dir)
	require.NoError(t, err)
	first, err := NewGenerationSession(dir, WithModule(module))
	require.NoError(t, err)
	second, err := NewGenerationSession(dir, WithModule(module))
	require.NoError(t, err)

	require.NotEmpty(t, first.Packages())
	assert.Same(t, first.Packages()[0], second.Packages()[0], "the same packages, not a second load of them")
	first.Close()
	assert.Nil(t, first.Packages())
	assert.NotEmpty(t, second.Packages(), "closing a session does not take the packages from the module")
	assert.NotEmpty(t, module.packages)

	_, err = LoadModule(t.TempDir())
	assert.ErrorContains(t, err, "has no go.mod")
}
