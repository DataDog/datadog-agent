// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Node is an element of the extracted assertion tree.
type Node struct {
	// Kind is one of: test, suite, hook, subtest, helper, closure, predicate, callback, if,
	// else, loop, switch, case, defer, eventually, assertion, implicit, skip,
	// flaky, opaque.
	Kind      string     `json:"kind"`
	Label     string     `json:"label,omitempty"`
	Pos       string     `json:"pos,omitempty"`
	Def       string     `json:"def,omitempty"`
	Assertion *Assertion `json:"assertion,omitempty"`
	// Values lists string literals flowing into the node: for a test, every
	// value checked below it; for helper/implicit/opaque nodes, their arguments.
	Values []string `json:"values,omitempty"`
	// Ref is set on a helper call whose checks were already expanded at the
	// given position in the same tree; Repeat is how many assertions that hides.
	Ref    string `json:"ref,omitempty"`
	Repeat int    `json:"repeatedAssertions,omitempty"`
	// AssertionCount is set on tests: assertions below them, including repeats
	// collapsed into Ref nodes and the tests of the suites they run.
	AssertionCount int     `json:"assertionCount,omitempty"`
	Children       []*Node `json:"children,omitempty"`
}

// Assertion describes a single check.
type Assertion struct {
	Library string   `json:"library"` // assert, require, suite, suite.Require(), t, condition...
	Func    string   `json:"func"`
	Fatal   bool     `json:"fatal"` // stops the test (or the current polling attempt) on failure
	Summary string   `json:"summary"`
	Args    []string `json:"args,omitempty"`
	Message string   `json:"message,omitempty"`
	Call    string   `json:"call"`
	Origins []Origin `json:"origins,omitempty"`
	// Values are the string literals the assertion is about, resolved through
	// variables, helper parameters and constants (see literals).
	Values []string `json:"values,omitempty"`
}

// Origin explains where a variable used in an assertion comes from.
type Origin struct {
	Name string `json:"name"`
	From string `json:"from"`
}

type varKind int

const (
	kindNone varKind = iota
	kindTestingT
	kindCollectT
	kindAssertObj
	kindRequireObj
)

type varInfo struct {
	kind    varKind
	typ     *typeRef
	origin  string
	rhs     ast.Expr // expression the origin was computed from (for chaining)
	closure *ast.FuncLit
	capture *scope   // scope captured by closure
	fnRefs  []callee // function value (helper or method value) bound to this variable
	vals    []valRef // expressions the value comes from, for literal extraction
}

type scope struct {
	file     *fileInfo
	vars     map[string]*varInfo
	condFunc bool // inside a func() bool given to Eventually/Never/Condition
	errFunc  bool // condFunc variant for func() error: non-nil returns fail the attempt
	depth    int
}

func (s *scope) child() *scope {
	vars := make(map[string]*varInfo, len(s.vars))
	for k, v := range s.vars {
		vars[k] = v
	}
	return &scope{file: s.file, vars: vars, depth: s.depth}
}

type followMode string

const (
	followModule  followMode = "module"
	followRepo    followMode = "repo"
	followPackage followMode = "package"
)

type extractor struct {
	l        *loader
	module   string
	pkgDir   string
	follow   followMode
	maxDepth int
	argWidth int
	active   map[ast.Node]bool
	// calls whose result is discarded (statement-level), used to limit [?] false positives
	stmtCalls map[*ast.CallExpr]bool
	// set by pollingNode: the next condition function polls a func() error
	errCond bool
	// walking an if condition (to recognize predicate callbacks)
	inCond bool
	// helper expansions already printed in the current top-level test
	expanded  map[expansionKey]*expansion
	testDepth int
	retActive map[ast.Node]bool
}

func newExtractor(l *loader, pkg *pkgInfo, follow followMode, maxDepth int) *extractor {
	return &extractor{l: l, module: pkg.module, pkgDir: pkg.dir, follow: follow, maxDepth: maxDepth, argWidth: 90, active: map[ast.Node]bool{}, stmtCalls: map[*ast.CallExpr]bool{},
		expanded: map[expansionKey]*expansion{}, retActive: map[ast.Node]bool{}}
}

// ---------- entry points ----------

func (e *extractor) testNode(fi *funcInfo) *Node {
	label := fi.name
	if fi.recv != "" {
		label = fmt.Sprintf("(%s).%s", fi.recv, fi.name)
	}
	n := &Node{Kind: "test", Label: label, Pos: e.relPos(fi.decl.Pos())}
	if e.active[fi.decl] {
		return n
	}
	e.active[fi.decl] = true
	defer delete(e.active, fi.decl)
	if e.testDepth == 0 {
		e.expanded = map[expansionKey]*expansion{}
	}
	e.testDepth++
	defer func() { e.testDepth-- }()
	sc := e.funcScope(nil, fi, nil, 0)
	n.Children = e.walkStmts(sc, fi.decl.Body.List)
	if e.testDepth == 1 {
		aggregateValues(n)
	}
	return n
}

// ---------- statements ----------

func (e *extractor) walkStmts(sc *scope, list []ast.Stmt) []*Node {
	var out []*Node
	for _, st := range list {
		out = append(out, e.walkStmt(sc, st)...)
	}
	return out
}

