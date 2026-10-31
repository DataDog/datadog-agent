// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language.
package parser

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// ParsingContext parses the expressions of a rule set, and optionally caches
// the trees of its rules: the same expression is parsed once, however many
// rules use it.
//
// A cached tree is shared by every rule built from the same expression, which
// is safe since the compilation doesn't mutate the tree.
//
// A ParsingContext with a cache isn't safe for concurrent use.
type ParsingContext struct {
	ruleCache map[string]*ast.Rule
}

// NewParsingContext returns a new parsing context, caching the trees of the
// rules if withRuleCache is set.
func NewParsingContext(withRuleCache bool) *ParsingContext {
	var ruleCache map[string]*ast.Rule
	if withRuleCache {
		ruleCache = make(map[string]*ast.Rule)
	}

	return &ParsingContext{ruleCache: ruleCache}
}

// BuildRule parses the expression of a rule, see ParseRule.
func (pc *ParsingContext) BuildRule(expr string) (*ast.Rule, error) {
	if rule, ok := pc.ruleCache[expr]; ok {
		return rule, nil
	}

	rule, err := ParseRule(expr)
	if err != nil {
		return nil, err
	}

	if pc.ruleCache != nil {
		pc.ruleCache[expr] = rule
	}

	return rule, nil
}

// BuildMacro parses the expression of a macro, see ParseMacro.
func (pc *ParsingContext) BuildMacro(expr string) (*ast.Macro, error) {
	return ParseMacro(expr)
}

// BuildExpression parses an expression, see ParseExpression.
func (pc *ParsingContext) BuildExpression(expr string) (ast.Node, error) {
	return ParseExpression(expr)
}
