package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the tests run the command itself: the test binary, started with
// DOCGEN_AS_CLI set, is docgen.
func TestMain(m *testing.M) {
	if os.Getenv("DOCGEN_AS_CLI") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

type result struct {
	stdout, stderr string
	code           int
}

func docgenCLI(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "DOCGEN_AS_CLI=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return result{stdout.String(), stderr.String(), code}
}

func copyExample(t *testing.T) string {
	t.Helper()
	from, to := "../../examples/petstore", t.TempDir()
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return to
}

// The command exits 0 when the checked-in documents are current and says so
// quietly, exits 1 and names the stale file when one is not, writes nothing in
// check mode, and is brought back to 0 by generating.
func TestCheckModeThroughTheCommand(t *testing.T) {
	dir := copyExample(t)

	if got := docgenCLI(t, "-C", dir, "-check"); got.code != 0 {
		t.Fatalf("current documents: exit %d\n%s", got.code, got.stderr)
	}

	stale := filepath.Join(dir, "docs/api/public.yaml")
	data, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(data), "Petstore API", "Petshop API", 1)
	if changed == string(data) {
		t.Fatal("the edit of the fixture changed nothing")
	}
	if err := os.WriteFile(stale, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}

	got := docgenCLI(t, "-C", dir, "-check")
	if got.code != 1 {
		t.Fatalf("a stale document: exit %d, want 1\n%s", got.code, got.stderr)
	}
	for _, want := range []string{"docs/api/public.yaml", "out of date", "first at line"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr does not say %q:\n%s", want, got.stderr)
		}
	}
	if after, _ := os.ReadFile(stale); string(after) != changed {
		t.Error("-check wrote a document")
	}

	if got := docgenCLI(t, "-C", dir, "-doc", "public"); got.code != 0 {
		t.Fatalf("generating: exit %d\n%s", got.code, got.stderr)
	}
	if got := docgenCLI(t, "-C", dir, "-check"); got.code != 0 {
		t.Fatalf("after generating: exit %d\n%s", got.code, got.stderr)
	}
}

func TestCommandSaysWhatWentWrong(t *testing.T) {
	empty := t.TempDir()

	got := docgenCLI(t, "-C", empty)
	if got.code != 1 || !strings.Contains(got.stderr, "docgen: read config") || !strings.Contains(got.stderr, "docgen.yaml") {
		t.Errorf("no configuration: exit %d\n%s", got.code, got.stderr)
	}

	if err := os.WriteFile(filepath.Join(empty, "docgen.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got = docgenCLI(t, "-C", empty)
	if got.code != 1 || !strings.Contains(got.stderr, "the configuration is empty") {
		t.Errorf("empty configuration: exit %d\n%s", got.code, got.stderr)
	}

	got = docgenCLI(t, "-C", copyExample(t), "-doc", "nope")
	if got.code != 1 || !strings.Contains(got.stderr, `no document named "nope"`) || !strings.Contains(got.stderr, "internal") {
		t.Errorf("unknown document: exit %d\n%s", got.code, got.stderr)
	}

	got = docgenCLI(t, "-version")
	if got.code != 0 || !strings.HasPrefix(got.stdout, "docgen ") {
		t.Errorf("-version: exit %d, stdout %q", got.code, got.stdout)
	}
}

// The command takes flags only: a directory given as an argument would be taken for
// nothing, and the run would document the current directory.
func TestTheCommandRefusesArguments(t *testing.T) {
	got := docgenCLI(t, "./docs")
	if got.code != 2 || !strings.Contains(got.stderr, `unexpected argument "./docs"`) || !strings.Contains(got.stderr, "-C") {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
}

// An output that is the configuration file would replace the configuration with a
// document on the first run.
func TestTheCommandRefusesToOverwriteItsConfiguration(t *testing.T) {
	dir := copyExample(t)
	config, err := os.ReadFile(filepath.Join(dir, "docgen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(config), "output: docs/api/internal.yaml", "output: docgen.yaml", 1)
	if bad == string(config) {
		t.Fatal("the edit changed nothing")
	}
	if err := os.WriteFile(filepath.Join(dir, "docgen.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	got := docgenCLI(t, "-C", dir)
	if got.code != 1 || !strings.Contains(got.stderr, "the configuration file itself") {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "docgen.yaml"))
	if string(after) != bad {
		t.Error("the configuration was overwritten")
	}
}