func (e *extractor) walkStmt(sc *scope, st ast.Stmt) []*Node {
	if st == nil {
		return nil
	}
	switch s := st.(type) {
	case *ast.BlockStmt:
		return e.walkStmts(sc, s.List)
	case *ast.LabeledStmt:
		return e.walkStmt(sc, s.Stmt)
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			e.stmtCalls[call] = true
		}
		return e.scanExpr(sc, s.X)
	case *ast.SendStmt:
		return e.scanExpr(sc, s.Value)
	case *ast.GoStmt:
		return e.scanExpr(sc, s.Call)
	case *ast.DeferStmt:
		if nodes := e.scanExpr(sc, s.Call); len(nodes) > 0 {
			return []*Node{{Kind: "defer", Label: "deferred", Pos: e.pos(s.Pos()), Children: nodes}}
		}
	case *ast.AssignStmt:
		var out []*Node
		for i, r := range s.Rhs {
			if fl, ok := r.(*ast.FuncLit); ok && len(s.Lhs) == len(s.Rhs) {
				if id, ok := s.Lhs[i].(*ast.Ident); ok {
					// closures are expanded where they are called
					sc.vars[id.Name] = &varInfo{closure: fl, capture: sc, origin: "← closure"}
					continue
				}
			}
			out = append(out, e.scanExpr(sc, r)...)
		}
		if s.Tok == token.ASSIGN || s.Tok == token.DEFINE {
			e.recordAssign(sc, s.Lhs, s.Rhs)
		}
		return out
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return nil
		}
		var out []*Node
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for _, v := range vs.Values {
				out = append(out, e.scanExpr(sc, v)...)
			}
			lhs := make([]ast.Expr, len(vs.Names))
			for i, n := range vs.Names {
				lhs[i] = n
			}
			if len(vs.Values) > 0 {
				e.recordAssign(sc, lhs, vs.Values)
			} else {
				kind, typ := e.classifyType(sc.file, vs.Type)
				for _, n := range vs.Names {
					sc.vars[n.Name] = &varInfo{kind: kind, typ: typ}
				}
			}
		}
		return out
	case *ast.IfStmt:
		out := e.walkStmt(sc, s.Init)
		prevCond := e.inCond
		e.inCond = true
		out = append(out, e.scanExpr(sc, s.Cond)...)
		e.inCond = prevCond
		ifLabel, elseLabel := "if "+e.render(s.Cond, 110), "else (not: "+e.render(s.Cond, 100)+")"
		if passed, ok := e.assertionCond(sc, s.Cond); ok {
			ifLabel, elseLabel = "if the assertion above passed", "else (the assertion above failed)"
			if !passed {
				ifLabel, elseLabel = "if the assertion above failed", "else (the assertion above passed)"
			}
		}
		if body := e.walkStmts(sc, s.Body.List); len(body) > 0 {
			out = append(out, &Node{Kind: "if", Label: ifLabel, Pos: e.pos(s.Pos()), Children: body})
		}
		if s.Else != nil {
			if els := e.walkStmt(sc, s.Else); len(els) > 0 {
				out = append(out, &Node{Kind: "else", Label: elseLabel, Pos: e.pos(s.Else.Pos()), Children: els})
			}
		}
		return out
	case *ast.ForStmt:
		out := e.walkStmt(sc, s.Init)
		out = append(out, e.scanExpr(sc, s.Cond)...)
		body := e.walkStmts(sc, s.Body.List)
		body = append(body, e.walkStmt(sc, s.Post)...)
		if len(body) > 0 {
			label := "loop"
			if s.Cond != nil {
				label = "loop while " + e.render(s.Cond, 100)
			}
			out = append(out, &Node{Kind: "loop", Label: label, Pos: e.pos(s.Pos()), Children: body})
		}
		return out
	case *ast.RangeStmt:
		out := e.scanExpr(sc, s.X)
		var names []string
		for i, x := range []ast.Expr{s.Key, s.Value} {
			id, ok := x.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			names = append(names, id.Name)
			what := "element of "
			if i == 0 && s.Value != nil {
				what = "key/index of "
			} else if i == 0 {
				what = "key/index/element of "
			}
			sc.vars[id.Name] = &varInfo{origin: what + e.render(s.X, 100), rhs: s.X, vals: []valRef{{s.X, sc}}}
		}
		if body := e.walkStmts(sc, s.Body.List); len(body) > 0 {
			label := "for each "
			if len(names) > 0 {
				label += strings.Join(names, ", ") + " in "
			}
			out = append(out, &Node{Kind: "loop", Label: label + e.render(s.X, 100), Pos: e.pos(s.Pos()), Children: body})
		}
		return out
	case *ast.SwitchStmt:
		out := e.walkStmt(sc, s.Init)
		out = append(out, e.scanExpr(sc, s.Tag)...)
		label := "switch"
		if s.Tag != nil {
			label += " " + e.render(s.Tag, 100)
		}
		if cases := e.walkCases(sc, s.Body); len(cases) > 0 {
			out = append(out, &Node{Kind: "switch", Label: label, Pos: e.pos(s.Pos()), Children: cases})
		}
		return out
	case *ast.TypeSwitchStmt:
		out := e.walkStmt(sc, s.Init)
		if cases := e.walkCases(sc, s.Body); len(cases) > 0 {
			out = append(out, &Node{Kind: "switch", Label: "type switch", Pos: e.pos(s.Pos()), Children: cases})
		}
		return out
	case *ast.SelectStmt:
		var out []*Node
		for _, c := range s.Body.List {
			cc := c.(*ast.CommClause)
			out = append(out, e.walkStmts(sc, cc.Body)...)
		}
		return out
	case *ast.ReturnStmt:
		var out []*Node
		for _, r := range s.Results {
			out = append(out, e.scanExpr(sc, r)...)
		}
		if sc.condFunc && sc.errFunc && len(s.Results) == 1 {
			if id, ok := s.Results[0].(*ast.Ident); ok && id.Name == "nil" {
				return out
			}
			out = append(out, &Node{Kind: "assertion", Pos: e.pos(s.Pos()), Assertion: &Assertion{
				Library: "condition", Func: "return",
				Summary: "attempt fails, returning " + e.render(s.Results[0], e.argWidth),
				Call:    "return " + e.render(s.Results[0], 160), Origins: e.origins(sc, s.Results),
				Values: e.literals(sc, s.Results),
			}})
			return out
		}
		if sc.condFunc && len(s.Results) == 1 {
			if id, ok := s.Results[0].(*ast.Ident); ok && id.Name == "false" {
				return out
			}
			summary := e.render(s.Results[0], e.argWidth)
			if id, ok := s.Results[0].(*ast.Ident); ok && id.Name == "true" {
				summary = "condition satisfied (returns true)"
			}
			out = append(out, &Node{Kind: "assertion", Pos: e.pos(s.Pos()), Assertion: &Assertion{
				Library: "condition", Func: "return", Summary: summary,
				Call: "return " + e.render(s.Results[0], 160), Origins: e.origins(sc, s.Results),
				Values: e.literals(sc, s.Results),
			}})
		}
		return out
	}
	return nil
}

// assertionCond reports whether cond is `assert.X(...)` (passed=true) or `!assert.X(...)` (passed=false).
func (e *extractor) assertionCond(sc *scope, cond ast.Expr) (passed bool, ok bool) {
	passed = true
	if u, isNot := cond.(*ast.UnaryExpr); isNot && u.Op == token.NOT {
		passed, cond = false, u.X
	}
	call, isCall := cond.(*ast.CallExpr)
	if !isCall || e.classifyAssertion(sc, call) == nil {
		return false, false
	}
	return passed, true
}

