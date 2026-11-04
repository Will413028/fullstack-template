// Package archtest asserts the module boundaries (backend/AGENTS.md) on the transitive dependencies that
// go list reports, so new modules are covered without editing a rule list. depguard in .golangci.yml checks the
// direct imports of every file, including tests; this package catches what reaches a module through another package.
package archtest

import (
	"fmt"
	"strings"
)

const connectPkg = "connectrpc.com/connect"

// Package is a Go package with its transitive non-test dependencies.
type Package struct {
	ImportPath string
	Deps       []string
}

// Check returns one violation per forbidden dependency of a package under internal/modules/<m>: another module,
// net/http, internal/httpapi, the generated contract code (internal/gen) or Connect.
func Check(modulePath string, pkgs []Package) []string {
	modules := modulePath + "/internal/modules/"
	var violations []string
	for _, p := range pkgs {
		self, ok := moduleOf(modules, p.ImportPath)
		if !ok {
			continue
		}
		for _, d := range p.Deps {
			if rule := forbidden(modulePath, self, d); rule != "" {
				violations = append(violations, fmt.Sprintf("%s -> %s: module %s %s", p.ImportPath, d, self, rule))
			}
		}
	}
	return violations
}

// forbidden is the rule a dependency d of module self breaks, "" when it breaks none.
func forbidden(modulePath, self, d string) string {
	under := func(pkg string) bool { return d == pkg || strings.HasPrefix(d, pkg+"/") }
	if other, ok := moduleOf(modulePath+"/internal/modules/", d); ok && other != self {
		return "must not depend on module " + other
	}
	switch {
	case d == "net/http":
		return "must not depend on net/http; handlers live in internal/httpapi"
	case under(modulePath + "/internal/httpapi"):
		return "must not depend on internal/httpapi"
	case under(modulePath + "/internal/gen"):
		return "must not depend on internal/gen; handlers in internal/httpapi convert generated messages"
	case under(connectPkg):
		return "must not depend on Connect; handlers live in internal/httpapi"
	}
	return ""
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
