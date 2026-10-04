package errcat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const valid = `[
  {"name": "Success", "code": 0, "message": "Success", "httpCode": 200},
  {"name": "NotFoundErr", "code": 100000404, "message": "not found", "httpCode": 404},
  {"name": "Timeout_2", "code": 100000504, "message": "", "httpCode": 504}
]`

func TestParseKeepsTheOrderOfTheFile(t *testing.T) {
	c, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range c.Entries() {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "Success,NotFoundErr,Timeout_2" {
		t.Fatalf("entries = %s", got)
	}
	if e := c.Entries()[1]; e.Code != 100000404 || e.Message != "not found" || e.HTTPCode != 404 {
		t.Fatalf("NotFoundErr = %+v", e)
	}
}

func TestParseAcceptsAnEmptyCatalog(t *testing.T) {
	c, err := Parse([]byte(" [ ] \n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries()) != 0 {
		t.Fatalf("entries = %v", c.Entries())
	}
}

func TestParseRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"not an array", `{"name": "A"}`, "want a JSON array"},
		{"null", `null`, "want a JSON array"},
		{"empty file", ``, "want a JSON array"},
		{"unknown field", `[{"name":"A","code":1,"message":"m","httpCode":400,"status":1}]`, "unknown field"},
		{"code is not a whole number", `[{"name":"A","code":1.5,"message":"m","httpCode":400}]`, "decode error catalog"},
		{"code overflows", `[{"name":"A","code":4294967296,"message":"m","httpCode":400}]`, "decode error catalog"},
		{"data after the array", `[] []`, "unexpected data"},
		{"empty name", `[{"name":"","code":1,"message":"m","httpCode":400}]`, "errors[0].name: required"},
		{"name with a dot", `[{"name":"a.B","code":1,"message":"m","httpCode":400}]`, `"a.B" is not an identifier`},
		{"name with a space", `[{"name":"A B","code":1,"message":"m","httpCode":400}]`, `"A B" is not an identifier`},
		{"name starting with a digit", `[{"name":"1A","code":1,"message":"m","httpCode":400}]`, `"1A" is not an identifier`},
		{"duplicate name", `[{"name":"A","code":1,"message":"m","httpCode":400},{"name":"A","code":2,"message":"n","httpCode":500}]`, `errors[1].name: "A" is already errors[0]`},
		{"http code too low", `[{"name":"A","code":1,"message":"m","httpCode":99}]`, "errors[0].httpCode: want 100 to 599, got 99"},
		{"http code too high", `[{"name":"A","code":1,"message":"m","httpCode":600}]`, "errors[0].httpCode: want 100 to 599, got 600"},
		{"http code missing", `[{"name":"A","code":1,"message":"m"}]`, "errors[0].httpCode: want 100 to 599, got 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse([]byte(tc.in))
			if err == nil {
				t.Fatalf("accepted %s: %+v", tc.in, c.Entries())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestParseReportsEveryProblemAtOnce(t *testing.T) {
	_, err := Parse([]byte(`[
		{"name":"","code":1,"message":"m","httpCode":400},
		{"name":"A","code":1,"message":"m","httpCode":42},
		{"name":"A","code":1,"message":"m","httpCode":400}
	]`))
	if err == nil {
		t.Fatal("accepted a catalog with three problems")
	}
	for _, want := range []string{"errors[0].name: required", "errors[1].httpCode", "errors[2].name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not report %q", err, want)
		}
	}
}

func TestFindNamesTheErrorAfterTheLastDot(t *testing.T) {
	c, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key  string
		want string // the name found, "" for none
	}{
		{"NotFoundErr", "NotFoundErr"},
		{"errors.NotFoundErr", "NotFoundErr"},
		{"example.com.shop.errors.NotFoundErr", "NotFoundErr"},
		{"Timeout_2", "Timeout_2"},
		{"example.com.shop.errors.BatchResult", ""}, // a type in the errors' package that is not an error
		{"notfounderr", ""},                         // names are case-sensitive
		{"NotFoundErr.", ""},
		{"", ""},
		{"404", ""}, // a status is not a name
	} {
		e, ok := c.Find(tc.key)
		if ok != (tc.want != "") || e.Name != tc.want {
			t.Errorf("Find(%q) = %q, %v; want %q", tc.key, e.Name, ok, tc.want)
		}
	}
}

func TestANilCatalogHasNoErrors(t *testing.T) {
	var c *Catalog
	if len(c.Entries()) != 0 {
		t.Fatal("a nil catalog has entries")
	}
	if _, ok := c.Find("NotFoundErr"); ok {
		t.Fatal("a nil catalog found an error")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "errors.json")
	if err := os.WriteFile(good, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(good)
	if err != nil || len(c.Entries()) != 3 {
		t.Fatalf("Load = %v, %v", c, err)
	}

	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil || !strings.Contains(err.Error(), "read error catalog") {
		t.Fatalf("a missing file: %v", err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"name":""}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("an invalid file must be reported with its path: %v", err)
	}
}