// isExternalPkgCall reports calls to packages outside this repository (stdlib, third-party).
// hasTestingArg reports whether a call receives a *testing.T, a CollectT or a suite.
func (e *extractor) hasTestingArg(sc *scope, call *ast.CallExpr) bool {
	for _, a := range call.Args {
		switch x := a.(type) {
		case *ast.Ident:
			if v := sc.vars[x.Name]; v != nil && (v.kind != kindNone || v.typ != nil) {
				return true
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && len(x.Args) == 0 && sel.Sel.Name == "T" {
				return true
			}
		}
	}
	return false
}

// definesLocally reports whether the method is declared in the analyzed test package
// (in which case it is a custom helper rather than a testify assertion).
func (e *extractor) definesLocally(tr *typeRef, name string) bool {
	for _, m := range e.l.lookupMethod(tr, name) {
		if m.file.pkg.dir == e.pkgDir {
			return true
		}
	}
	return false
}

func (e *extractor) isExternalPkgCall(sc *scope, fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || sc.vars[id.Name] != nil {
		return false
	}
	path, isImport := sc.file.imports[id.Name]
	return isImport && e.l.importDir(path) == ""
}

func (e *extractor) walkCases(sc *scope, body *ast.BlockStmt) []*Node {
	var out []*Node
	for _, c := range body.List {
		cc, ok := c.(*ast.CaseClause)
		if !ok {
			continue
		}
		nodes := e.walkStmts(sc, cc.Body)
		if len(nodes) == 0 {
			continue
		}
		label := "default"
		if cc.List != nil {
			parts := make([]string, len(cc.List))
			for i, x := range cc.List {
				parts[i] = e.render(x, 60)
			}
			label = "case " + strings.Join(parts, ", ")
		}
		out = append(out, &Node{Kind: "case", Label: label, Pos: e.pos(cc.Pos()), Children: nodes})
	}
	return out
}

func (e *extractor) recordAssign(sc *scope, lhs, rhs []ast.Expr) {
	for i, l := range lhs {
		if sel, ok := l.(*ast.SelectorExpr); ok && len(lhs) == len(rhs) {
			// field mutation: x.F = v
			if root, ok := sel.X.(*ast.Ident); ok {
				if v := sc.vars[root.Name]; v != nil && v.origin != "" {
					cp := *v
					cp.origin = truncate(v.origin+"; then ."+sel.Sel.Name+" = "+e.render(rhs[i], 60), 220)
					cp.vals = append(append([]valRef(nil), v.vals...), valRef{rhs[i], sc})
					sc.vars[root.Name] = &cp
				}
			}
			continue
		}
		id, ok := l.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}
		var r ast.Expr
		switch {
		case len(lhs) == len(rhs):
			r = rhs[i]
		case len(rhs) == 1:
			r = rhs[0]
		default:
			continue
		}
		if _, isLit := r.(*ast.FuncLit); isLit {
			continue
		}
		v := &varInfo{origin: "← " + e.render(r, 140), rhs: r, vals: []valRef{{r, sc}}}
		if prev := sc.vars[id.Name]; prev != nil && refersTo(r, id.Name) {
			// x = append(x, ...): keep the values x had before
			v.vals = append(append([]valRef(nil), prev.vals...), v.vals...)
		}
		v.kind, v.typ = e.inferVar(sc, r)
		sc.vars[id.Name] = v
	}
}

// inferVar guesses what kind of value an expression produces.
func (e *extractor) inferVar(sc *scope, r ast.Expr) (varKind, *typeRef) {
	if call, ok := r.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && len(call.Args) == 0 {
			switch sel.Sel.Name {
			case "T":
				return kindTestingT, nil
			case "Require":
				return kindRequireObj, nil
			case "Assert":
				return kindAssertObj, nil
			}
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "New" {
			if id, ok := sel.X.(*ast.Ident); ok {
				switch sc.file.imports[id.Name] {
				case assertPkg:
					return kindAssertObj, nil
				case requirePkg:
					return kindRequireObj, nil
				}
			}
		}
	}
	return kindNone, e.typeOfExpr(sc, r)
}

func (e *extractor) typeOfExpr(sc *scope, x ast.Expr) *typeRef {
	switch t := x.(type) {
	case *ast.UnaryExpr:
		return e.typeOfExpr(sc, t.X)
	case *ast.ParenExpr:
		return e.typeOfExpr(sc, t.X)
	case *ast.CompositeLit:
		if t.Type != nil {
			return e.l.resolveType(sc.file, t.Type)
		}
	case *ast.CallExpr:
		if id, ok := t.Fun.(*ast.Ident); ok && id.Name == "new" && len(t.Args) == 1 {
			return e.l.resolveType(sc.file, t.Args[0])
		}
		// constructor-like helpers: use the declared type of the first result, or
		// the concrete type they return when they are declared to return an
		// interface (e.g. func newSuite() e2e.Suite[env] { return &mySuite{} })
		for _, c := range e.resolveCallee(sc, t.Fun) {
			if c.fi == nil || c.fi.decl.Type.Results == nil || len(c.fi.decl.Type.Results.List) == 0 {
				continue
			}
			tr := e.l.resolveType(c.fi.file, c.fi.decl.Type.Results.List[0].Type)
			if tr == nil || tr.pkg.ifaces[tr.name] {
				if concrete := e.returnedType(c.fi); concrete != nil {
					return concrete
				}
			}
			if tr != nil {
				return tr
			}
		}
	case *ast.Ident:
		if v := sc.vars[t.Name]; v != nil {
			return v.typ
		}
	}
	return nil
}

func (e *extractor) classifyType(file *fileInfo, expr ast.Expr) (varKind, *typeRef) {
	if expr == nil {
		return kindNone, nil
	}
	if sel, ok := stripType(expr).(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			path, name := file.imports[id.Name], sel.Sel.Name
			switch {
			case path == "testing" && (name == "T" || name == "TB" || name == "B" || name == "F"):
				return kindTestingT, nil
			case (path == assertPkg || path == requirePkg) && name == "TestingT":
				return kindTestingT, nil
			case path == assertPkg && name == "CollectT":
				return kindCollectT, nil
			case path == assertPkg && name == "Assertions":
				return kindAssertObj, nil
			case path == requirePkg && name == "Assertions":
				return kindRequireObj, nil
			}
		}
	}
	return kindNone, e.l.resolveType(file, expr)
}

// ---------- expressions ----------

func (e *extractor) scanExpr(sc *scope, expr ast.Expr) []*Node {
	if expr == nil {
		return nil
	}
	var out []*Node
	ast.Inspect(expr, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			out = append(out, e.handleCall(sc, x)...)
			return false
		case *ast.FuncLit:
			if nodes := e.walkFuncLit(sc, x, false, nil); len(nodes) > 0 {
				out = append(out, &Node{Kind: "callback", Label: "function literal", Pos: e.pos(x.Pos()), Children: nodes})
			}
			return false
		}
		return true
	})
	return out
}

