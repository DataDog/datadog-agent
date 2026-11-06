// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// ParsingContext parses the expressions of a rule set, optionally caching the
// trees of its rules. Sharing a cached tree between rules is safe since the
// compilation passes run over a copy of it. A ParsingContext with a cache isn't
// safe for concurrent use.
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
func (pc *ParsingContext) BuildMacro(id, expr string) (*ast.Macro, error) {
	return ParseMacro(id, expr)
}

// BuildExpression parses an expression, see ParseExpression.
func (pc *ParsingContext) BuildExpression(expr string) (ast.Node, error) {
	return ParseExpression(expr)
}
