// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Tripwire test added by vuln-hunt campaign 2026-05-19-codex /
// callctx-openfile.

package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestVulnHuntSubsystemCallctxOpenfile_NoDirectPathFilesystemCallsInBuiltins
// pins the core filesystem invariant for command implementation files:
// production builtins must reach the filesystem through CallContext methods
// rather than direct path-taking os/syscall/unix helpers. Documented kernel
// pseudo-file readers live under builtins/internal/ and are covered by the
// internal allowlist instead of this tripwire.
func TestVulnHuntSubsystemCallctxOpenfile_NoDirectPathFilesystemCallsInBuiltins(t *testing.T) {
	root := repoRoot(t)
	builtinsRoot := filepath.Join(root, "builtins")

	banned := map[string]map[string]bool{
		"os": {
			"Open": true, "OpenFile": true, "ReadFile": true, "ReadDir": true,
			"Stat": true, "Lstat": true, "Readlink": true,
			"Create": true, "CreateTemp": true, "WriteFile": true,
			"Mkdir": true, "MkdirAll": true, "Remove": true, "RemoveAll": true,
			"Rename": true, "Symlink": true, "Link": true, "Truncate": true,
			"Chmod": true, "Chown": true,
		},
		"syscall": {
			"Open": true, "Openat": true, "Stat": true, "Lstat": true,
			"Fstatat": true, "Readlink": true, "Unlink": true,
			"Mkdir": true, "Rmdir": true, "Rename": true, "Symlink": true,
			"Link": true, "Chmod": true, "Chown": true, "Truncate": true,
		},
		"golang.org/x/sys/unix": {
			"Open": true, "Openat": true, "Stat": true, "Lstat": true,
			"Fstatat": true, "Readlink": true, "Unlink": true,
			"Mkdir": true, "Rmdir": true, "Rename": true, "Symlink": true,
			"Link": true, "Chmod": true, "Chown": true, "Truncate": true,
		},
	}

	err := filepath.WalkDir(builtinsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "internal", "tests", "testutil":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}

		localToImport := map[string]string{}
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if _, ok := banned[importPath]; !ok {
				continue
			}
			local := filepath.Base(importPath)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			if local != "_" && local != "." {
				localToImport[local] = importPath
			}
		}

		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			importPath, ok := localToImport[id.Name]
			if !ok {
				return true
			}
			if banned[importPath][sel.Sel.Name] {
				pos := fset.Position(sel.Pos())
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d calls %s.%s directly; production builtins must use CallContext filesystem methods unless the documented exception lives under builtins/internal/",
					rel, pos.Line, id.Name, sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk builtins: %v", err)
	}
}
