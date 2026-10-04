package engine

import (
	"os"
	"testing"
)

// The fixtures under testdata are packages of this very module. A session sets
// ModuleName from the module it loads; the tests that build a parser without one
// need it set here.
func TestMain(m *testing.M) {
	ModuleName = "github.com/Danceiny/docgen"
	os.Exit(m.Run())
}
