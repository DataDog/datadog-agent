// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
)

// Optimize rewrites every field read in a checked expression into a read by
// index — `evt.process.file.path` becomes `secl.readString(evt, 4711)` — and
// returns the fields the expression reads.
//
// Which field a chain of selects denotes is decided here, once per rule, and the
// index it becomes is all the interpreter needs afterwards. That is what lets the
// per-event machinery for walking a chain go: no positions above a leaf, no
// per-member binding, nothing resolved by name while an event is being matched.
//
// It runs on the checked expression rather than on the translation, so the CEL a
// rule reads as is unaffected — see Translate — and the types the checker inferred
// are what say which read to emit. cel-go re-checks the result, so a rewrite that
// does not preserve a type fails here rather than misreading later.
func Optimize(env *cel.Env, checked *cel.Ast) (*cel.Ast, []string, error) {
	pass := &fieldReads{fields: map[string]bool{}}

	optimizer, err := cel.NewStaticOptimizer(pass)
	if err != nil {
		return nil, nil, fmt.Errorf("building the SECL optimizer: %w", err)
	}

	optimized, iss := optimizer.Optimize(env, checked)
	// The pass reports its own errors through the optimizer's issues, so check
	// those first: they say what went wrong, where the issue list says only that
	// the result did not check.
	if pass.err != nil {
		return nil, nil, pass.err
	}
	if iss.Err() != nil {
		return nil, nil, iss.Err()
	}

	return optimized, sortedFields(pass.fields), nil
}

// fieldReads is the cel.ASTOptimizer that does it.
type fieldReads struct {
	fields map[string]bool
	err    error
}

// Optimize implements cel.ASTOptimizer.
func (p *fieldReads) Optimize(ctx *cel.OptimizerContext, a *celast.AST) *celast.AST {
	root := celast.NavigateAST(a)

	// Bottom up, which is what lets a chain stand on another one: the elements of
	// an iterated field become a read before the field of an element does.
	for _, expr := range celast.MatchDescendants(root, celast.KindMatcher(celast.SelectKind)) {
		if consumedByAnotherSelect(expr) {
			continue
		}
		if err := p.rewrite(ctx, expr); err != nil {
			p.err = err
			return a
		}
	}

	return a
}

// consumedByAnotherSelect reports whether this select is in the middle of a chain,
// in which case the select at the end of it stands for the whole thing. A select
// has exactly one child, its operand, so a parent that is a select can only be
// selecting through this one.
func consumedByAnotherSelect(expr celast.NavigableExpr) bool {
	parent, ok := expr.Parent()
	return ok && parent.Kind() == celast.SelectKind
}

// rewrite replaces one chain of selects with the read it denotes.
func (p *fieldReads) rewrite(ctx *cel.OptimizerContext, expr celast.NavigableExpr) error {
	base, path, testOnly, ok := chain(expr)
	if !ok {
		// Not a field: a variable under VariablesRoot, or anything else reached
		// through a dynamic value. cel-go resolves those itself.
		return nil
	}
	if testOnly {
		return fmt.Errorf("a presence test over the event field %q is not supported", path)
	}

	read, index, err := readOf(path, expr.Type())
	if err != nil {
		return err
	}
	p.fields[path] = true

	ctx.UpdateExpr(expr, ctx.NewCall(read, base, ctx.NewLiteral(types.Int(index))))
	return nil
}

// chain resolves what a chain of selects reads: the expression the field hangs
// off, and the SECL field it names.
//
// The base is what carries the position at evaluation time, and its type says
// where in the namespace the chain starts — the root type for `evt`, an iterated
// node's element type for an iteration variable or for what a subscript yielded.
// A select on any of them is therefore the same case, which is why an iteration
// variable needs none of its own.
func chain(expr celast.NavigableExpr) (base celast.Expr, path string, testOnly bool, ok bool) {
	var segments []string

	node := expr
	for node.Kind() == celast.SelectKind {
		sel := node.AsSelect()
		testOnly = testOnly || sel.IsTestOnly()
		segments = append(segments, sel.FieldName())

		children := node.Children()
		if len(children) != 1 {
			return nil, "", false, false
		}
		node = children[0]
	}

	prefix, ok := modelPaths[node.Type().TypeName()]
	if !ok {
		return nil, "", false, false
	}

	// The segments were collected from the end of the chain backwards.
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}

	return node, join(prefix, strings.Join(segments, ".")), testOnly, true
}

// readOf names the function and the index a field is read by.
//
// The type is the one the checker inferred for the chain, so the function it picks
// returns what the expression was already checked as, and the rewrite cannot
// change what the rule means.
func readOf(path string, result *types.Type) (string, int, error) {
	if index, ok := celIteratorIndex[path]; ok {
		elements, isObjectList := objectListElem(result)
		if !isObjectList {
			return "", 0, fmt.Errorf("field %q is iterated but is read as %s", path, result)
		}
		read, ok := celElementReads[elements.TypeName()]
		if !ok {
			return "", 0, fmt.Errorf("field %q yields %s, which has no element read", path, elements)
		}
		return read, index, nil
	}

	index, ok := celReaderIndex[path]
	if !ok {
		// The types and the readers are two outputs of one generator run, and this
		// is the only way a field is read, so a path that is typed but unreadable
		// has to fail the rule rather than fall back to something slower.
		return "", 0, fmt.Errorf("field %q has no reader", path)
	}

	read, ok := leafReadFunc(result)
	if !ok {
		return "", 0, fmt.Errorf("field %q is read as %s, which no read function returns", path, result)
	}
	return read, index, nil
}

// leafReadFunc names the function that reads a leaf of the given type.
func leafReadFunc(t *types.Type) (string, bool) {
	if t.Kind() == types.ListKind {
		read, ok := listReads[t.Parameters()[0].TypeName()]
		return read, ok
	}
	read, ok := scalarReads[t.TypeName()]
	return read, ok
}

// scalarReads and listReads are keyed by type name rather than by type, because a
// list type is built on demand and so is never the same value twice.
var (
	scalarReads = map[string]string{
		types.StringType.TypeName(): ReadStringFunc,
		types.IntType.TypeName():    ReadIntFunc,
		types.BoolType.TypeName():   ReadBoolFunc,
		ext.CIDRType.TypeName():     ReadCIDRFunc,
	}

	listReads = map[string]string{
		types.StringType.TypeName(): ReadStringsFunc,
		types.IntType.TypeName():    ReadIntsFunc,
		types.BoolType.TypeName():   ReadBoolsFunc,
		ext.CIDRType.TypeName():     ReadCIDRsFunc,
	}
)

func sortedFields(fields map[string]bool) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
