package pipeline

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
	"github.com/Danceiny/docgen/internal/suggest"
)

// typeKey is a key of the configuration that names a type, and where it is.
type typeKey struct{ in, key string }

// typeKeysOf lists the keys of the configuration that name a type: the keys of
// type_map, request.multipart, request.query and response.binary, the types of
// headers and request.runtime_only, and generic_titles.
func typeKeysOf(cfg *config.Config) []typeKey {
	var keys []typeKey
	add := func(in, key string) { keys = append(keys, typeKey{in, key}) }
	for _, key := range cfg.GenericTitles {
		add("generic_titles", key)
	}
	for key := range cfg.TypeMap {
		add("type_map", key)
	}
	for name, key := range cfg.Headers.Types {
		add("headers.types."+name, key)
	}
	for key := range cfg.Request.Multipart {
		add("request.multipart", key)
	}
	for key := range cfg.Request.Query {
		add("request.query", key)
	}
	for _, r := range cfg.Request.RuntimeOnly {
		add("request.runtime_only", r.Type)
	}
	for key := range cfg.Response.Binary {
		add("response.binary", key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].in != keys[j].in {
			return keys[i].in < keys[j].in
		}
		return keys[i].key < keys[j].key
	})
	return keys
}

// warnAboutTypeKeysThatNameNoType says so for each key of the configuration that
// names a type of the module that the module does not declare. Such an entry
// does nothing, and it is almost always a typo: a key that is not spelled
// exactly as the type's full key is never looked up. Keys of other modules are
// not checked: whether a module uses a type of another one is not known until
// the types are met.
func warnAboutTypeKeysThatNameNoType(cfg *config.Config, session *engine.GenerationSession) {
	own := engine.OwnKeyPrefix()
	declared := session.TypeKeysOfModule()
	have := make(map[string]bool, len(declared))
	for _, key := range declared {
		have[key] = true
	}
	for _, k := range typeKeysOf(cfg) {
		if !strings.HasPrefix(k.key, own) || have[k.key] {
			continue
		}
		args := []any{"in", k.in, "key", k.key}
		if guess := suggest.Nearest(k.key, declared); guess != "" {
			args = append(args, "didYouMean", guess)
		}
		engine.Logger().Warn("the configuration names a type that the module does not declare, so the entry does nothing; a type is named by its full key, the import path with dots and then the name of the type", args...)
	}
}

// warnAboutForceKeepsThatNameNoSchema says so for each schema that force_keep
// keeps although the document has no schema of that key.
func warnAboutForceKeepsThatNameNoSchema(d config.Doc, doc *openapi3.T) {
	var names []string
	for name := range doc.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, key := range d.ForceKeep {
		if doc.Components.Schemas[key] != nil {
			continue
		}
		args := []any{"document", d.Name, "key", key}
		if guess := suggest.Nearest(key, names); guess != "" {
			args = append(args, "didYouMean", guess)
		}
		engine.Logger().Warn("force_keep names a schema that the document does not have, so it keeps nothing; a schema is named by the full key of its type", args...)
	}
}
