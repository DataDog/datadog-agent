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

func newRuleSet() *RuleSet {
	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(testConstants)

	var opts Opts
	opts.
		WithSupportedDiscarders(testSupportedDiscarders).
		WithEventTypeEnabled(enabled)

	return NewRuleSet(&testModel{}, func() eval.Event { return &testEvent{} }, &opts, &evalOpts, &eval.MacroStore{})
}

func emptyReplCtx() eval.ReplacementContext {
	return eval.ReplacementContext{
		Opts:       &eval.Opts{},
		MacroStore: &eval.MacroStore{},
	}
}

func TestRuleBuckets(t *testing.T) {
	exprs := []string{
		`(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`,
		`(mkdir.filename =~ "/sbin/*" || mkdir.filename =~ "/usr/sbin/*") && process.uid != 0`,
	}

	rs := newRuleSet()
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

	rs := newRuleSet()
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

func TestRuleSetApprovers1(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.filename in ["/etc/passwd", "/etc/shadow"] && (process.uid == 0 && process.gid == 0)`)

	caps := FieldCapabilities{
		{
			Field:        "process.uid",
			Types:        eval.ScalarValueType,
			FilterWeight: 1,
		},
		{
			Field:        "process.gid",
			Types:        eval.ScalarValueType,
			FilterWeight: 2,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("should get an approver")
	}

	if values, exists := approvers["process.gid"]; !exists || len(values) != 1 {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["process.uid"]; exists {
		t.Fatal("unexpected approver found")
	}

	if _, exists := approvers["open.filename"]; exists {
		t.Fatal("unexpected approver found")
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	approvers, _ = rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("should get an approver")
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers2(t *testing.T) {
	exprs := []string{
		`open.filename in ["/etc/passwd", "/etc/shadow"] && process.uid == 0`,
		`open.flags & O_CREAT > 0 && process.uid == 0`,
	}

	rs := newRuleSet()
	addRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get any approver")
	}

	caps = FieldCapabilities{
		{
			Field:        "open.filename",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
		{
			Field:        "process.uid",
			Types:        eval.ScalarValueType,
			FilterWeight: 2,
		},
	}

	approvers, _ = rs.GetEventApprovers("open", caps)
	if len(approvers) != 2 {
		t.Fatal("should get 2 field approvers")
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %+v", values)
	}

	if _, exists := approvers["process.uid"]; !exists {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers3(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.filename in ["/etc/passwd", "/etc/shadow"] && (process.uid == process.gid)`)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatal("should get only one field approver")
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers4(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.filename =~ "/etc/passwd" && process.uid == 0`)

	caps := FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType,
		},
	}

	if approvers, _ := rs.GetEventApprovers("open", caps); len(approvers) != 0 {
		t.Fatalf("shouldn't get any approver, got: %+v", approvers)
	}

	caps = FieldCapabilities{
		{
			Field: "open.filename",
			Types: eval.ScalarValueType | eval.PatternValueType,
		},
	}

	if approvers, _ := rs.GetEventApprovers("open", caps); len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers5(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `(open.flags & O_CREAT > 0 || open.flags & O_EXCL > 0) && open.flags & O_RDWR > 0`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	for _, value := range approvers["open.flags"] {
		if value.Value.(int)&syscall.O_RDWR == 0 {
			t.Fatal("expected approver not found")
		}
	}
}

func TestRuleSetApprovers6(t *testing.T) {
	rs := newRuleSet()
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

	if approvers, _ := rs.GetEventApprovers("open", caps); len(approvers) == 0 {
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

	if approvers, _ := rs.GetEventApprovers("open", caps); len(approvers) > 0 {
		t.Fatal("shouldn't get any approver")
	}
}

func TestRuleSetApprovers7(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	if len(approvers["open.flags"]) != 1 || approvers["open.flags"][0].Value.(int)&syscall.O_CREAT == 0 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers8(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT && open.filename in ["/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType,
		},
		{
			Field:        "open.filename",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["open.flags"]; exists {
		t.Fatal("shouldn't get an approver for flags")
	}
}

func TestRuleSetApprovers9(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT && open.filename not in ["/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType,
		},
		{
			Field:        "open.filename",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["open.filename"]; exists {
		t.Fatal("shouldn't get an approver for filename")
	}

	if _, exists := approvers["open.flags"]; !exists {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers10(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.filename in [~"/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:        "open.filename",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for filename")
	}
}

func TestRuleSetApprovers11(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.filename in [~"/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:        "open.filename",
			Types:        eval.ScalarValueType | eval.PatternValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers12(t *testing.T) {
	exprs := []string{
		`open.filename in ["/etc/passwd", "/etc/shadow"]`,
		`open.filename in [~"/etc/httpd", "/etc/nginx"]`,
	}

	rs := newRuleSet()
	addRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.filename",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for filename")
	}
}

func TestRuleSetApprovers13(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_RDWR`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for filename")
	}
}

func TestGetRuleEventType(t *testing.T) {
	rule := &eval.Rule{
		ID:         "aaa",
		Expression: `open.filename == "test"`,
	}
	if err := rule.GenEvaluator(&testModel{}, emptyReplCtx()); err != nil {
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
