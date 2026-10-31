// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pass holds the compilation passes of the SECL compiler.
//
// A pass turns an abstract syntax tree into another one. The passes run between
// the parse and the compilation of an expression:
//
//	source ──parse──▶ ast ──passes──▶ ast ──compile──▶ evaluators
//
// A pass owns the tree it is given and modifies it in place (see ast.EditChildren).
// Run hands the passes a copy, the tree of an expression being cached and shared
// by the rules using it.
package pass

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"

// Pass is a compilation pass.
type Pass func(ast.Node) (ast.Node, error)

// Run runs the passes, in order, over a copy of the tree. It stops at the first
// pass returning an error.
func Run(node ast.Node, passes ...Pass) (ast.Node, error) {
	node = ast.DeepCopy(node)
	for _, pass := range passes {
		var err error
		if node, err = pass(node); err != nil {
			return nil, err
		}
	}
	return node, nil
}
