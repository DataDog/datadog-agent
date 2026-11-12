// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pass

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
)

func TestFoldBools(t *testing.T) {
	for expr, expected := range map[string]string{
		`true && open.file.name == "a"`:                         `(== open.file.name "a")`,
		`open.file.name == "a" && true`:                         `(== open.file.name "a")`,
		`false && open.file.name == "a"`:                        `false`,
		`open.file.name == "a" || false`:                        `(== open.file.name "a")`,
		`open.file.name == "a" || true`:                         `true`,
		`!false`:                                                `true`,
		`!(true && false)`:                                      `true`,
		`!(open.file.name == "a" && false) && process.uid == 0`: `(== process.uid 0)`,
		`(true || process.uid == 0) && open.file.name == "a"`:   `(== open.file.name "a")`,

		// only the boolean operations are folded
		`open.file.name == "a" && process.uid == 0`: `(&& (== open.file.name "a") (== process.uid 0))`,
		`!!(open.file.name == "a")`:                 `(! (! (== open.file.name "a")))`,
		`process.is_root == true`:                   `(== process.is_root true)`,
		`1 == 1 && open.file.name == "a"`:           `(&& (== 1 1) (== open.file.name "a"))`,
	} {
		t.Run(expr, func(t *testing.T) {
			rule, err := parser.ParseRule(expr)
			require.NoError(t, err)

			folded, err := Run(rule, ResolveIdents(testLookup), FoldBools())
			require.NoError(t, err)
			require.Equal(t, expected, ast.Sprint(folded))
		})
	}
}

// An absorbed operation is positioned at the operation, a folded negation at the
// negation.
func TestFoldBoolsPositions(t *testing.T) {
	for expr, expected := range map[string]ast.Position{
		`process.uid == 0 || true`:   {Offset: 0, Line: 1, Column: 1},
		`process.is_root == !true`:   {Offset: 19, Line: 1, Column: 20},
		`process.is_root == (!true)`: {Offset: 20, Line: 1, Column: 21},
	} {
		t.Run(expr, func(t *testing.T) {
			rule, err := parser.ParseRule(expr)
			require.NoError(t, err)

			folded, err := Run(rule, ResolveIdents(testLookup), FoldBools())
			require.NoError(t, err)
			root := folded.(*ast.Rule).Root
			if compare, ok := root.(*ast.ScalarCompareExpr); ok {
				root = compare.Right
			}
			require.Equal(t, expected, root.Position())
		})
	}
}