type assertCall struct {
	lib       string
	fn        string
	base      string
	spec      assertSpec
	known     bool
	fatal     bool
	argsStart int
}

func (e *extractor) classifyAssertion(sc *scope, call *ast.CallExpr) *assertCall {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	name := sel.Sel.Name
	base, spec, _, known := lookupSpec(name)
	mk := func(lib string, fatal bool, argsStart int) *assertCall {
		fatal = fatal || (known && base == "FailNow") // assert.FailNow/s.FailNow stop the test too
		return &assertCall{lib: lib, fn: name, base: base, spec: spec, known: known, fatal: fatal, argsStart: argsStart}
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		if v := sc.vars[x.Name]; v != nil {
			switch {
			case v.kind == kindAssertObj && known:
				return mk(x.Name, false, 0)
			case v.kind == kindRequireObj && known:
				return mk(x.Name, true, 0)
			case v.kind == kindTestingT && testingFailures[name]:
				return mk(x.Name, strings.HasPrefix(name, "Fatal") || name == "FailNow", 0)
			case v.kind == kindCollectT && testingFailures[name]:
				return mk(x.Name, name == "FailNow", 0)
			case v.typ != nil && known && len(call.Args) > 0 && !e.definesLocally(v.typ, name) && e.l.isSuite(v.typ):
				// method promoted from an embedded testify suite.Suite (or a framework
				// wrapper with the same semantics, like e2e.BaseSuite.EventuallyWithT)
				return mk(x.Name, false, 0)
			}
			return nil
		}
		switch sc.file.imports[x.Name] {
		case assertPkg:
			if known {
				return mk("assert", false, 1)
			}
		case requirePkg:
			if known {
				return mk("require", true, 1)
			}
		}
	case *ast.CallExpr:
		inner, ok := x.Fun.(*ast.SelectorExpr)
		if !ok || len(x.Args) != 0 {
			return nil
		}
		recv := e.render(inner.X, 40)
		switch {
		case inner.Sel.Name == "Require" && known:
			return mk(recv+".Require()", true, 0)
		case inner.Sel.Name == "Assert" && known:
			return mk(recv+".Assert()", false, 0)
		case inner.Sel.Name == "T" && testingFailures[name]:
			return mk(recv+".T()", strings.HasPrefix(name, "Fatal") || name == "FailNow", 0)
		}
	}
	return nil
}

const flakePkg = repoModule + "/pkg/util/testutil/flake"

// skipOrFlake recognizes t.Skip*/s.T().Skip* and flake.Mark* calls: they are not
// assertions, but they tell when the assertions around them are not enforced.
func (e *extractor) skipOrFlake(sc *scope, call *ast.CallExpr) *Node {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	name := sel.Sel.Name
	switch x := sel.X.(type) {
	case *ast.Ident:
		if sc.vars[x.Name] == nil && sc.file.imports[x.Name] == flakePkg && strings.HasPrefix(name, "Mark") {
			label := "the test is marked as a known flake"
			switch {
			case name == "MarkOnLog" && len(call.Args) > 1:
				label += " when its logs contain " + e.render(call.Args[1], e.argWidth)
				if id, ok := call.Args[1].(*ast.Ident); ok && sc.vars[id.Name] == nil {
					if x, ok := sc.file.pkg.values[id.Name]; ok && x != nil {
						label += " (= " + e.render(x, e.argWidth) + ")"
					}
				}
			case name == "MarkOnLogRegex" && len(call.Args) > 1:
				label += " when its logs match " + e.render(call.Args[1], e.argWidth)
			case name == "MarkOnJobName" && len(call.Args) > 1:
				label += " on CI jobs " + e.renderList(call.Args[1:])
			}
			return &Node{Kind: "flaky", Label: label, Pos: e.pos(call.Pos())}
		}
		if v := sc.vars[x.Name]; v == nil || v.kind != kindTestingT {
			return nil
		}
	case *ast.CallExpr:
		inner, ok := x.Fun.(*ast.SelectorExpr)
		if !ok || inner.Sel.Name != "T" || len(x.Args) != 0 {
			return nil
		}
	default:
		return nil
	}
	if name != "Skip" && name != "Skipf" && name != "SkipNow" {
		return nil
	}
	label := "the test is skipped"
	if msg := e.message(call.Args); msg != "" {
		label += ": " + msg
	}
	return &Node{Kind: "skip", Label: label, Pos: e.pos(call.Pos())}
}

func (e *extractor) renderList(args []ast.Expr) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = e.render(a, 40)
	}
	return strings.Join(parts, ", ")
}

var suspiciousName = regexp.MustCompile(`(?i)^(assert|require|check|verify|validate|expect|ensure)`)

