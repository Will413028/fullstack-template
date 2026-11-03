package rpc

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// appCode is a code the server can send: an apperr.New or apperr.Invalid call, or an apperr.Error literal.
type appCode struct {
	pos   string
	field bool // Kind is Validation, so Convert sends a FieldViolation reason; otherwise an ErrorInfo reason
	code  string
}

// Every code a non-test file in the backend passes to apperr.New or apperr.Invalid, or sets in an apperr.Error
// literal, is declared in app.common.v1, which the frontend translates value by value; Convert otherwise drops it
// and only logs it (Undeclared). The codes are read from the source, so a new code in any package is covered without
// editing a list here. A code assigned later (e.Code = ...) is not seen.
func TestEveryAppErrCodeIsDeclared(t *testing.T) {
	codes, problems := scanAppErrCodes(t)
	for _, p := range problems {
		t.Error(p)
	}
	if len(codes) < 10 {
		t.Fatalf("found only %d apperr codes; the scan no longer sees the calls", len(codes))
	}
	for _, c := range codes {
		if c.field {
			if _, ok := fieldReason(c.code); !ok {
				t.Errorf("%s: field code %q is not an app.common.v1.FieldViolationReason value", c.pos, c.code)
			}
		} else if _, ok := errorReason(c.code); !ok {
			t.Errorf("%s: code %q is not an app.common.v1.ErrorReason value", c.pos, c.code)
		}
	}
}

// goPackage is a package of the backend module as go list reports it.
type goPackage struct {
	Dir, ImportPath string
	GoFiles         []string
	Module          struct{ Path string }
}

// scanAppErrCodes parses the non-test files of every package of the backend module (go list, so test data and the
// separate tools module are left out) except the generated code, and returns the codes passed to apperr. A code it
// cannot check (not a string literal or a package-level string constant, or a literal without keys) is a problem.
func scanAppErrCodes(t *testing.T) ([]appCode, []string) {
	t.Helper()
	pkgs := listModulePackages(t)
	fset := token.NewFileSet()
	files := map[string][]*ast.File{}
	for _, p := range pkgs {
		if p.ImportPath == p.Module.Path+"/internal/gen" || strings.HasPrefix(p.ImportPath, p.Module.Path+"/internal/gen/") {
			continue
		}
		for _, name := range p.GoFiles {
			// object resolution on (mode 0): resolve needs ident.Obj to tell a parameter from a constant
			f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files[p.ImportPath] = append(files[p.ImportPath], f)
		}
	}
	apperrPkg := pkgs[0].Module.Path + "/internal/platform/apperr"
	consts := stringConsts(files)
	var codes []appCode
	var problems []string
	for pkg, pkgFiles := range files {
		for _, f := range pkgFiles {
			s := &codeScanner{fset: fset, pkg: pkg, apperr: apperrPkg, consts: consts, imports: importNames(f)}
			ast.Inspect(f, s.visit)
			codes = append(codes, s.codes...)
			problems = append(problems, s.problems...)
		}
	}
	return codes, problems
}

func listModulePackages(t *testing.T) []goPackage {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json=Dir,ImportPath,GoFiles,Module", "./...")
	cmd.Dir = filepath.Join("..", "..", "..")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []goPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p goPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list found no packages")
	}
	return pkgs
}

// stringConsts maps "<import path>.<name>" to the value of every package-level string constant.
func stringConsts(files map[string][]*ast.File) map[string]string {
	consts := map[string]string{}
	for pkg, pkgFiles := range files {
		for _, f := range pkgFiles {
			for _, decl := range f.Decls {
				if g, ok := decl.(*ast.GenDecl); ok && g.Tok == token.CONST {
					addConsts(consts, pkg, g)
				}
			}
		}
	}
	return consts
}

func addConsts(consts map[string]string, pkg string, g *ast.GenDecl) {
	for _, spec := range g.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if i >= len(vs.Values) {
				continue
			}
			if v, ok := stringLit(vs.Values[i]); ok {
				consts[pkg+"."+name.Name] = v
			}
		}
	}
}

// importNames maps each import's local name to its path.
func importNames(f *ast.File) map[string]string {
	imports := map[string]string{}
	for _, im := range f.Imports {
		ip, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			continue
		}
		name := ip[strings.LastIndex(ip, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		imports[name] = ip
	}
	return imports
}

// codeScanner collects the apperr codes of one file.
type codeScanner struct {
	fset     *token.FileSet
	pkg      string
	apperr   string
	consts   map[string]string
	imports  map[string]string
	codes    []appCode
	problems []string
}

func (s *codeScanner) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.CallExpr:
		if len(n.Args) == 3 && s.isApperr(n.Fun, "New") {
			s.add(n, s.isApperr(n.Args[0], "Validation"), n.Args[1])
		} else if len(n.Args) == 3 && s.isApperr(n.Fun, "Invalid") {
			s.add(n, true, n.Args[1])
		}
	case *ast.CompositeLit:
		if s.isApperr(n.Type, "Error") {
			s.literal(n)
		}
	}
	return true
}

// literal checks an apperr.Error literal: its Code, classified by its Kind as Convert does.
func (s *codeScanner) literal(n *ast.CompositeLit) {
	var code ast.Expr
	field := false
	for _, el := range n.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			s.problem(n, "an apperr.Error literal without keys cannot be checked")
			return
		}
		if k, ok := kv.Key.(*ast.Ident); ok {
			switch k.Name {
			case "Code":
				code = kv.Value
			case "Kind":
				field = s.isApperr(kv.Value, "Validation")
			}
		}
	}
	if code != nil {
		s.add(n, field, code)
	} else if field {
		s.codes = append(s.codes, appCode{pos: s.fset.Position(n.Pos()).String(), field: true})
	}
}

func (s *codeScanner) isApperr(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && s.imports[x.Name] == s.apperr
}

func (s *codeScanner) resolve(e ast.Expr) (string, bool) {
	if v, ok := stringLit(e); ok {
		return v, true
	}
	switch e := e.(type) {
	case *ast.Ident:
		if e.Obj != nil && e.Obj.Kind != ast.Con { // a parameter or variable, even one named like a constant
			return "", false
		}
		v, ok := s.consts[s.pkg+"."+e.Name]
		return v, ok
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			v, ok := s.consts[s.imports[x.Name]+"."+e.Sel.Name]
			return v, ok
		}
	}
	return "", false
}

// add records the code e; an empty ErrorInfo code is skipped (Convert sends no ErrorInfo), an empty field code is
// checked like any other (Convert sends INVALID and logs it).
func (s *codeScanner) add(at ast.Node, field bool, e ast.Expr) {
	code, ok := s.resolve(e)
	switch {
	case !ok:
		s.problem(at, "the apperr code is not a string literal or constant, so it cannot be checked")
	case code != "" || field:
		s.codes = append(s.codes, appCode{pos: s.fset.Position(at.Pos()).String(), field: field, code: code})
	}
}

func (s *codeScanner) problem(at ast.Node, msg string) {
	s.problems = append(s.problems, s.fset.Position(at.Pos()).String()+": "+msg)
}

func stringLit(e ast.Expr) (string, bool) {
	b, ok := e.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(b.Value)
	return s, err == nil
}
