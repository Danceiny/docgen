package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The binary links the modules `go list` names, and the license of each has to
// be reproduced where the binary is distributed. A dependency added or bumped
// without THIRD_PARTY_NOTICES.md following is caught here.
func TestThirdPartyNoticesListEveryLinkedModule(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool to ask which modules are linked")
	}
	cmd := exec.Command(goTool, "list", "-deps", "-f", "{{with .Module}}{{.Path}} {{.Version}}{{end}}", "./cmd/docgen")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	notices, err := os.ReadFile("../../THIRD_PARTY_NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, version, ok := strings.Cut(line, " ")
		if !ok || path == "github.com/Danceiny/docgen" {
			continue
		}
		checked++
		if want := "| `" + path + "` | " + version + " |"; !strings.Contains(string(notices), want) {
			t.Errorf("THIRD_PARTY_NOTICES.md has no row %q", want)
		}
	}
	if checked == 0 {
		t.Fatal("go list named no third-party module: the check checked nothing")
	}
}