func (e *extractor) handleCall(sc *scope, call *ast.CallExpr) []*Node {
	// immediately invoked function literal
	if fl, ok := call.Fun.(*ast.FuncLit); ok {
		out := e.scanArgs(sc, call, "")
		return append(out, e.walkFuncLit(sc, fl, false, call.Args)...)
	}
	if a := e.classifyAssertion(sc, call); a != nil {
		if pollingAssertions[a.base] {
			return e.pollingNode(sc, call, a)
		}
		out := e.scanArgs(sc, call, "")
		return append(out, &Node{Kind: "assertion", Pos: e.pos(call.Pos()), Assertion: e.buildAssertion(sc, call, a)})
	}
	if n := e.suiteRun(sc, call); n != nil {
		return []*Node{n}
	}
	if n := e.skipOrFlake(sc, call); n != nil {
		return []*Node{n}
	}
	if n, ok := e.subtest(sc, call); ok {
		if n == nil {
			return nil
		}
		return []*Node{n}
	}

	name := calleeName(call.Fun)
	var out []*Node
	// chained calls (e.g. helper(t).Check()) may hide assertions in the receiver
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if _, isIdent := sel.X.(*ast.Ident); !isIdent {
			out = append(out, e.scanExpr(sc, sel.X)...)
		}
	}
	callees := e.resolveCallee(sc, call.Fun)
	willFollow := false
	for _, c := range callees {
		willFollow = willFollow || c.fi == nil || e.allowed(c.fi)
	}
	// function arguments of followed helpers are expanded where the helper calls them
	out = append(out, e.scanArgsSkip(sc, call, name, willFollow)...)

	followed := false
	var outside *funcInfo
	for _, c := range callees {
		if c.fi != nil && !e.allowed(c.fi) {
			outside = c.fi
			continue
		}
		followed = true
		// a func(...) bool literal called in an if condition is a predicate: what it
		// returns decides the branch, so report its return expressions as conditions
		predicate := e.inCond && c.lit != nil && returnsBool(c.lit.Type)
		if c.fi != nil {
			// the same helper called again performs the same checks: show them once
			// per top-level test, keeping what differs between calls (the values and
			// the function literals passed in)
			if rec, seen := e.expanded[e.expansionKeyOf(c, call)]; seen {
				own := e.scanArgs(sc, call, name)
				out = append(out, &Node{
					Kind: "helper", Label: e.render(call, 140), Pos: e.pos(call.Pos()), Def: e.relPos(c.fi.decl.Pos()),
					Values: e.literals(sc, call.Args), Ref: rec.pos, Repeat: max(0, rec.assertions-countAssertions(own)), Children: own,
				})
				continue
			}
		}
		var children []*Node
		if predicate {
			e.errCond = false
			children = e.expandCalleeCond(sc, c, call.Args, true)
		} else {
			children = e.expandCallee(sc, c, call.Args)
		}
		if len(children) == 0 {
			continue
		}
		n := &Node{Kind: "helper", Label: e.render(call, 140), Pos: e.pos(call.Pos()), Children: children}
		if c.fi != nil {
			n.Def = e.relPos(c.fi.decl.Pos())
			n.Values = e.literals(sc, call.Args)
			if key := e.expansionKeyOf(c, call); e.expanded[key] == nil {
				e.expanded[key] = &expansion{pos: n.Pos, assertions: countAssertions(children)}
			}
		} else {
			n.Kind = "closure"
			if predicate {
				n.Kind = "predicate"
			}
		}
		out = append(out, n)
	}
	accessor := len(call.Args) == 0 && (name == "Require" || name == "Assert" || name == "T")
	if !followed && strings.HasPrefix(name, "Must") && !e.isExternalPkgCall(sc, call.Fun) {
		// e2e helpers such as RemoteHost.MustExecute fail the test when the action fails
		out = append(out, &Node{Kind: "implicit", Label: e.render(call, 140), Pos: e.pos(call.Pos()), Values: e.literals(sc, call.Args)})
		return out
	}
	if !followed && !accessor && suspiciousName.MatchString(name) && !e.isExternalPkgCall(sc, call.Fun) &&
		(e.stmtCalls[call] || e.hasTestingArg(sc, call)) {
		n := &Node{Kind: "opaque", Label: e.render(call, 140), Pos: e.pos(call.Pos()), Values: e.literals(sc, call.Args)}
		if outside != nil {
			n.Def = e.relPos(outside.decl.Pos())
		}
		out = append(out, n)
	}
	return out
}

// scanArgs looks for assertions inside call arguments, including callbacks.
func (e *extractor) scanArgs(sc *scope, call *ast.CallExpr, callee string) []*Node {
	return e.scanArgsSkip(sc, call, callee, false)
}

func (e *extractor) scanArgsSkip(sc *scope, call *ast.CallExpr, callee string, skipFuncs bool) []*Node {
	var out []*Node
	for _, arg := range call.Args {
		var nodes []*Node
		switch a := arg.(type) {
		case *ast.FuncLit:
			if skipFuncs {
				continue
			}
			nodes = e.walkFuncLit(sc, a, false, nil)
		case *ast.Ident, *ast.SelectorExpr:
			if skipFuncs {
				continue
			}
			// function values (closures, helpers, method values)
			for _, c := range e.resolveCallee(sc, a) {
				if c.fi == nil || e.allowed(c.fi) {
					nodes = append(nodes, e.expandCallee(sc, c, nil)...)
				}
			}
		default:
			out = append(out, e.scanExpr(sc, arg)...)
			continue
		}
		if len(nodes) > 0 {
			label := "callback"
			if callee != "" {
				label = "callback passed to " + callee + "()"
			}
			out = append(out, &Node{Kind: "callback", Label: label, Pos: e.pos(arg.Pos()), Children: nodes})
		}
	}
	return out
}

func (e *extractor) pollingNode(sc *scope, call *ast.CallExpr, a *assertCall) []*Node {
	args := call.Args[min(a.argsStart, len(call.Args)):]
	var summary string
	switch a.base {
	case "EventuallyWithT":
		summary = "eventually, all assertions below pass in a single attempt"
	case "Eventually":
		summary = "eventually, the condition below returns true"
	case "Never":
		summary = "the condition below never returns true"
	case "Condition":
		summary = "the condition below returns true"
	case "EventuallyWithExponentialBackoff":
		summary = "eventually, the function below returns nil"
		if len(args) >= 3 {
			summary += fmt.Sprintf(" (exponential backoff, max elapsed %s, max interval %s)", e.render(args[1], 40), e.render(args[2], 40))
		}
	}
	e.errCond = a.base == "EventuallyWithExponentialBackoff"
	if len(args) >= 3 && a.base != "EventuallyWithExponentialBackoff" {
		summary += fmt.Sprintf(" (timeout %s, every %s)", e.render(args[1], 40), e.render(args[2], 40))
	}
	asrt := &Assertion{Library: a.lib, Func: a.fn, Fatal: a.fatal, Summary: summary, Call: e.render(call, 160)}
	arity := a.spec.arity
	if a.base == "Condition" {
		arity = 1
	}
	if len(args) > arity {
		asrt.Message = e.message(args[arity:])
	}
	n := &Node{Kind: "eventually", Pos: e.pos(call.Pos()), Assertion: asrt}
	if len(args) == 0 {
		return []*Node{n}
	}
	cond := a.base != "EventuallyWithT"
	switch fn := args[0].(type) {
	case *ast.FuncLit:
		n.Children = e.walkFuncLit(sc, fn, cond, nil)
	default:
		callees := e.resolveCallee(sc, fn)
		for _, c := range callees {
			if c.fi == nil || e.allowed(c.fi) {
				n.Children = append(n.Children, e.expandCalleeCond(sc, c, nil, cond)...)
			}
		}
		if len(callees) == 0 {
			n.Children = e.scanExpr(sc, fn)
		}
	}
	return []*Node{n}
}

