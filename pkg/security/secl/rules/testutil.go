// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// AddTestRuleExpr adds a rule expression
func AddTestRuleExpr(t testing.TB, rs *RuleSet, exprs ...string) {
	var ruleDefs []*RuleDefinition

	for i, expr := range exprs {
		ruleDef := &RuleDefinition{
			ID:         fmt.Sprintf("ID%d", i),
			Expression: expr,
			Tags:       make(map[string]string),
		}
		ruleDefs = append(ruleDefs, ruleDef)
	}

	pc := ast.NewParsingContext()

	if err := rs.AddRules(pc, ruleDefs); err != nil {
		t.Fatal(err)
	}
}
