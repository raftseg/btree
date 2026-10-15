// Copyright 2026 The raftseg authors. Licensed under the Apache License,
// Version 2.0.

// Package depcheck holds a test that keeps the module free of third-party
// dependencies: go.mod requires nothing, and no Go file imports anything
// outside the standard library and this module.
package depcheck

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/raftseg/btree"

// minFiles guards against the walk silently finding nothing.
const minFiles = 4

func moduleRoot(t *testing.T) string {
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// go.mod must declare the module and a Go version, and nothing else: no
// require, replace, exclude, retract, tool or toolchain lines.
func TestGoModHasNoRequirements(t *testing.T) {
	f, err := os.Open(filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		word := strings.Fields(line)[0]
		switch word {
		case "module":
			if line != "module "+modulePath {
				t.Errorf("go.mod:%d: %q", n, line)
			}
		case "go":
		default:
			t.Errorf("go.mod:%d: %q: the module must not depend on anything", n, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moduleRoot(t), "go.sum")); err == nil {
		t.Error("go.sum exists: the module must not depend on anything")
	}
}

// Every Go file, tests included and whatever its build tags, imports only
// the standard library and this module.
func TestImportsStandardLibraryOnly(t *testing.T) {
	root := moduleRoot(t)
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Go ignores testdata, and directories starting with . or _.
			if path != root && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_")) {
				return filepath.SkipDir
			}
			// A nested module is not part of this one.
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		files++
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if !allowed(p) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s imports %q", rel, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < minFiles {
		t.Fatalf("only %d Go files found under %s", files, root)
	}
	t.Logf("%d Go files import only the standard library and %s", files, modulePath)
}

// allowed reports whether an import path is in the standard library or this
// module. Standard library paths have no dot in their first element.
func allowed(p string) bool {
	if p == modulePath || strings.HasPrefix(p, modulePath+"/") {
		return true
	}
	first, _, _ := strings.Cut(p, "/")
	return !strings.Contains(first, ".")
}
