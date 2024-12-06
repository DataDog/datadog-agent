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
	t.Helper()

	var rules []*PolicyRule

	for i, expr := range exprs {
		rule := &PolicyRule{
			Def: &RuleDefinition{
				ID:         fmt.Sprintf("ID%d", i),
				Expression: expr,
				Tags:       make(map[string]string),
			},
		}
		rules = append(rules, rule)
	}

	pc := ast.NewParsingContext(false)

	if err := rs.AddRules(pc, rules); err != nil {
		t.Fatal(err)
	}
}
