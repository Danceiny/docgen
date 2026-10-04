package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enumValues(entries []EnumEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name+"="+e.Value)
	}
	return out
}

// The values of an enum are what the compiler says they are, however they are
// declared, and all the constants of the type are there, the ones that repeat
// the type implicitly too.
func TestEnumValuesAreWhatTheCompilerComputes(t *testing.T) {
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/enums")

	assert.Equal(t, []string{"Low=0", "Medium=1", "High=2"}, enumValues(collectEnumEntries(pkg, "Priority")))
	assert.Equal(t, []string{"Unknown=-1", "Base=10", "Next=15"}, enumValues(collectEnumEntries(pkg, "Level")))
	assert.Equal(t, []string{"Read=1", "Write=2", "Exec=4"}, enumValues(collectEnumEntries(pkg, "Mask")))
	assert.Equal(t, []string{"NameA=x-a", "NameB=b"}, enumValues(collectEnumEntries(pkg, "Name")), "a blank constant is no value")
	assert.Empty(t, collectEnumEntries(pkg, "Plain"))

	priority := collectEnumEntries(pkg, "Priority")
	require.Len(t, priority, 3)
	assert.Equal(t, "is the first and has the iota.", priority[0].Comment)
	assert.Equal(t, "is the third.", priority[2].Comment)

	// A name that is an alias is a name for string: the constants that are declared
	// with it are its values, not every string constant of the package.
	assert.Equal(t, []string{"AliasA=a"}, enumValues(collectEnumEntries(pkg, "Alias")))
}

// legacy_schema_shapes keeps what documents generated before the fix have: the
// constants declared with the type and a literal, and an empty value for the rest.
func TestEnumValuesOfLegacyDocuments(t *testing.T) {
	withSettings(t, Settings{CompatLegacySchemaShapes: true}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/enums")

	assert.Equal(t, []string{"Low="}, enumValues(collectEnumEntries(pkg, "Priority")), "only the first, and its iota is no literal")
	assert.Equal(t, []string{"Unknown=", "Base=10", "Next="}, enumValues(collectEnumEntries(pkg, "Level")))
	assert.Equal(t, []string{"NameA=", "NameB=b", "_=skipped"}, enumValues(collectEnumEntries(pkg, "Name")))
}
