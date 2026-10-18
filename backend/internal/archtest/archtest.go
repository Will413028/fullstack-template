// Package archtest asserts the module boundaries (plan invariant I4) on the transitive dependencies that
// go list reports, so new modules are covered without editing a rule list. depguard in .golangci.yml checks the
// direct imports of every file, including tests; this package catches what reaches a module through another package.
package archtest

import (
	"fmt"
	"strings"
)

// Package is a Go package with its transitive non-test dependencies.
type Package struct {
	ImportPath string
	Deps       []string
}

// Check returns one violation per forbidden dependency of a package under internal/modules/<m>: another module,
// net/http, or internal/httpapi.
func Check(modulePath string, pkgs []Package) []string {
	modules := modulePath + "/internal/modules/"
	httpapi := modulePath + "/internal/httpapi"
	var violations []string
	for _, p := range pkgs {
		self, ok := moduleOf(modules, p.ImportPath)
		if !ok {
			continue
		}
		for _, d := range p.Deps {
			var rule string
			if other, ok := moduleOf(modules, d); ok && other != self {
				rule = "must not depend on module " + other
			} else if d == "net/http" {
				rule = "must not depend on net/http; handlers live in internal/httpapi"
			} else if d == httpapi || strings.HasPrefix(d, httpapi+"/") {
				rule = "must not depend on internal/httpapi"
			}
			if rule != "" {
				violations = append(violations, fmt.Sprintf("%s -> %s: module %s %s", p.ImportPath, d, self, rule))
			}
		}
	}
	return violations
}

// moduleOf returns the module name of an import path under internal/modules/.
func moduleOf(modules, importPath string) (string, bool) {
	rest, ok := strings.CutPrefix(importPath, modules)
	if !ok || rest == "" {
		return "", false
	}
	name, _, _ := strings.Cut(rest, "/")
	return name, true
}
