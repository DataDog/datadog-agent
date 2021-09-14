// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func addProfileRuleExpr(t *testing.T, re *RuleEngine, selector string, exprs ...string) {
	var ruleDefs []*RuleDefinition

	for i, expr := range exprs {
		ruleDef := &RuleDefinition{
			ID:         fmt.Sprintf("ID%d", i),
			Expression: expr,
			Tags:       make(map[string]string),
		}
		ruleDefs = append(ruleDefs, ruleDef)
	}

	selectorRule := &eval.Rule{
		ID:         "selector",
		Expression: selector,
	}
	if err := selectorRule.Parse(); err != nil {
		t.Fatal(err)
	}

	ruleSet := re.newRuleSet()
	if err := selectorRule.GenEvaluator(ruleSet.model, &ruleSet.opts.Opts); err != nil {
		t.Fatal(err)
	}

	if err := ruleSet.AddRules(ruleDefs); err != nil {
		t.Fatal(err)
	}

	re.profiles[selectorRule] = ruleSet
}

func TestRuleEngineEvaluation(t *testing.T) {
	m := &testModel{}

	handler := &testHandler{
		model:   m,
		filters: make(map[string]testFieldValues),
	}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(map[eval.EventType]bool{"*": true})

	re := NewRuleEngine(m, func() eval.Event { return &testEvent{} }, &opts)
	re.AddListener(handler)

	policyExprs := []string{
		`open.filename in ["/etc/passwd", "/etc/shadow"] && process.uid != 0`,
	}

	profileExprs := []string{
		`open.filename == "/etc/passwd"`,
	}

	addPolicyRuleExpr(t, re, policyExprs...)
	addProfileRuleExpr(t, re, "true", profileExprs...)

	event := &testEvent{
		process: testProcess{
			uid: 0,
		},
	}

	ev1 := *event
	ev1.kind = "open"
	ev1.open = testOpen{
		filename: "/etc/passwd",
		flags:    syscall.O_RDONLY,
	}

	result := re.Evaluate(&ev1)

	if result {
		t.Fatal("expected event to either trigger policy rule or no profile rule")
	}

	values, ok := handler.filters["open"]
	if !ok || len(values) == 0 {
		t.Error("no discarder found for 'open'")
	}

	ev2 := ev1
	ev2.open.filename = "/etc/shadow"
	result = re.Evaluate(&ev2)

	if !result {
		t.Fatal("expected event to not trigger policy rule or match a profile rule")
	}

	values, ok = handler.filters["open"]
	if !ok || len(values) == 0 {
		t.Error("no discarder found for 'open'")
	}

	t.Logf("%+v", values)
}

func TestRuleEngineFilters(t *testing.T) {
	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(map[eval.EventType]bool{"*": true})

	re := NewRuleEngine(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addPolicyRuleExpr(t, re, `open.filename in ["/etc/passwd", "/etc/shadow"] && (process.uid == 0 || process.gid == 0)`)
	addProfileRuleExpr(t, re, "true", `open.filename in ["/etc/passwd"]`)

	caps := FieldCapabilities{
		{
			Field: "process.uid",
			Types: eval.ScalarValueType,
		},
		{
			Field: "process.gid",
			Types: eval.ScalarValueType,
		},
	}

	approvers, err := re.GetEventApprovers("open", caps)
	if err != nil {
		t.Errorf("failed to get approvers: %w", err)
	}

	if len(approvers["process.uid"]) > 0 {
		t.Errorf("unexpected approvers found: %+v", approvers)
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	approvers, err = re.GetEventApprovers("open", caps)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.filename"]; exists || len(values) != 0 {
		t.Fatalf("expected not approver to be found: %v", values)
	}
}
