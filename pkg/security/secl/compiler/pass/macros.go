// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pass holds the compilation passes of the SECL compiler.
package pass

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"

// ExpandMacros returns the pass replacing the identifiers referring to a macro
// by a copy of the tree of the macro. lookup returns the tree of a macro, its
// own macros already expanded.
func ExpandMacros(lookup func(name string) (*ast.Macro, bool)) Pass {
	e := identEditor{lookup: lookup}
	return e.edit
}

// identEditor replaces the identifiers referring to a macro.
type identEditor struct {
	lookup func(name string) (*ast.Macro, bool)
}

// edit returns the node, the identifiers of its tree referring to a macro
// replaced.
func (e identEditor) edit(node ast.Node) ast.Node {
	if ident, ok := node.(*ast.Ident); ok {
		// the tree of a macro is already expanded, it isn't walked
		if macro, ok := e.lookup(ident.Name); ok {
			// the root is positioned at the reference, its operands in the macro
			root := ast.DeepCopy(macro.Root)
			root.Metadata().Pos = ident.Pos
			return root
		}
	}
	ast.EditChildren(node, e.edit)
	return node
}
