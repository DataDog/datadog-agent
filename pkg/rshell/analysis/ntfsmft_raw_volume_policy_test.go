// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// TestNTFSMFTRawVolumeCapabilitiesArePinned prevents an unreviewed expansion
// of ntfs-du's Windows filesystem capabilities. It parses Windows-only source
// so the rule runs in Linux CI too. Any new CreateFile or DeviceIoControl call
// in either ntfs package must be added here and receive the analysis review
// gate.
func TestNTFSMFTRawVolumeCapabilitiesArePinned(t *testing.T) {
	type createFilePolicy struct {
		access string
		flags  string
	}

	createFiles := map[string]createFilePolicy{
		"builtins/internal/ntfsmft/du_windows.go:openVolume": {
			access: "windows.GENERIC_READ",
			flags:  "0",
		},
		"builtins/internal/ntfsmft/du_windows.go:resolvePathLocation": {
			access: "0",
			flags:  "windows.FILE_FLAG_BACKUP_SEMANTICS | windows.FILE_FLAG_OPEN_REPARSE_POINT",
		},
		"builtins/internal/ntfsmft/topn.go:resolveCandidatePaths": {
			access: "0",
			flags:  "windows.FILE_FLAG_BACKUP_SEMANTICS",
		},
	}
	deviceIOControls := map[string]string{
		"builtins/internal/ntfsmft/du_windows.go:openVolume": "fsctlGetNTFSVolumeData",
	}

	root := repoRoot(t)
	var files []string
	for _, dir := range []string{
		filepath.Join(root, "builtins", "internal", "ntfsmft"),
		filepath.Join(root, "builtins", "ntfsdu"),
	} {
		packageFiles, err := collectGoFilesRecursive(dir, map[string]bool{}, nil)
		if err != nil {
			t.Fatalf("collect production Go files in %s: %v", dir, err)
		}
		files = append(files, packageFiles...)
	}
	seenCreateFile := make(map[string]int, len(createFiles))
	seenDeviceIOControl := make(map[string]int, len(deviceIOControls))
	seenFSCTLConstant := false
	fset := token.NewFileSet()

	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("relative path for %s: %v", path, err)
		}
		rel = filepath.ToSlash(rel)

		if rel == "builtins/internal/ntfsmft/du_windows.go" {
			seenFSCTLConstant = pinFSCTLGetNTFSVolumeData(t, file)
		}
		windowsAliases := windowsImportAliases(t, rel, file)
		for _, violation := range indirectWindowsCapabilityUses(fset, file, windowsAliases) {
			t.Errorf("%s: %s", rel, violation)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := rel + ":" + fn.Name.Name
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !windowsCall(call, windowsAliases, "CreateFile") && !windowsCall(call, windowsAliases, "DeviceIoControl") {
					return true
				}
				if windowsCall(call, windowsAliases, "CreateFile") {
					policy, allowed := createFiles[key]
					if !allowed {
						t.Errorf("%s:%d: unpinned windows.CreateFile in %s", rel, fset.Position(call.Pos()).Line, fn.Name.Name)
						return true
					}
					seenCreateFile[key]++
					if len(call.Args) != 7 {
						t.Errorf("%s:%d: windows.CreateFile has %d arguments, want 7", rel, fset.Position(call.Pos()).Line, len(call.Args))
						return true
					}
					if got := formatExpr(t, fset, call.Args[1]); got != policy.access {
						t.Errorf("%s:%d: CreateFile access = %s, want %s", rel, fset.Position(call.Pos()).Line, got, policy.access)
					}
					if got := formatExpr(t, fset, call.Args[5]); got != policy.flags {
						t.Errorf("%s:%d: CreateFile flags = %s, want %s", rel, fset.Position(call.Pos()).Line, got, policy.flags)
					}
				}
				if windowsCall(call, windowsAliases, "DeviceIoControl") {
					controlCode, allowed := deviceIOControls[key]
					if !allowed {
						t.Errorf("%s:%d: unpinned windows.DeviceIoControl in %s", rel, fset.Position(call.Pos()).Line, fn.Name.Name)
						return true
					}
					seenDeviceIOControl[key]++
					if len(call.Args) < 2 {
						t.Errorf("%s:%d: windows.DeviceIoControl has no control-code argument", rel, fset.Position(call.Pos()).Line)
						return true
					}
					if got := formatExpr(t, fset, call.Args[1]); got != controlCode {
						t.Errorf("%s:%d: DeviceIoControl code = %s, want %s", rel, fset.Position(call.Pos()).Line, got, controlCode)
					}
				}
				return true
			})
		}
	}

	for key := range createFiles {
		if got := seenCreateFile[key]; got != 1 {
			t.Errorf("%s: windows.CreateFile calls = %d, want 1", key, got)
		}
	}
	for key := range deviceIOControls {
		if got := seenDeviceIOControl[key]; got != 1 {
			t.Errorf("%s: windows.DeviceIoControl calls = %d, want 1", key, got)
		}
	}
	if !seenFSCTLConstant {
		t.Error("fsctlGetNTFSVolumeData = 0x00090064 is not pinned")
	}
}

