package pipeline

import (
	"fmt"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
	"github.com/Danceiny/docgen/internal/overlay"
)

// loadedOverlay is an overlay file that has been read, and where it applies.
type loadedOverlay struct {
	name  string
	stage string
	file  *overlay.File
}

// loadOverlays reads the overlay files a document lists, which are relative to
// the module directory. It runs before the module is loaded, so that a file
// that cannot be read stops the run early.
func loadOverlays(dir string, files []config.OverlayFile) ([]loadedOverlay, error) {
	out := make([]loadedOverlay, 0, len(files))
	for i, o := range files {
		f, err := overlay.Load(filepath.Join(dir, o.File))
		if err != nil {
			return nil, fmt.Errorf("overlay[%d]: %w", i, err)
		}
		out = append(out, loadedOverlay{name: o.File, stage: o.Stage, file: f})
	}
	return out, nil
}

// applyOverlays applies the overlays that belong to a stage, in the order the
// configuration lists them.
func applyOverlays(doc *openapi3.T, overlays []loadedOverlay, stage string) {
	for _, o := range overlays {
		if o.stage != stage {
			continue
		}
		for _, key := range o.file.Apply(doc) {
			engine.Logger().Debug("overlay replaced a generated schema", "overlay", o.name, "key", key)
		}
	}
}
