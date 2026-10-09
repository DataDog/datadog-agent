// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

// valRef is an expression a variable's value was computed from, together with
// the scope its identifiers must be resolved in (the caller's scope for
// parameters, the declaring package for package-level values).
type valRef struct {
	x  ast.Expr
	sc *scope
}

const (
	maxValuesPerNode = 12
	maxValuesPerTest = 500 // per test, including the tests of the suites it runs
	maxValueLen      = 100
)

// literals returns, best effort, the string literals that flow into exprs:
// literals written in the expressions themselves, and those reachable through
// variable assignments, range loops, parameter bindings (across helper calls)
// and package-level constants, in this package or an imported one. These are
// the concrete values a check is about: metric names, config keys, commands,
// expected output.
func (e *extractor) literals(sc *scope, exprs []ast.Expr) []string {
	c := &litCollector{e: e, seen: map[string]bool{}, visited: map[any]bool{}}
	for _, x := range exprs {
		c.walk(sc, x, 0)
	}
	return c.out
}

type litCollector struct {
	e       *extractor
	out     []string
	seen    map[string]bool
	visited map[any]bool
}

func (c *litCollector) add(s string) {
	s = strings.TrimSpace(s)
	if !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return // "", "%s", "\n"...
	}
	s = truncate(s, maxValueLen)
	if c.seen[s] || len(c.out) >= maxValuesPerNode {
		return
	}
	c.seen[s] = true
	c.out = append(c.out, s)
}

func (c *litCollector) walk(sc *scope, x ast.Node, depth int) {
	if x == nil || sc == nil || depth > 10 {
		return
	}
	ast.Inspect(x, func(n ast.Node) bool {
		if len(c.out) >= maxValuesPerNode {
			return false
		}
		switch t := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BasicLit:
			if t.Kind == token.STRING {
				if s, err := strconv.Unquote(t.Value); err == nil {
					c.add(s)
				}
			}
		case *ast.CompositeLit:
			for _, elt := range t.Elts {
				c.walk(sc, elt, depth)
			}
			return false
		case *ast.KeyValueExpr:
			if !c.isFieldKey(sc, t.Key) {
				c.walk(sc, t.Key, depth)
			}
			c.walk(sc, t.Value, depth)
			return false
		case *ast.SelectorExpr:
			if id, ok := t.X.(*ast.Ident); ok && sc.vars[id.Name] == nil {
				if path := sc.file.imports[id.Name]; path != "" {
					if dir := c.e.l.importDir(path); dir != "" {
						c.pkgValue(c.e.l.load(dir, false), t.Sel.Name, depth)
					}
					return false
				}
			}
			// x.F: only the values of field F, when x comes from a struct literal
			c.project(sc, t.X, []string{t.Sel.Name}, depth)
			return false
		case *ast.Ident:
			if v := sc.vars[t.Name]; v != nil {
				if !c.visited[v] {
					c.visited[v] = true
					for _, r := range v.vals {
						c.walk(r.sc, r.x, depth+1)
					}
				}
				return false
			}
			c.pkgValue(sc.file.pkg, t.Name, depth) // the loaded package, including its test files
		}
		return true
	})
}

func (c *litCollector) isFieldKey(sc *scope, key ast.Expr) bool { return isFieldKey(sc, key) }

// isFieldKey reports whether a composite literal key is a struct field name
// (rather than a map key referring to a variable or constant).
func isFieldKey(sc *scope, key ast.Expr) bool {
	id, ok := key.(*ast.Ident)
	if !ok {
		return false
	}
	if sc.vars[id.Name] != nil {
		return false
	}
	_, isValue := sc.file.pkg.values[id.Name]
	return !isValue
}

