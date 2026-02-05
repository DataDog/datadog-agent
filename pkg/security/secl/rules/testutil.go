// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package rules holds rules related files
package rules

import (
	"fmt"
	"testing"

	"go.uber.org/atomic"
)

var ruleID = atomic.NewInt32(0)

// AddTestRuleExpr adds a rule expression
func AddTestRuleExpr(t testing.TB, rs *RuleSet, exprs ...string) {
	t.Helper()

	var rules []*PolicyRule

	for _, expr := range exprs {
		rule := &PolicyRule{
			Def: &RuleDefinition{
				ID:         fmt.Sprintf("ID%d", ruleID.Load()),
				Expression: expr,
				Tags:       make(map[string]string),
			},
		}
		rules = append(rules, rule)
		ruleID.Inc()
	}

	if err := rs.AddRules(rules); err != nil {
		t.Fatal(err)
	}
}
