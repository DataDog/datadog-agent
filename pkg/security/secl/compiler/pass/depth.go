// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pass

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// LimitDepth returns the pass rejecting a tree deeper than ast.MaxDepth. The
// parser rejects such a tree, but expanding the macros it refers to deepens it.
func LimitDepth() Pass {
	return func(node ast.Node) (ast.Node, error) {
		root := node
		switch n := node.(type) {
		case *ast.Rule:
			root = n.Root
		case *ast.Macro:
			root = n.Root
		}
		if _, ok := ast.TooDeep(root, ast.MaxDepth); ok {
			return nil, fmt.Errorf("expression too deep once its macros are expanded, its tree has more than %d levels", ast.MaxDepth)
		}
		return node, nil
	}
}
