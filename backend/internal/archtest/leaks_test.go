package archtest

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Every package with tests fails when a goroutine its tests started is still running at the end: its TestMain calls
// goleak.VerifyTestMain, or dbtest.Main, which runs goleak after stopping the database.
func TestEveryTestPackageChecksLeaks(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json=ImportPath,Dir,TestGoFiles,XTestGoFiles", "./...")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p struct {
			ImportPath, Dir           string
			TestGoFiles, XTestGoFiles []string
		}
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		var files []string
		for _, f := range append(p.TestGoFiles, p.XTestGoFiles...) {
			files = append(files, filepath.Join(p.Dir, f))
		}
		if len(files) > 0 && !checksLeaks(t, files) {
			t.Errorf("%s: add func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }, or use dbtest.Main", p.ImportPath)
		}
	}
}

// checksLeaks reports whether one of files has a TestMain that calls goleak.VerifyTestMain or a Main of dbtest.
func checksLeaks(t *testing.T, files []string) bool {
	t.Helper()
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "TestMain" && callsLeakCheck(f.Name.Name, fn) {
				return true
			}
		}
	}
	return false
}

// callsLeakCheck reports whether fn, declared in package pkg, calls goleak.VerifyTestMain or dbtest's Main.
func callsLeakCheck(pkg string, fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && isLeakCheck(pkg, call.Fun) {
			found = true
		}
		return !found
	})
	return found
}

func isLeakCheck(pkg string, fun ast.Expr) bool {
	switch fun := fun.(type) {
	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		return ok && (x.Name == "goleak" && fun.Sel.Name == "VerifyTestMain" || x.Name == "dbtest" && fun.Sel.Name == "Main")
	case *ast.Ident:
		return pkg == "dbtest" && fun.Name == "Main"
	}
	return false
}

func TestChecksLeaksRecognisesTestMain(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, tc := range []struct {
		name, src string
		want      bool
	}{
		{"goleak", "package p\nfunc TestMain(m *testing.M) { goleak.VerifyTestMain(m) }\n", true},
		{"dbtest", "package p\nfunc TestMain(m *testing.M) { dbtest.Main(m, &pg) }\n", true},
		{"dbtest itself", "package dbtest\nfunc TestMain(m *testing.M) { Main(m, &pg) }\n", true},
		{"no TestMain", "package p\nfunc TestX(t *testing.T) {}\n", false},
		{"TestMain without a check", "package p\nfunc TestMain(m *testing.M) { m.Run() }\n", false},
	} {
		if got := checksLeaks(t, []string{write(tc.name+"_test.go", tc.src)}); got != tc.want {
			t.Errorf("%s: checksLeaks = %t, want %t", tc.name, got, tc.want)
		}
	}
}
