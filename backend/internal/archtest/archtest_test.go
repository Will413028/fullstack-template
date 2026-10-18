package archtest

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// listPackages reads every package of the main module with its transitive non-test dependencies (go list -deps).
// Test-only imports are left out: a module's integration test may pull in net/http through testcontainers.
func listPackages(t *testing.T) (modulePath string, pkgs []Package) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json=ImportPath,Deps,Module", "./...")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p struct {
			Package
			Module struct{ Path string }
		}
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		modulePath = p.Module.Path
		pkgs = append(pkgs, p.Package)
	}
	return modulePath, pkgs
}

func TestModulesFollowDependencyRules(t *testing.T) {
	modulePath, pkgs := listPackages(t)
	for _, v := range Check(modulePath, pkgs) {
		t.Error(v)
	}
}

// Deliberate violations: if a rule stops working, this test fails.
func TestCheckRejectsViolations(t *testing.T) {
	const mod = "example.com/app"
	const m = mod + "/internal/modules/"
	cases := []struct {
		name string
		pkg  Package
		want string
	}{
		{"module imports another module", Package{m + "bb", []string{m + "aa"}}, "must not depend on module aa"},
		{"module subpackage imports another module's subpackage", Package{m + "bb/store", []string{m + "aa/store"}}, "must not depend on module aa"},
		{"module reaches net/http", Package{m + "aa", []string{"net/http"}}, "must not depend on net/http"},
		{"module imports httpapi", Package{m + "aa", []string{mod + "/internal/httpapi"}}, "must not depend on internal/httpapi"},
		{"module imports httpapi subpackage", Package{m + "aa", []string{mod + "/internal/httpapi/gen"}}, "must not depend on internal/httpapi"},
	}
	for _, c := range cases {
		got := Check(mod, []Package{c.pkg})
		if !slices.ContainsFunc(got, func(v string) bool { return strings.Contains(v, c.want) }) {
			t.Errorf("%s: want a violation containing %q, got %v", c.name, c.want, got)
		}
	}
	allowed := []Package{
		{m + "aa", []string{m + "aa/store", mod + "/internal/platform/authctx", mod + "/internal/httpapiutil", "context", "net/url"}},
		{m + "aa/store", []string{"github.com/jackc/pgx/v5"}},
		{m + "aabb", []string{m + "aabb/store"}},
		{mod + "/internal/httpapi", []string{m + "aa", m + "bb", "net/http"}},
	}
	if got := Check(mod, allowed); len(got) != 0 {
		t.Errorf("allowed dependencies were rejected: %v", got)
	}
}
