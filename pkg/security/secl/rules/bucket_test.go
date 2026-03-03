// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// newTestRuleWithExecContextTag creates a *Rule with the given id and tag value
func newOrderTestRule(t *testing.T, id string, internalPolicyType InternalPolicyType, execContextTag string, priority int) *Rule {
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
				Priority:   priority,
			},
			Policy: PolicyInfo{
				InternalType: internalPolicyType,
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

	assertOrder := func(t *testing.T, bucket *RuleBucket, expected []string) {
		t.Helper()

		rules := bucket.GetRules()
		ids := make([]string, len(rules))
		for i, r := range rules {
			ids[i] = r.Def.ID
		}

		if !reflect.DeepEqual(ids, expected) {
			t.Errorf("unexpected rule order: got %v, want %v", ids, expected)
		}
	}

	t.Run("test1", func(t *testing.T) {
		bucket := &RuleBucket{}

		e1 := newOrderTestRule(t, "E1", DefaultPolicyType, "true", 0)
		e2 := newOrderTestRule(t, "E2", DefaultPolicyType, "True", 0)
		e3 := newOrderTestRule(t, "E3", DefaultPolicyType, "true", 0)
		n1 := newOrderTestRule(t, "N1", DefaultPolicyType, "false", 0)
		n2 := newOrderTestRule(t, "N2", DefaultPolicyType, "false", 0)

		// Add in mixed order
		for _, r := range []*Rule{e1, n2, n1, e2, e3} {
			if err := bucket.AddRule(r); err != nil {
				t.Error(err)
			}
		}

		assertOrder(t, bucket, []string{"E1", "E2", "E3", "N2", "N1"})
	})

	t.Run("test2", func(t *testing.T) {
		bucket := &RuleBucket{}

		e1 := newOrderTestRule(t, "E1", DefaultPolicyType, "true", 0)
		e2 := newOrderTestRule(t, "E2", DefaultPolicyType, "True", 99)
		e3 := newOrderTestRule(t, "E3", DefaultPolicyType, "true", 999)
		n1 := newOrderTestRule(t, "N1", DefaultPolicyType, "false", 4)
		n2 := newOrderTestRule(t, "N2", DefaultPolicyType, "false", 5)

		// Add in mixed order
		for _, r := range []*Rule{e1, n2, n1, e2, e3} {
			if err := bucket.AddRule(r); err != nil {
				t.Error(err)
			}
		}
		assertOrder(t, bucket, []string{"E3", "E2", "E1", "N2", "N1"})
	})

	t.Run("test3", func(t *testing.T) {
		bucket := &RuleBucket{}

		e1 := newOrderTestRule(t, "E1", CustomPolicyType, "true", 0)
		e2 := newOrderTestRule(t, "E2", DefaultPolicyType, "True", 99)
		e3 := newOrderTestRule(t, "E3", CustomPolicyType, "true", 999)
		n1 := newOrderTestRule(t, "N1", DefaultPolicyType, "false", 4)
		n2 := newOrderTestRule(t, "N2", DefaultPolicyType, "false", 5)

		// Add in mixed order
		for _, r := range []*Rule{e1, n2, n1, e2, e3} {
			if err := bucket.AddRule(r); err != nil {
				t.Error(err)
			}
		}
		assertOrder(t, bucket, []string{"E2", "E3", "E1", "N2", "N1"})
	})

	t.Run("test4", func(t *testing.T) {
		bucket := &RuleBucket{}

		e1 := newOrderTestRule(t, "E1", DefaultPolicyType, "true", 0)
		e2 := newOrderTestRule(t, "E2", DefaultPolicyType, "True", 0)
		e3 := newOrderTestRule(t, "E3", DefaultPolicyType, "true", 0)
		n1 := newOrderTestRule(t, "N1", DefaultPolicyType, "", 0)
		n2 := newOrderTestRule(t, "N2", DefaultPolicyType, "", 0)

		// Add in mixed order
		for _, r := range []*Rule{e1, n2, n1, e2, e3} {
			if err := bucket.AddRule(r); err != nil {
				t.Error(err)
			}
		}

		assertOrder(t, bucket, []string{"E1", "E2", "E3", "N2", "N1"})
	})

	t.Run("test5", func(t *testing.T) {
		bucket := &RuleBucket{}

		n1 := newOrderTestRule(t, "N1", CustomPolicyType, "", 500)
		n2 := newOrderTestRule(t, "N2", CustomPolicyType, "", 999)
		n3 := newOrderTestRule(t, "N3", DefaultPolicyType, "", 500)
		n4 := newOrderTestRule(t, "N4", DefaultPolicyType, "", 999)
		n5 := newOrderTestRule(t, "N5", DefaultPolicyType, "", 100)

		// Add in mixed order
		for _, r := range []*Rule{n1, n2, n3, n4, n5} {
			if err := bucket.AddRule(r); err != nil {
				t.Error(err)
			}
		}

		assertOrder(t, bucket, []string{"N4", "N3", "N5", "N2", "N1"})
	})
}