func (e *extractor) buildAssertion(sc *scope, call *ast.CallExpr, a *assertCall) *Assertion {
	args := call.Args[min(a.argsStart, len(call.Args)):]
	arity := len(args)
	if a.known {
		arity = min(a.spec.arity, len(args))
	}
	if testingFailures[a.fn] && !a.known || strings.HasSuffix(a.lib, ".T()") || sc.isTestingVar(a.lib) {
		arity = 0
	}
	main, extra := args[:arity], args[arity:]
	rendered := make([]string, len(main))
	for i, x := range main {
		rendered[i] = e.render(x, e.argWidth)
	}
	asrt := &Assertion{
		Library: a.lib, Func: a.fn, Fatal: a.fatal, Args: rendered,
		Message: e.message(extra), Call: e.render(call, 160), Origins: e.origins(sc, main),
		Values: e.literals(sc, main),
	}
	switch {
	case arity == 0 && len(main) == 0 && testingFailures[a.fn]:
		asrt.Summary = "test fails"
		if asrt.Message != "" {
			asrt.Summary += ": " + asrt.Message
			asrt.Message = ""
		}
		asrt.Origins = e.origins(sc, extra)
		// values of the failure message, minus the literal parts already in the summary
		for _, v := range e.literals(sc, extra) {
			if !strings.Contains(asrt.Summary, v) {
				asrt.Values = append(asrt.Values, v)
			}
		}
	case a.known && a.spec.tmpl != "":
		asrt.Summary = fillTemplate(a.spec.tmpl, rendered)
	default:
		asrt.Summary = fmt.Sprintf("%s(%s)", a.fn, strings.Join(rendered, ", "))
	}
	return asrt
}

// pkgLevel returns the declaration of a package-level const/var, if any.
func (e *extractor) pkgLevel(sc *scope, name string) *varInfo {
	if x, ok := sc.file.pkg.values[name]; ok && x != nil {
		return &varInfo{origin: "(package-level) = " + e.render(x, 120), rhs: x,
			vals: []valRef{{x, &scope{file: sc.file.pkg.valueFiles[name], vars: map[string]*varInfo{}}}}}
	}
	return nil
}

func (s *scope) isTestingVar(name string) bool {
	v := s.vars[name]
	return v != nil && (v.kind == kindTestingT || v.kind == kindCollectT)
}

func (e *extractor) message(extra []ast.Expr) string {
	if len(extra) == 0 {
		return ""
	}
	if lit, ok := extra[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
		msg, err := strconv.Unquote(lit.Value)
		if err == nil {
			if len(extra) > 1 {
				rest := make([]string, len(extra)-1)
				for i, x := range extra[1:] {
					rest[i] = e.render(x, 50)
				}
				msg += " ‹" + strings.Join(rest, ", ") + "›"
			}
			return msg
		}
	}
	parts := make([]string, len(extra))
	for i, x := range extra {
		parts[i] = e.render(x, 60)
	}
	return strings.Join(parts, ", ")
}

// origins explains, best effort, where the variables used in exprs come from.
func (e *extractor) origins(sc *scope, exprs []ast.Expr) []Origin {
	var out []Origin
	seen := map[string]bool{}
	var collect func(x ast.Node, depth int)
	collect = func(x ast.Node, depth int) {
		var visit func(n ast.Node) bool
		visit = func(n ast.Node) bool {
			switch t := n.(type) {
			case *ast.SelectorExpr:
				ast.Inspect(t.X, visit)
				return false
			case *ast.KeyValueExpr:
				ast.Inspect(t.Value, visit)
				return false
			case *ast.FuncLit:
				return false
			case *ast.Ident:
				v := sc.vars[t.Name]
				if v == nil {
					v = e.pkgLevel(sc, t.Name)
				}
				if v == nil || v.origin == "" || seen[t.Name] || len(out) >= 6 {
					return false
				}
				seen[t.Name] = true
				out = append(out, Origin{Name: t.Name, From: v.origin})
				if v.rhs != nil && depth < 2 {
					collect(v.rhs, depth+1)
				}
			}
			return true
		}
		ast.Inspect(x, visit)
	}
	for _, x := range exprs {
		collect(x, 0)
	}
	return out
}

// ---------- subtests and suites ----------

func (e *extractor) subtest(sc *scope, call *ast.CallExpr) (*Node, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || len(call.Args) != 2 {
		return nil, false
	}
	isRunner := false
	switch x := sel.X.(type) {
	case *ast.CallExpr:
		if inner, ok := x.Fun.(*ast.SelectorExpr); ok && inner.Sel.Name == "T" && len(x.Args) == 0 {
			isRunner = true
		}
	case *ast.Ident:
		if v := sc.vars[x.Name]; v != nil && (v.kind == kindTestingT || v.typ != nil) {
			isRunner = true
		}
	}
	if !isRunner {
		return nil, false
	}
	var children []*Node
	switch fn := call.Args[1].(type) {
	case *ast.FuncLit:
		children = e.walkFuncLit(sc, fn, false, nil)
	default:
		for _, c := range e.resolveCallee(sc, fn) {
			if c.fi == nil || e.allowed(c.fi) {
				children = append(children, e.expandCallee(sc, c, nil)...)
			}
		}
	}
	children = append(e.scanExpr(sc, call.Args[0]), children...)
	if len(children) == 0 {
		return nil, true
	}
	return &Node{Kind: "subtest", Label: e.render(call.Args[0], 100), Pos: e.pos(call.Pos()), Children: children}, true
}

