// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// newTestRuleWithExecContextTag creates a *Rule with the given id and tag value
func newTestRuleWithExecContextTag(t *testing.T, id string, execContextTag string) *Rule {
	expression := `open.file.path == "/tmp/test"`
	pc := ast.NewParsingContext(false)
	evalRule, err := eval.NewRule(id, expression, pc, &eval.Opts{})
	if err != nil {
		t.Error(err)
	}

	rule := &Rule{
		PolicyRule: &PolicyRule{
			Def: &RuleDefinition{
				ID:         id,
				Expression: expression,
				Tags:       map[string]string{},
			},
		},
		Rule: evalRule,
	}

	err = rule.GenEvaluator(&model.Model{})
	if err != nil {
		t.Error(err)
	}

	if execContextTag != "" {
		rule.Def.Tags[ExecutionContextTagName] = execContextTag
	}

	return rule
}

func TestRuleBucket_AddRule_Order(t *testing.T) {
	bucket := &RuleBucket{}

	// Create rules with unique fields
	e1 := newTestRuleWithExecContextTag(t, "E1", "true")
	e2 := newTestRuleWithExecContextTag(t, "E2", "true")
	e3 := newTestRuleWithExecContextTag(t, "E3", "true")
	n1 := newTestRuleWithExecContextTag(t, "N1", "false")
	n2 := newTestRuleWithExecContextTag(t, "N2", "false")

	// Add in mixed order
	for _, r := range []*Rule{e1, n2, n1, e2, e3} {
		if err := bucket.AddRule(r); err != nil {
			t.Error(err)
		}
	}

	rules := bucket.GetRules()
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.Def.ID
	}

	expected := []string{"E1", "E2", "E3", "N2", "N1"}
	if !reflect.DeepEqual(ids, expected) {
		t.Errorf("unexpected rule order: got %v, want %v", ids, expected)
	}
}
