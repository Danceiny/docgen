package engine

import (
	"sync"
)

func ResetGenerationCaches() {
	typeDescCache = sync.Map{}
	pkgCache = sync.Map{}
	pkgLoadError = sync.Map{}
	typeKeyCache = sync.Map{}
}

// typeDescCache keeps recursive types from being described twice:
// map[string]*TypeDescriptor.
var typeDescCache sync.Map

// ModuleName is the path of the documented module; a session sets it from the
// module it loads.
var (
	ModuleName = ""
)