func (e *extractor) suiteRun(sc *scope, call *ast.CallExpr) *Node {
	fun := call.Fun
	switch f := fun.(type) { // e2e.Run[environments.Host](t, s)
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || len(call.Args) < 2 {
		return nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || sc.vars[id.Name] != nil {
		return nil
	}
	if path := sc.file.imports[id.Name]; path != e2ePkg && path != suitePkg {
		return nil
	}
	tr := e.typeOfExpr(sc, call.Args[1])
	if tr == nil {
		return &Node{Kind: "opaque", Label: "runs a suite whose type could not be resolved: " + e.render(call, 120), Pos: e.pos(call.Pos())}
	}
	n := &Node{Kind: "suite", Label: tr.name, Pos: e.pos(call.Pos()), Def: e.relPath(tr.pkg.dir)}
	for _, m := range e.l.methodSet(tr) {
		switch {
		case strings.HasPrefix(m.name, "Test"):
			n.Children = append(n.Children, e.testNode(m))
		case lifecycleHooks[m.name] && e.allowed(m):
			if children := e.expandCallee(nil, callee{fi: m}, nil); len(children) > 0 {
				n.Children = append(n.Children, &Node{Kind: "hook", Label: fmt.Sprintf("(%s).%s", m.recv, m.name), Pos: e.relPos(m.decl.Pos()), Children: children})
			}
		}
	}
	return n
}

// ---------- call resolution ----------

type callee struct {
	fi    *funcInfo
	lit   *ast.FuncLit
	scope *scope   // scope captured by lit
	recv  ast.Expr // receiver expression of a method call, in the caller's scope
}

func withRecv(cs []callee, recv ast.Expr) []callee {
	for i := range cs {
		cs[i].recv = recv
	}
	return cs
}

func (e *extractor) resolveCallee(sc *scope, fun ast.Expr) []callee {
	wrap := func(fs []*funcInfo) []callee {
		out := make([]callee, 0, len(fs))
		for _, f := range fs {
			if f.decl.Body != nil {
				out = append(out, callee{fi: f})
			}
		}
		return out
	}
	switch f := fun.(type) {
	case *ast.ParenExpr:
		return e.resolveCallee(sc, f.X)
	case *ast.IndexExpr:
		return e.resolveCallee(sc, f.X)
	case *ast.IndexListExpr:
		return e.resolveCallee(sc, f.X)
	case *ast.Ident:
		if v := sc.vars[f.Name]; v != nil {
			if v.closure != nil {
				return []callee{{lit: v.closure, scope: v.capture}}
			}
			return v.fnRefs
		}
		return wrap(sc.file.pkg.funcs[f.Name])
	case *ast.SelectorExpr:
		id, ok := f.X.(*ast.Ident)
		if !ok {
			// method on an expression of known type: T{...}.Assert(t), helper(t).Check()
			if tr := e.typeOfExpr(sc, f.X); tr != nil {
				return withRecv(wrap(e.l.lookupMethod(tr, f.Sel.Name)), f.X)
			}
			return e.fieldFuncs(sc, f.X, f.Sel.Name)
		}
		if v := sc.vars[id.Name]; v != nil {
			if v.typ != nil {
				if ms := wrap(e.l.lookupMethod(v.typ, f.Sel.Name)); len(ms) > 0 {
					return withRecv(ms, f.X)
				}
			}
			// function-valued struct field: e.test(p) with e := T{test: func(...) {...}}
			return e.fieldFuncs(sc, f.X, f.Sel.Name)
		}
		if dir := e.l.importDir(sc.file.imports[id.Name]); dir != "" {
			return wrap(e.l.load(dir, false).funcs[f.Sel.Name])
		}
	}
	return nil
}

func (e *extractor) allowed(fi *funcInfo) bool {
	switch e.follow {
	case followRepo:
		return true
	case followPackage:
		return fi.file.pkg.dir == e.pkgDir
	default:
		return fi.file.pkg.module == e.module
	}
}

func (e *extractor) expandCallee(sc *scope, c callee, args []ast.Expr) []*Node {
	return e.expandCalleeCond(sc, c, args, false)
}

func (e *extractor) expandCalleeCond(sc *scope, c callee, args []ast.Expr, cond bool) []*Node {
	depth := 0
	if sc != nil {
		depth = sc.depth
	}
	if depth >= e.maxDepth {
		return nil
	}
	var key ast.Node = c.lit
	if c.fi != nil {
		key = c.fi.decl
	}
	if e.active[key] {
		return nil // recursion
	}
	e.active[key] = true
	defer delete(e.active, key)
	prevCond := e.inCond
	e.inCond = false // statements of the callee are not part of the caller's condition
	defer func() { e.inCond = prevCond }()

	if c.lit != nil {
		base := sc
		if c.scope != nil {
			base = c.scope
		}
		nsc := base.child()
		nsc.depth = depth + 1
		nsc.condFunc = cond
		nsc.errFunc = cond && e.errCond
		e.bindParams(nsc, sc, c.lit.Type, args)
		return e.walkStmts(nsc, c.lit.Body.List)
	}
	nsc := e.funcScope(sc, c.fi, args, depth+1)
	if c.recv != nil && sc != nil && len(c.fi.decl.Recv.List) > 0 {
		for _, n := range c.fi.decl.Recv.List[0].Names {
			if v := nsc.vars[n.Name]; v != nil {
				v.vals = []valRef{{c.recv, sc}} // the receiver's value, e.g. a struct literal
			}
		}
	}
	nsc.condFunc = cond
	nsc.errFunc = cond && e.errCond
	return e.walkStmts(nsc, c.fi.decl.Body.List)
}

func (e *extractor) funcScope(caller *scope, fi *funcInfo, args []ast.Expr, depth int) *scope {
	sc := &scope{file: fi.file, vars: map[string]*varInfo{}, depth: depth}
	if fi.decl.Recv != nil && len(fi.decl.Recv.List) > 0 {
		r := fi.decl.Recv.List[0]
		for _, n := range r.Names {
			sc.vars[n.Name] = &varInfo{typ: &typeRef{pkg: fi.file.pkg, name: fi.recv}}
		}
	}
	e.bindParams(sc, caller, fi.decl.Type, args)
	return sc
}

func (e *extractor) walkFuncLit(sc *scope, fl *ast.FuncLit, cond bool, args []ast.Expr) []*Node {
	nsc := sc.child()
	nsc.condFunc = cond
	nsc.errFunc = cond && e.errCond
	e.bindParams(nsc, sc, fl.Type, args)
	return e.walkStmts(nsc, fl.Body.List)
}

func (e *extractor) bindParams(sc, caller *scope, ft *ast.FuncType, args []ast.Expr) {
	if ft.Params == nil {
		return
	}
	i := 0
	for _, field := range ft.Params.List {
		kind, typ := e.classifyType(sc.file, field.Type)
		if len(field.Names) == 0 {
			i++
			continue
		}
		_, variadic := field.Type.(*ast.Ellipsis)
		for _, name := range field.Names {
			v := &varInfo{kind: kind, typ: typ}
			if caller != nil && i < len(args) {
				bound := args[i : i+1]
				if variadic {
					bound = args[i:]
				}
				parts := make([]string, len(bound))
				for j, b := range bound {
					parts[j] = e.render(b, 100)
				}
				v.origin = "= " + strings.Join(parts, ", ")
				for _, b := range bound {
					v.vals = append(v.vals, valRef{b, caller})
				}
				id, isIdent := bound[0].(*ast.Ident)
				if isIdent && len(bound) == 1 && caller.vars[id.Name] != nil && caller.vars[id.Name].origin == "" {
					v.origin = "" // receiver, t, or another param without known origin: not informative
				}
				cv := caller.vars[identName(id)]
				if cv == nil && isIdent {
					cv = e.pkgLevel(caller, id.Name)
				}
				if isIdent && len(bound) == 1 && cv != nil {
					switch {
					case cv.origin != "" && id.Name == name.Name:
						v.origin = cv.origin
						v.rhs = cv.rhs
					case cv.origin != "":
						v.origin += "; " + id.Name + " " + cv.origin
					}
					if v.kind == kindNone {
						v.kind = cv.kind
					}
					if v.typ == nil {
						v.typ = cv.typ
					}
				} else if os := e.origins(caller, bound); len(os) > 0 {
					v.origin += "; " + os[0].Name + " " + os[0].From
				}
				v.origin = truncate(v.origin, 160)
			}
			if caller != nil && i < len(args) {
				// generic or interface-typed params: prefer the concrete type of the argument
				if v.typ == nil || v.typ.pkg.ifaces[v.typ.name] {
					if tr := e.typeOfExpr(caller, args[i]); tr != nil {
						v.typ = tr
					}
				}
				switch a := args[i].(type) {
				case *ast.FuncLit:
					v.closure, v.capture = a, caller
				case *ast.Ident, *ast.SelectorExpr:
					for _, c := range e.resolveCallee(caller, a) {
						if c.lit != nil {
							v.closure, v.capture = c.lit, c.scope
						} else {
							v.fnRefs = append(v.fnRefs, c)
						}
					}
				}
			}
			if name.Name != "_" {
				sc.vars[name.Name] = v
			}
			i++
		}
	}
}

func returnsBool(ft *ast.FuncType) bool {
	if ft.Results == nil || len(ft.Results.List) != 1 || len(ft.Results.List[0].Names) > 1 {
		return false
	}
	id, ok := ft.Results.List[0].Type.(*ast.Ident)
	return ok && id.Name == "bool"
}

// fieldFuncs resolves a call to a function-valued field, x.name(...), to the
// function literals (or named functions) stored in that field of the struct
// literals x was built from.
func (e *extractor) fieldFuncs(sc *scope, x ast.Expr, name string) []callee {
	var out []callee
	for _, r := range e.fieldExprs(sc, x, []string{name}, map[any]bool{}, 0) {
		switch f := r.x.(type) {
		case *ast.FuncLit:
			out = append(out, callee{lit: f, scope: r.sc})
		case *ast.Ident, *ast.SelectorExpr:
			out = append(out, e.resolveCallee(r.sc, f)...)
		}
	}
	return out
}

// fieldExprs returns the expressions x.path[0].path[1]... was set to in the
// struct literals x comes from (see litCollector.project for the traversal).
func (e *extractor) fieldExprs(sc *scope, x ast.Expr, path []string, visited map[any]bool, depth int) []valRef {
	if x == nil || sc == nil || depth > 10 {
		return nil
	}
	if len(path) == 0 {
		if id, ok := x.(*ast.Ident); ok {
			if v := sc.vars[id.Name]; v != nil && !visited[v] && (v.closure == nil && len(v.fnRefs) == 0) {
				visited[v] = true
				var out []valRef
				for _, r := range v.vals {
					out = append(out, e.fieldExprs(r.sc, r.x, nil, visited, depth+1)...)
				}
				if len(out) > 0 {
					return out
				}
			}
		}
		return []valRef{{x, sc}}
	}
	switch t := x.(type) {
	case *ast.ParenExpr:
		return e.fieldExprs(sc, t.X, path, visited, depth)
	case *ast.StarExpr:
		return e.fieldExprs(sc, t.X, path, visited, depth)
	case *ast.UnaryExpr:
		return e.fieldExprs(sc, t.X, path, visited, depth)
	case *ast.IndexExpr:
		return e.fieldExprs(sc, t.X, path, visited, depth)
	case *ast.SelectorExpr:
		return e.fieldExprs(sc, t.X, append([]string{t.Sel.Name}, path...), visited, depth)
	case *ast.Ident:
		v := sc.vars[t.Name]
		if v == nil || visited[[2]any{v, len(path)}] {
			return nil
		}
		visited[[2]any{v, len(path)}] = true
		var out []valRef
		for _, r := range v.vals {
			out = append(out, e.fieldExprs(r.sc, r.x, path, visited, depth+1)...)
		}
		return out
	case *ast.CompositeLit:
		var out []valRef
		keyed := false
		for _, elt := range t.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok || !isFieldKey(sc, kv.Key) {
				continue
			}
			keyed = true
			if kv.Key.(*ast.Ident).Name == path[0] {
				out = append(out, e.fieldExprs(sc, kv.Value, path[1:], visited, depth)...)
			}
		}
		if keyed {
			return out
		}
		for _, elt := range t.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			out = append(out, e.fieldExprs(sc, elt, path, visited, depth)...)
		}
		return out
	}
	return nil
}

