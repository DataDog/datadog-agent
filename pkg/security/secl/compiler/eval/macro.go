// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/pass"
)

// MacroID - ID of a Macro
type MacroID = string

// Macro - Macro object identified by an `ID` containing a SECL `Expression`
type Macro struct {
	ID   MacroID
	Opts *Opts

	// ast is the tree of the macro, the compilation passes run over it: the
	// tree expanded where the macro is referred to.
	ast *ast.Macro
}

// NewMacro parses an expression and returns a new macro, the compilation passes
// run over its tree.
func NewMacro(id, expression string, model Model, parsingContext *parser.ParsingContext, opts *Opts) (*Macro, error) {
	astMacro, err := parsingContext.BuildMacro(id, expression)
	if err != nil {
		return nil, fmt.Errorf("syntax error: %w", err)
	}

	macro := &Macro{
		ID:   id,
		Opts: opts,
		ast:  pass.Run(astMacro, opts.passes()...).(*ast.Macro),
	}

	// compiled only to report its errors where it's defined, each rule using it
	// compiles its own copy
	state := NewState(model, "")
	if _, _, err := compileExpr(macro.ast.Root, opts, state); err != nil {
		if err, ok := err.(*ErrAstToEval); ok && err.Pos.Source == id {
			return nil, fmt.Errorf("compilation error: macro syntax error: %w", &ErrRuleParse{pos: err.Pos, expr: expression})
		}
		return nil, fmt.Errorf("compilation error: macro compilation error: %w", err)
	}
	if _, err := EventTypeFromState(model, state); err != nil {
		return nil, fmt.Errorf("compilation error: macro compilation error: %w", err)
	}

	return macro, nil
}

// NewStringValuesMacro returns a new macro from an array of strings
func NewStringValuesMacro(id string, values []string, opts *Opts) (*Macro, error) {
	pos := ast.At(ast.Position{Source: id})
	array := &ast.Array{Meta: pos}
	for _, value := range values {
		array.Members = append(array.Members, &ast.Literal{Meta: pos, Value: ast.MakeString(value)})
	}

	return &Macro{
		ID:   id,
		Opts: opts,
		ast:  &ast.Macro{Root: array},
	}, nil
}
