// Command docgen generates OpenAPI documents from the Go source of a module.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/pipeline"
)

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to file")
	configPath = flag.String("config", "docgen.yaml", "path of the configuration file")
	moduleDir  = flag.String("C", "", "run as if started in this directory, the root of the module to document")
	verbose    = flag.Bool("v", false, "also log what only helps to follow a generation, such as each type hidden by its annotation")
	check      = flag.Bool("check", false, "generate into a temporary directory and fail if a document differs from the file on disk; writes nothing")
	version    = flag.Bool("version", false, "print the version and exit")
	docNames   []string
)

func init() {
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: docgen [flags]\n\n"+
			"docgen generates the OpenAPI documents that docgen.yaml describes from the Go source of the\n"+
			"module in the current directory. See https://github.com/Danceiny/docgen for the configuration.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Func("doc", "generate only the document with this name; may be repeated", func(name string) error {
		docNames = append(docNames, name)
		return nil
	})
}

func main() {
	flag.Parse()
	if *version {
		fmt.Println(versionString())
		return
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "docgen: unexpected argument %q: docgen takes flags only; name the module to document with -C and the configuration with -config\n", flag.Arg(0))
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docgen:", err)
		os.Exit(1)
	}
}

// versionString describes this build: the module version it was built from, the
// revision it was built at when known, and the Go toolchain.
func versionString() string {
	version, revision := "(devel)", ""
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				revision = " " + s.Value[:12]
			}
		}
	}
	return fmt.Sprintf("docgen %s%s (%s)", version, revision, runtime.Version())
}

// newLogger returns the logger a run reports its diagnostics to: text on w, at
// Info and above, or at Debug and above when verbose.
func newLogger(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

func run() error {
	st := time.Now()

	// CPU profiling
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}

	if *moduleDir != "" {
		if err := os.Chdir(*moduleDir); err != nil {
			return err
		}
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if err := refuseToOverwrite(dir, *configPath, cfg); err != nil {
		return err
	}

	logger := newLogger(os.Stderr, *verbose)
	opts := pipeline.Options{Dir: dir, Config: cfg, Docs: docNames, Logger: logger}
	if *check {
		return checkDocuments(opts, logger)
	}
	if err := pipeline.Run(opts); err != nil {
		return err
	}

	// Memory profiling
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			return err
		}
		if err := pprof.WriteHeapProfile(f); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	logger.Info("done", "took", time.Since(st).Round(time.Millisecond))
	return nil
}

// checkDocuments is -check: it fails when a document on disk is not what
// generating it gives, and says which ones.
func checkDocuments(opts pipeline.Options, logger *slog.Logger) error {
	drifts, err := pipeline.Check(opts)
	if err != nil {
		return err
	}
	if len(drifts) == 0 {
		logger.Info("documents are up to date")
		return nil
	}
	for _, d := range drifts {
		fmt.Fprintf(os.Stderr, "%s (%s): %s\n", d.Output, d.Document, d.Reason)
	}
	return errors.New("documents are out of date; run docgen to write them")
}

// refuseToOverwrite fails when the output of a document is the configuration file,
// which the first run would replace with a document.
func refuseToOverwrite(dir, configPath string, cfg *config.Config) error {
	abs := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		return filepath.Clean(p)
	}
	for i, d := range cfg.Docs {
		if abs(d.Output) == abs(configPath) {
			return fmt.Errorf("docs[%d].output is %s, the configuration file itself, which the document would replace", i, d.Output)
		}
	}
	return nil
}
