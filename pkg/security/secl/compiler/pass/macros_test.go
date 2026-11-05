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

func TestExpandMacros(t *testing.T) {
	macros := map[string]*ast.Macro{}
	for id, expr := range map[string]string{
		"is_root": `process.uid == 0 && process.gid == 0`,
		"names":   `["a", "b"]`,
	} {
		macro, err := parser.ParseMacro(id, expr)
		require.NoError(t, err)
		macros[id] = macro
	}
	lookup := func(name string) (*ast.Macro, bool) {
		macro, ok := macros[name]
		return macro, ok
	}

	for expr, expected := range map[string]string{
		`is_root && open.flags == 1`: `(&& {is_root (&& (== process.uid 0) (== process.gid 0))} (== open.flags 1))`,
		`process.name in names`:      `(in process.name {names ["a" "b"]})`,
	} {
		rule, err := parser.ParseRule(expr)
		require.NoError(t, err)
		require.Equal(t, expected, ast.Sprint(Run(rule, ExpandMacros(lookup))))
	}
}
