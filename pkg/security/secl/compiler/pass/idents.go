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

// ResolveIdents returns the pass replacing the identifiers referring to a macro
// or to a constant by a copy of the tree they refer to. lookup returns the tree
// a name refers to: the tree of a macro, its own identifiers already resolved,
// or the value of a constant.
//
// The root of the copy is positioned at the reference. Its other nodes keep
// their position, except the ones without a position, the nodes of a constant,
// which are positioned at the reference too.
//
// An array of identifiers, `[O_RDONLY, O_WRONLY]`, is replaced by the array of
// the trees they refer to, which must all be literals.
//
// Once the identifiers are resolved, an identifier refers to a field.
func ResolveIdents(lookup func(name string) (ast.Node, bool)) Pass {
	return func(node ast.Node) (ast.Node, error) {
		// one editor per tree, its error is the one of the tree
		e := &identEditor{lookup: lookup}
		node = e.edit(node)
		return node, e.err
	}
}

// identEditor replaces the identifiers referring to a macro or to a constant.
type identEditor struct {
	lookup func(name string) (ast.Node, bool)
	// err is the first error met, the rest of the tree is left unchanged
	err error
}

// edit returns the node, the identifiers of its tree referring to a macro or to
// a constant replaced.
func (e *identEditor) edit(node ast.Node) ast.Node {
	if e.err != nil {
		return node
	}

	switch n := node.(type) {
	case *ast.Ident:
		// the tree a name refers to is already resolved, it isn't walked
		if tree, ok := e.lookup(n.Name); ok {
			return copyAt(tree, n.Pos)
		}
		return node
	case *ast.IdentArray:
		array, err := e.resolveIdentArray(n)
		if err != nil {
			e.err = err
			return node
		}
		return array
	}

	ast.EditChildren(node, e.edit)
	return node
}

// resolveIdentArray returns the array of the trees the identifiers of an array
// of identifiers refer to.
func (e *identEditor) resolveIdentArray(node *ast.IdentArray) (ast.Node, error) {
	members := make([]*ast.Literal, 0, len(node.Names))
	for _, name := range node.Names {
		tree, ok := e.lookup(name)
		if !ok {
			return nil, fmt.Errorf("`%s` is not a constant, the members of an array of identifiers have to be constants", name)
		}
		literal, ok := copyAt(tree, node.Pos).(*ast.Literal)
		if !ok {
			return nil, fmt.Errorf("`%s` can't be a member of an array, it isn't a literal", name)
		}
		members = append(members, literal)
	}
	return &ast.Array{Meta: node.Meta, Members: members}, nil
}

// copyAt returns a copy of the tree, its root and its nodes without a position
// at the given position.
func copyAt(tree ast.Node, pos ast.Position) ast.Node {
	root := positionAt(ast.DeepCopy(tree), pos)
	root.Metadata().Pos = pos
	return root
}

// positionAt positions the nodes of the tree without a position at the given
// position.
func positionAt(node ast.Node, pos ast.Position) ast.Node {
	if node.Position() == (ast.Position{}) {
		node.Metadata().Pos = pos
	}
	ast.EditChildren(node, func(child ast.Node) ast.Node {
		return positionAt(child, pos)
	})
	return node
}
