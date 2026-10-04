// Package pipeline generates the OpenAPI documents of a Go module: it loads the
// module, builds the schemas and operations with the engine, filters them per
// audience and writes the YAML files.
package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
)

// rootDir is the module root Run was given, and outDir the directory the
// documents are written under, which is rootDir unless Options.OutputDir says
// otherwise. A run generates one configuration at a time.
var rootDir, outDir string

// Options selects what Run generates.
type Options struct {
	// Dir is the root of the module to document; it must contain go.mod.
	Dir string
	// Config says which documents exist and how they are built.
	Config *config.Config
	// Docs are the names of the documents to build; with none, all of them.
	Docs []string
	// Logger receives the diagnostics of the run; nil means slog.Default().
	Logger *slog.Logger
	// OutputDir, when set, is where the documents are written instead of Dir.
	// Their paths in the configuration are relative to it.
	OutputDir string
}

// Run generates the selected documents. It returns the first error instead of
// exiting the process, so callers can clean up (stop profilers, close files).
func Run(opts Options) error {
	if opts.Config == nil {
		return errors.New("pipeline: no configuration")
	}
	docs, err := opts.Config.Select(opts.Docs)
	if err != nil {
		return err
	}
	rootDir, outDir = opts.Dir, opts.Dir
	if opts.OutputDir != "" {
		outDir = opts.OutputDir
	}

	settings := engineSettings(opts.Config)
	if settings.Errors, err = loadErrorCatalog(opts.Dir, opts.Config.Errors); err != nil {
		return err
	}
	engine.Configure(settings)
	engine.SetLogger(opts.Logger)

	for i, d := range docs {
		start := time.Now()
		if err := buildDoc(d, opts.Config, i == 0); err != nil {
			return fmt.Errorf("document %q: %w", d.Name, err)
		}
		engine.Logger().Info("document generated", "document", d.Name, "output", d.Output, "took", time.Since(start).Round(time.Millisecond))
	}
	return nil
}

// Drift is a document whose file on disk is not what generating it gives.
type Drift struct {
	// Document is the name of the document, Output its path in the module.
	Document, Output string
	// Reason says how the file differs: it is missing, or where it differs.
	Reason string
}

// Check generates the selected documents into a temporary directory and returns
// those that differ from the files they would be written to. It changes nothing
// under opts.Dir: it is the gate that says the checked-in documents are current.
func Check(opts Options) ([]Drift, error) {
	tmp, err := os.MkdirTemp("", "docgen-check-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // best effort: it is a temporary directory

	generate := opts
	generate.OutputDir = tmp
	if err := Run(generate); err != nil {
		return nil, err
	}
	docs, err := opts.Config.Select(opts.Docs)
	if err != nil {
		return nil, err
	}

	var drifts []Drift
	for _, d := range docs {
		fresh, err := os.ReadFile(filepath.Join(tmp, d.Output))
		if err != nil {
			return nil, err
		}
		onDisk, err := os.ReadFile(filepath.Join(opts.Dir, d.Output))
		switch {
		case os.IsNotExist(err):
			drifts = append(drifts, Drift{d.Name, d.Output, "does not exist"})
		case err != nil:
			return nil, err
		case !bytes.Equal(fresh, bytes.ReplaceAll(onDisk, []byte("\r\n"), []byte("\n"))):
			// A checkout that turns line feeds into carriage returns and line feeds
			// (core.autocrlf on Windows) has not made the document stale.
			drifts = append(drifts, Drift{d.Name, d.Output, describeDifference(onDisk, fresh)})
		}
	}
	return drifts, nil
}

// describeDifference says where two versions of a file first differ.
func describeDifference(onDisk, fresh []byte) string {
	a, b := bytes.Split(onDisk, []byte("\n")), bytes.Split(fresh, []byte("\n"))
	for i := 0; i < len(a) && i < len(b); i++ {
		if !bytes.Equal(a[i], b[i]) {
			return fmt.Sprintf("differs from what generation gives, first at line %d", i+1)
		}
	}
	return fmt.Sprintf("has %d lines and generation gives %d", len(a), len(b))
}