// indirectWindowsCapabilityUses rejects taking CreateFile or DeviceIoControl
// as function values. The symbol allowlist permits those selectors, so an
// indirect call would otherwise evade the call-site policy below.
func indirectWindowsCapabilityUses(fset *token.FileSet, file *ast.File, aliases map[string]bool) []string {
	var violations []string
	var ancestors []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			ancestors = ancestors[:len(ancestors)-1]
			return false
		}
		if sel, ok := node.(*ast.SelectorExpr); ok && (windowsSelector(sel, aliases, "CreateFile") || windowsSelector(sel, aliases, "DeviceIoControl")) {
			parent, directCall := lastNode(ancestors).(*ast.CallExpr)
			if !directCall || parent.Fun != sel {
				violations = append(violations, fmt.Sprintf("%s at line %d must be called directly, not used as a function value", sel.Sel.Name, fset.Position(sel.Pos()).Line))
			}
		}
		ancestors = append(ancestors, node)
		return true
	})
	return violations
}

func TestNTFSMFTRawVolumePolicyRejectsFunctionValues(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package ntfsmft
func f() {
	create := windows.CreateFile
	ioctl := windows.DeviceIoControl
	_, _ = create, ioctl
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := indirectWindowsCapabilityUses(fset, file, map[string]bool{"windows": true}); len(got) != 2 {
		t.Fatalf("indirect privileged Windows calls = %v, want two violations", got)
	}
}

func pinFSCTLGetNTFSVolumeData(t *testing.T, file *ast.File) bool {
	if file == nil {
		t.Fatal("nil AST file")
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range values.Names {
				if name.Name != "fsctlGetNTFSVolumeData" {
					continue
				}
				if i >= len(values.Values) {
					t.Errorf("fsctlGetNTFSVolumeData has no value")
					return false
				}
				if got := formatExpr(t, token.NewFileSet(), values.Values[i]); got != "0x00090064" {
					t.Errorf("fsctlGetNTFSVolumeData = %s, want 0x00090064 (FSCTL_GET_NTFS_VOLUME_DATA)", got)
					return false
				}
				return true
			}
		}
	}
	return false
}

func windowsImportAliases(t *testing.T, rel string, file *ast.File) map[string]bool {
	t.Helper()
	aliases := make(map[string]bool)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "golang.org/x/sys/windows" {
			continue
		}
		if imp.Name != nil {
			switch imp.Name.Name {
			case ".":
				t.Errorf("%s: dot-importing golang.org/x/sys/windows is forbidden; capability calls must remain pinnable", rel)
			case "_":
				// A blank import cannot make a capability call.
			default:
				aliases[imp.Name.Name] = true
			}
			continue
		}
		aliases["windows"] = true
	}
	return aliases
}

func windowsCall(call *ast.CallExpr, aliases map[string]bool, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && windowsSelector(sel, aliases, name)
}

func windowsSelector(sel *ast.SelectorExpr, aliases map[string]bool, name string) bool {
	if sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && aliases[ident.Name]
}

func lastNode(nodes []ast.Node) ast.Node {
	if len(nodes) == 0 {
		return nil
	}
	return nodes[len(nodes)-1]
}

func formatExpr(t *testing.T, fset *token.FileSet, expr ast.Expr) string {
	t.Helper()
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		t.Fatalf("format expression: %v", err)
	}
	return buf.String()
}
