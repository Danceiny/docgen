package engine

import "go/types"

// OwnKeyPrefix is the prefix of the full key of every type of the documented
// module: its module path with the slashes turned into dots, and a dot. A
// configuration names a type by its full key.
func OwnKeyPrefix() string { return ownKeyPrefix() }

// TypeKeysOfModule returns the full keys of the types that the packages of the
// module declare at package level, which are the ones a configuration can name.
func (s *GenerationSession) TypeKeysOfModule() []string {
	var keys []string
	for _, pkg := range s.packages {
		if pkg.Types == nil || !isOwnImportPath(pkg.PkgPath) {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			if _, ok := scope.Lookup(name).(*types.TypeName); ok {
				keys = append(keys, fullComponentName(pkg.PkgPath, name))
			}
		}
	}
	return keys
}