// project collects the literals of x.path[0].path[1]...: it follows x to the
// struct literals it was built from and keeps only the selected fields. Slices
// and maps of structs are traversed element-wise (x is then a range variable
// or an index expression). When x cannot be traced to a literal (e.g. it is
// the result of a call), the whole expression is used instead.
func (c *litCollector) project(sc *scope, x ast.Expr, path []string, depth int) {
	if len(path) == 0 {
		c.walk(sc, x, depth)
		return
	}
	if x == nil || sc == nil || depth > 10 || len(c.out) >= maxValuesPerNode {
		return
	}
	switch t := x.(type) {
	case *ast.ParenExpr:
		c.project(sc, t.X, path, depth)
	case *ast.StarExpr:
		c.project(sc, t.X, path, depth)
	case *ast.UnaryExpr:
		c.project(sc, t.X, path, depth)
	case *ast.IndexExpr:
		c.project(sc, t.X, path, depth)
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok && sc.vars[id.Name] == nil && sc.file.imports[id.Name] != "" {
			return // field of a package-level value of another package: not traced
		}
		c.project(sc, t.X, append([]string{t.Sel.Name}, path...), depth)
	case *ast.Ident:
		if v := sc.vars[t.Name]; v != nil {
			key := [2]any{v, strings.Join(path, ".")}
			if c.visited[key] {
				return
			}
			c.visited[key] = true
			for _, r := range v.vals {
				c.project(r.sc, r.x, path, depth+1)
			}
			return
		}
		pkg := sc.file.pkg
		if v, ok := pkg.values[t.Name]; ok && v != nil && pkg.valueFiles[t.Name] != nil {
			c.project(&scope{file: pkg.valueFiles[t.Name], vars: map[string]*varInfo{}}, v, path, depth+1)
		}
	case *ast.CompositeLit:
		keyed := false
		for _, elt := range t.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok || !c.isFieldKey(sc, kv.Key) {
				continue
			}
			keyed = true
			if kv.Key.(*ast.Ident).Name == path[0] {
				c.project(sc, kv.Value, path[1:], depth)
			}
		}
		if keyed {
			return // a struct literal: the field is either set above or not at all
		}
		for _, elt := range t.Elts { // slice or map of structs
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			c.project(sc, elt, path, depth)
		}
	case *ast.CallExpr:
		c.walk(sc, t, depth)
	}
}

func (c *litCollector) pkgValue(pkg *pkgInfo, name string, depth int) {
	key := pkg.dir + "." + name
	if c.visited[key] {
		return
	}
	c.visited[key] = true
	if x, ok := pkg.values[name]; ok && x != nil && pkg.valueFiles[name] != nil {
		c.walk(&scope{file: pkg.valueFiles[name], vars: map[string]*varInfo{}}, x, depth+1)
	}
}

// aggregateValues sets, on test nodes, Values to the union of the values
// checked anywhere below them and AssertionCount to the number of assertions
// (including collapsed repeats). Both include the tests of the suites a test
// runs, so a top-level test describes everything that runs under it.
func aggregateValues(n *Node) {
	for _, c := range n.Children {
		aggregateValues(c)
	}
	if n.Kind != "test" {
		return
	}
	n.AssertionCount = countAssertions(n.Children)
	seen := map[string]bool{}
	var vals []string
	var walk func(*Node)
	walk = func(m *Node) {
		var own []string
		if m.Assertion != nil {
			own = m.Assertion.Values
		} else if m != n {
			own = m.Values
		}
		for _, v := range own {
			if !seen[v] && len(vals) < maxValuesPerTest {
				seen[v] = true
				vals = append(vals, v)
			}
		}
		if m != n && m.Kind == "test" {
			return // already aggregated
		}
		for _, c := range m.Children {
			walk(c)
		}
	}
	walk(n)
	n.Values = vals
}

// ---------- repeated helper expansions ----------

// expansionKey identifies a helper expansion: two calls to the same helper
// perform the same checks, except for the function literals they pass, which
// are still expanded under the collapsed call.
type expansionKey struct {
	decl ast.Node
}

// expansion records where a helper was first expanded and how many assertions it had.
type expansion struct {
	pos        string
	assertions int
}

func (e *extractor) expansionKeyOf(c callee, _ *ast.CallExpr) expansionKey {
	if c.fi != nil {
		return expansionKey{decl: c.fi.decl}
	}
	return expansionKey{decl: c.lit}
}

// countAssertions counts assertions in nodes, including those hidden in
// collapsed repeated helper calls.
func countAssertions(nodes []*Node) int {
	n := 0
	for _, c := range nodes {
		if c.Kind == "assertion" || c.Kind == "eventually" {
			n++
		}
		n += c.Repeat + countAssertions(c.Children)
	}
	return n
}
