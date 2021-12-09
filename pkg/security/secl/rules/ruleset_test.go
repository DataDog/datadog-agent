// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

type testFieldValues map[string][]interface{}

type testHandler struct {
	model   *testModel
	filters map[string]testFieldValues
}

func (f *testHandler) RuleMatch(rule *Rule, event eval.Event) {
}

func (f *testHandler) EventDiscarderFound(rs *RuleSet, event eval.Event, field string, eventType eval.EventType) {
	values, ok := f.filters[event.GetType()]
	if !ok {
		values = make(testFieldValues)
		f.filters[event.GetType()] = values
	}

	discarders, ok := values[field]
	if !ok {
		discarders = []interface{}{}
	}
	evaluator, _ := f.model.GetEvaluator(field, "")

	ctx := eval.NewContext(event.GetPointer())

	value := evaluator.Eval(ctx)

	found := false
	for _, d := range discarders {
		if d == value {
			found = true
		}
	}

	if !found {
		discarders = append(discarders, evaluator.Eval(ctx))
	}
	values[field] = discarders
}

func addRuleExpr(t *testing.T, rs *RuleSet, exprs ...string) {
	var ruleDefs []*RuleDefinition

	for i, expr := range exprs {
		ruleDef := &RuleDefinition{
			ID:         fmt.Sprintf("ID%d", i),
			Expression: expr,
			Tags:       make(map[string]string),
		}
		ruleDefs = append(ruleDefs, ruleDef)
	}

	if err := rs.AddRules(ruleDefs); err != nil {
		t.Fatal(err)
	}
}

func TestRuleBuckets(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	exprs := []string{
		`(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`,
		`(mkdir.filename =~ "/sbin/*" || mkdir.filename =~ "/usr/sbin/*") && process.uid != 0`,
	}

	addRuleExpr(t, rs, exprs...)

	if bucket, ok := rs.eventRuleBuckets["open"]; !ok || len(bucket.rules) != 1 {
		t.Fatal("unable to find `open` rules or incorrect number of rules")
	}
	if bucket, ok := rs.eventRuleBuckets["mkdir"]; !ok || len(bucket.rules) != 1 {
		t.Fatal("unable to find `mkdir` rules or incorrect number of rules")
	}
	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.rules {
			if rule.GetPartialEval("process.uid") == nil {
				t.Fatal("failed to initialize partials")
			}
		}
	}
}

func TestRuleSetDiscarders(t *testing.T) {
	m := &testModel{}

	handler := &testHandler{
		model:   m,
		filters: make(map[string]testFieldValues),
	}

	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(m, func() eval.Event { return &testEvent{} }, &opts)
	rs.AddListener(handler)

	exprs := []string{
		`open.filename == "/etc/passwd" && process.uid != 0`,
		`(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`,
		`(open.filename =~ "/var/run/*") && open.flags & O_CREAT > 0 && process.uid != 0`,
		`(mkdir.filename =~ "/var/run/*") && process.uid != 0`,
	}

	addRuleExpr(t, rs, exprs...)

	event := &testEvent{
		process: testProcess{
			uid: 0,
		},
	}

	ev1 := *event
	ev1.kind = "open"
	ev1.open = testOpen{
		filename: "/usr/local/bin/rootkit",
		flags:    syscall.O_RDONLY,
	}

	ev2 := *event
	ev2.kind = "mkdir"
	ev2.mkdir = testMkdir{
		filename: "/usr/local/bin/rootkit",
		mode:     0777,
	}

	rs.Evaluate(&ev1)
	rs.Evaluate(&ev2)

	expected := map[string]testFieldValues{
		"open": {
			"open.filename": []interface{}{
				"/usr/local/bin/rootkit",
			},
			"process.uid": []interface{}{
				0,
			},
		},
		"mkdir": {
			"mkdir.filename": []interface{}{
				"/usr/local/bin/rootkit",
			},
			"process.uid": []interface{}{
				0,
			},
		},
	}

	if !reflect.DeepEqual(expected, handler.filters) {
		t.Fatalf("unable to find expected discarders, expected: `%v`, got: `%v`", expected, handler.filters)
	}
}

