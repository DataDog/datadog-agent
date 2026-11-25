// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pass holds the compilation passes of the SECL compiler.
package pass

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// ResolveFields returns the pass replacing the names by the references to the
// fields they refer to, once the macros and the constants are resolved (see
// ResolveIdents): every name left refers to a field.
//
// The name of the field is the dotted name, without its subscript, a legacy
// field being renamed by legacy:
//   - `process.file.name` is the field `process.file.name`;
//   - `process.ancestors[A].file.name` is the field `process.ancestors.file.name`
//     iterated over `process.ancestors` with the variable `A`;
//   - `process.argv[0]` is the Index 0 of the field `process.argv`, the
//     subscript kept as an operation on the field;
//   - `process.env["PATH"]` is the Index "PATH" of the field `process.env`, a
//     key in a dictionary, kept as well.
//
// The subscript ending a name, unless an identifier, is an index or a key, any
// operand, its names resolved as fields: `process.env[process.file.name]`. The
// name it subscripts has at most one other subscript, an iterator variable, an
// identifier.
func ResolveFields(legacy map[string]string) Pass {
	return func(node ast.Node) (ast.Node, error) {
		// one resolver per tree, its error is the one of the tree
		r := &fieldResolver{legacy: legacy}
		node = r.edit(node)
		return node, r.err
	}
}

// fieldResolver replaces the names by field references.
type fieldResolver struct {
	legacy map[string]string
	// err is the first error met, the rest of the tree is left unchanged
	err error
}

// edit returns the node, its names replaced by field references.
func (r *fieldResolver) edit(node ast.Node) ast.Node {
	if r.err != nil {
		return node
	}

	switch node.(type) {
	case *ast.Ident, *ast.Selector, *ast.Index:
		// an Index of a field reference is already resolved, the tree of a
		// macro being resolved before it is inserted
		if !isName(node) {
			break
		}
		if index, ok := node.(*ast.Index); ok {
			if _, ok := index.Subscript.(*ast.Ident); !ok {
				// an index or a key, not an iterator variable: its name and
				// its subscript are edited below
				break
			}
		}
		field, err := r.resolve(node)
		if err != nil {
			r.err = err
			return node
		}
		return field
	}

	ast.EditChildren(node, r.edit)
	return node
}

// isName returns whether a node is a name: a chain of selectors and subscripts
// over an identifier.
func isName(node ast.Node) bool {
	for {
		switch n := node.(type) {
		case *ast.Ident:
			return true
		case *ast.Selector:
			node = n.Base
		case *ast.Index:
			node = n.Base
		default:
			return false
		}
	}
}

// resolve returns the reference to the field a name refers to.
func (r *fieldResolver) resolve(name ast.Node) (ast.Node, error) {
	var (
		dotted string
		// subscript is the subscript of the name, and prefix the name it
		// subscripts
		subscript *ast.Index
		prefix    string
	)

	var flatten func(ast.Node) error
	flatten = func(node ast.Node) error {
		switch n := node.(type) {
		case *ast.Ident:
			dotted = n.Name
		case *ast.Selector:
			if err := flatten(n.Base); err != nil {
				return err
			}
			dotted += "." + n.Member
		case *ast.Index:
			if err := flatten(n.Base); err != nil {
				return err
			}
			if subscript != nil {
				return fmt.Errorf("`%s` has more than one subscript", ast.Sprint(name))
			}
			subscript, prefix = n, dotted
		}
		return nil
	}
	if err := flatten(name); err != nil {
		return nil, err
	}

	field := &ast.FieldRef{Meta: *name.Metadata(), Name: dotted}
	if renamed, ok := r.legacy[dotted]; ok {
		field.Name = renamed
	}

	if subscript == nil {
		return field, nil
	}

	index, ok := subscript.Subscript.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("`%s`: cannot index the iterator `%s` with `%s`, use an iterator variable instead", ast.Sprint(name), prefix, ast.Sprint(subscript.Subscript))
	}
	field.Iter = &ast.Iteration{Iterator: prefix, Register: index.Name}
	return field, nil
}