// refersTo reports whether x mentions the identifier name.
func refersTo(x ast.Expr, name string) bool {
	found := false
	ast.Inspect(x, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// returnedType infers the concrete type a function returns from its return
// statements (first result), following simple local assignments.
func (e *extractor) returnedType(fi *funcInfo) *typeRef {
	if fi.decl.Body == nil || e.retActive[fi.decl] {
		return nil
	}
	e.retActive[fi.decl] = true
	defer delete(e.retActive, fi.decl)
	sc := &scope{file: fi.file, vars: map[string]*varInfo{}}
	var found *typeRef
	ast.Inspect(fi.decl.Body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		switch t := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			e.recordAssign(sc, t.Lhs, t.Rhs)
		case *ast.ReturnStmt:
			if len(t.Results) > 0 {
				if tr := e.typeOfExpr(sc, t.Results[0]); tr != nil && !tr.pkg.ifaces[tr.name] {
					found = tr
				}
			}
		}
		return true
	})
	return found
}

func identName(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Name
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.IndexExpr:
		return calleeName(f.X)
	case *ast.ParenExpr:
		return calleeName(f.X)
	}
	return ""
}

// ---------- rendering helpers ----------

var elided = &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: ast.NewIdent("…")}}}

// render prints an expression on a single line, eliding function literal bodies.
func (e *extractor) render(n ast.Node, width int) string {
	if n == nil {
		return ""
	}
	type saved struct {
		lit  *ast.FuncLit
		body *ast.BlockStmt
	}
	var lits []saved
	ast.Inspect(n, func(x ast.Node) bool {
		if fl, ok := x.(*ast.FuncLit); ok {
			lits = append(lits, saved{fl, fl.Body})
			return false
		}
		return true
	})
	for _, s := range lits {
		s.lit.Body = elided
	}
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, token.NewFileSet(), n)
	for _, s := range lits {
		s.lit.Body = s.body
	}
	return truncate(strings.Join(strings.Fields(buf.String()), " "), width)
}

func truncate(s string, width int) string {
	if width > 0 && len([]rune(s)) > width {
		return string([]rune(s)[:width-1]) + "…"
	}
	return s
}

func (e *extractor) pos(p token.Pos) string {
	pos := e.l.fset.Position(p)
	return fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)
}

func (e *extractor) relPos(p token.Pos) string {
	pos := e.l.fset.Position(p)
	return fmt.Sprintf("%s:%d", e.relPath(pos.Filename), pos.Line)
}

func (e *extractor) relPath(path string) string {
	if rel, err := filepath.Rel(e.l.root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}