func TestRuleSetFilters1(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addRuleExpr(t, rs, `open.filename in ["/etc/passwd", "/etc/shadow"] && (process.uid == 0 || process.gid == 0)`)

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

	approvers, err := rs.GetEventApprovers("open", caps)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := approvers["process.uid"]; !exists {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["process.gid"]; !exists {
		t.Fatal("expected approver not found")
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	approvers, err = rs.GetEventApprovers("open", caps)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}

	caps = FieldCapabilities{
		{
			Field: "process.uid",
			Types: eval.ScalarValueType,
		},
	}

	_, err = rs.GetEventApprovers("open", caps)
	if err == nil {
		t.Fatal("shouldn't get any approver")
	}
}

func TestRuleSetFilters2(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	exprs := []string{
		`open.filename in ["/etc/passwd", "/etc/shadow"] && process.uid == 0`,
		`open.flags & O_CREAT > 0 && (process.uid == 0 || process.gid == 0)`,
	}

	addRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	_, err := rs.GetEventApprovers("open", caps)
	if err == nil {
		t.Fatal("shouldn't get any approver")
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
		{
			Field: "process.uid",
			Types: eval.ScalarValueType,
		},
		{
			Field: "process.gid",
			Types: eval.ScalarValueType,
		},
	}

	approvers, err := rs.GetEventApprovers("open", caps)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["process.uid"]; !exists {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["process.gid"]; !exists {
		t.Fatal("expected approver not found")
	}

}

func TestRuleSetFilters3(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addRuleExpr(t, rs, `open.filename in ["/etc/passwd", "/etc/shadow"] && (process.uid == process.gid)`)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	approvers, err := rs.GetEventApprovers("open", caps)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}

	if len(approvers) != 1 {
		t.Fatal("should get only one approver")
	}
}

func TestRuleSetFilters4(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addRuleExpr(t, rs, `open.filename =~ "/etc/passwd" && process.uid == 0`)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	if approvers, err := rs.GetEventApprovers("open", caps); err == nil {
		t.Fatalf("shouldn't get any approver, got: %+v", approvers)
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType | eval.PatternValueType,
		},
	}

	if _, err := rs.GetEventApprovers("open", caps); err != nil {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetFilters5(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addRuleExpr(t, rs, `(open.flags & O_CREAT > 0 || open.flags & O_EXCL > 0) && open.flags & O_RDWR > 0`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType | eval.BitmaskValueType,
		},
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	if _, err := rs.GetEventApprovers("open", caps); err != nil {
		t.Fatal("expected approver not found")
	}
}

// TODO: re-add this test once approver on multiple event type rules will be fixed
func TestRuleSetFilters6(t *testing.T) {
	t.Skip()

	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addRuleExpr(t, rs, `(open.flags & O_CREAT > 0 || open.flags & O_EXCL > 0) || process.name == "httpd"`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	if _, err := rs.GetEventApprovers("open", caps); err == nil {
		t.Fatal("shouldn't get any approver")
	}
}

func TestRuleSetFilters7(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts Opts
	opts.
		WithConstants(testConstants).
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	rs := NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts)

	addRuleExpr(t, rs, `open.filename == "123456"`)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
			ValidateFnc: func(value FilterValue) bool {
				return strings.HasSuffix(value.Value.(string), "456")
			},
		},
	}

	if _, err := rs.GetEventApprovers("open", caps); err != nil {
		t.Fatal("expected approver not found")
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
			ValidateFnc: func(value FilterValue) bool {
				return strings.HasSuffix(value.Value.(string), "777")
			},
		},
	}

	if _, err := rs.GetEventApprovers("open", caps); err == nil {
		t.Fatal("shouldn't get any approver")
	}
}

func TestGetRuleEventType(t *testing.T) {
	rule := &eval.Rule{
		ID:         "aaa",
		Expression: `open.filename == "test"`,
	}
	if err := rule.GenEvaluator(&testModel{}, &eval.Opts{}); err != nil {
		t.Fatal(err)
	}

	eventType, err := GetRuleEventType(rule)
	if err != nil {
		t.Fatalf("should get an event type: %s", err)
	}

	event := &testEvent{}
	fieldEventType, err := event.GetFieldEventType("open.filename")
	if err != nil {
		t.Fatal("should get a field event type")
	}

	if eventType != fieldEventType {
		t.Fatal("unexpected event type")
	}
}
