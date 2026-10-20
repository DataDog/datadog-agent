// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dupcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obfuscateFunc is the function of pkg/trace/agent that obfuscates a span
// according to its type: the only place where trace processing branches on
// the span type.
const obfuscateFunc = "obfuscateSpanInternal"

// TestCorpusCoversObfuscatedSpanTypes checks that the corpus has spans of
// every type obfuscateSpanInternal branches on, so that the dupcheck report
// covers the code that obfuscates them.
func TestCorpusCoversObfuscatedSpanTypes(t *testing.T) {
	path := obfuscateGoPath()
	types, err := obfuscatedSpanTypes(path)
	require.NoError(t, err)
	// Guards against a parser that silently stops finding anything.
	require.Contains(t, types, "sql", "%s in %s must branch on span types", obfuscateFunc, path)
	for _, typ := range types {
		assert.Contains(t, CorpusSpanTypes, typ,
			"span type %q is handled by %s in pkg/trace/agent/obfuscate.go but missing from dupcheck.CorpusSpanTypes: add it so that the dupcheck report covers it",
			typ, obfuscateFunc)
	}
}

// obfuscateGoPath returns the path of pkg/trace/agent/obfuscate.go: in the
// Bazel runfiles, or in the source tree, relative to this package, outside
// Bazel. The rules_go runfiles library isn't used, so as not to add it to the
// go.mod of pkg/trace, which other repositories import.
func obfuscateGoPath() string {
	if rel := os.Getenv("DUPCHECK_OBFUSCATE_GO"); rel != "" {
		return filepath.Join(os.Getenv("TEST_SRCDIR"), filepath.FromSlash(rel))
	}
	return filepath.Join("..", "..", "agent", "obfuscate.go")
}

// obfuscatedSpanTypes returns the span types that obfuscateFunc branches on in
// the Go source file at path: the string labels of its switch statements on a
// Type() call, and the strings it compares a Type() call with. The Type()
// call is what tells span types apart from other Type fields, such as the
// types of attribute values or of stats groups.
func obfuscatedSpanTypes(path string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == obfuscateFunc {
			fn = d
		}
	}
	if fn == nil {
		return nil, fmt.Errorf("function %s not found in %s: update this test to where the agent obfuscates spans by type", obfuscateFunc, path)
	}
	types := make(map[string]bool)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SwitchStmt:
			if !isTypeCall(n.Tag) {
				return true
			}
			for _, stmt := range n.Body.List {
				for _, e := range stmt.(*ast.CaseClause).List {
					if s, ok := stringLiteral(e); ok {
						types[s] = true
					}
				}
			}
		case *ast.BinaryExpr:
			if n.Op != token.EQL && n.Op != token.NEQ {
				return true
			}
			for _, pair := range [][2]ast.Expr{{n.X, n.Y}, {n.Y, n.X}} {
				if s, ok := stringLiteral(pair[1]); ok && isTypeCall(pair[0]) {
					types[s] = true
				}
			}
		}
		return true
	})
	return slices.Sorted(maps.Keys(types)), nil
}

// isTypeCall reports whether e is a call x.Type().
func isTypeCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Type"
}

func stringLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func TestObfuscatedSpanTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "obfuscate.go")
	require.NoError(t, os.WriteFile(path, []byte(`package agent

const constType = "const-type"

func (a *Agent) obfuscateSpanInternal(span obfuscateSpan) {
	switch span.Type() {
	case "sql", "cassandra":
	case constType:
	case "redis":
		if span.Type() == "redis-nested" {
		}
		if "reversed" != span.Type() {
		}
	}
	switch span.Resource() {
	case "not-a-type":
	}
	for _, v := range span.Attributes() {
		switch v.Type {
		case "attribute-value-type":
		}
	}
}

func (a *Agent) obfuscateStatsGroup(b *pb.ClientGroupedStats) {
	switch b.Type() {
	case "other-function":
	}
}
`), 0o644))

	types, err := obfuscatedSpanTypes(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"cassandra", "redis", "redis-nested", "reversed", "sql"}, types)

	require.NoError(t, os.WriteFile(path, []byte("package agent\n\nfunc renamed() {}\n"), 0o644))
	_, err = obfuscatedSpanTypes(path)
	assert.ErrorContains(t, err, "function obfuscateSpanInternal not found")
}
