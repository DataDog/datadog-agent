// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type testFieldValues map[string][]interface{}

type testHandler struct {
	filters map[string]testFieldValues
}

// SetRuleSetTag sets the value of the "ruleset" tag, which is the tag of the rules that belong in this rule set. This method is only used for testing.
func (rs *RuleSet) setRuleSetTagValue(value eval.RuleSetTagValue) error {
	if len(rs.GetRules()) > 0 {
		return ErrCannotChangeTagAfterLoading
	}
	if _, ok := rs.opts.RuleSetTag[RuleSetTagKey]; !ok {
		rs.opts.RuleSetTag = map[string]eval.RuleSetTagValue{RuleSetTagKey: ""}
	}
	rs.opts.RuleSetTag[RuleSetTagKey] = value

	return nil
}

func (f *testHandler) RuleMatch(_ *Rule, _ eval.Event) bool {
	return true
}

func (f *testHandler) EventDiscarderFound(_ *RuleSet, event eval.Event, field string, _ eval.EventType) {
	values, ok := f.filters[event.GetType()]
	if !ok {
		values = make(testFieldValues)
		f.filters[event.GetType()] = values
	}

	discarders, ok := values[field]
	if !ok {
		discarders = []interface{}{}
	}

	var m model.Model
	evaluator, _ := m.GetEvaluator(field, "")

	ctx := eval.NewContext(event)

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

	pc := ast.NewParsingContext()

	if err := rs.AddRules(pc, ruleDefs); err != nil {
		t.Fatal(err)
	}
}

func newDefaultEvent() eval.Event {
	return model.NewDefaultEvent()
}

func newRuleSet() *RuleSet {
	ruleOpts, evalOpts := NewEvalOpts(map[eval.EventType]bool{"*": true})
	return NewRuleSet(&model.Model{}, newDefaultEvent, ruleOpts, evalOpts)
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
	handler := &testHandler{
		filters: make(map[string]testFieldValues),
	}

	rs := newRuleSet()
	rs.AddListener(handler)

	exprs := []string{
		`open.file.path == "/etc/passwd" && process.uid != 0`,
		`(open.file.path =~ "/sbin/*" || open.file.path =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`,
		`(open.file.path =~ "/var/run/*") && open.flags & O_CREAT > 0 && process.uid != 0`,
		`(mkdir.file.path =~ "/var/run/*") && process.uid != 0`,
	}

	addRuleExpr(t, rs, exprs...)

	ev1 := model.NewDefaultEvent()
	ev1.Type = uint32(model.FileOpenEventType)
	ev1.SetFieldValue("open.file.path", "/usr/local/bin/rootkit")
	ev1.SetFieldValue("open.flags", syscall.O_RDONLY)
	ev1.SetFieldValue("process.uid", 0)

	ev2 := model.NewDefaultEvent()
	ev2.Type = uint32(model.FileMkdirEventType)
	ev2.SetFieldValue("mkdir.file.path", "/usr/local/bin/rootkit")
	ev2.SetFieldValue("mkdir.mode", 0777)
	ev2.SetFieldValue("process.uid", 0)

	if !rs.Evaluate(ev1) {
		rs.EvaluateDiscarders(ev1)
	}
	if !rs.Evaluate(ev2) {
		rs.EvaluateDiscarders(ev2)
	}

	expected := map[string]testFieldValues{
		"open": {
			"open.file.path": []interface{}{
				"/usr/local/bin/rootkit",
			},
			"process.uid": []interface{}{
				0,
			},
		},
		"mkdir": {
			"mkdir.file.path": []interface{}{
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
	addRuleExpr(t, rs, `open.file.path in ["/etc/passwd", "/etc/shadow"] && (process.uid == 0 && process.gid == 0)`)

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

	if _, exists := approvers["open.file.path"]; exists {
		t.Fatal("unexpected approver found")
	}

	caps = FieldCapabilities{
		{
			Field: "open.file.path",
			Types: eval.ScalarValueType,
		},
	}

	approvers, _ = rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("should get an approver")
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers2(t *testing.T) {
	exprs := []string{
		`open.file.path in ["/etc/passwd", "/etc/shadow"] && process.uid == 0`,
		`open.flags & O_CREAT > 0 && process.uid == 0`,
	}

	rs := newRuleSet()
	addRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field: "open.file.path",
			Types: eval.ScalarValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get any approver")
	}

	caps = FieldCapabilities{
		{
			Field:        "open.file.path",
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

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %+v", values)
	}

	if _, exists := approvers["process.uid"]; !exists {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers3(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.file.path in ["/etc/passwd", "/etc/shadow"] && (process.uid == process.gid)`)

	caps := FieldCapabilities{
		{
			Field: "open.file.path",
			Types: eval.ScalarValueType,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatal("should get only one field approver")
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers4(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.file.path =~ "/etc/passwd" && process.uid == 0`)

	caps := FieldCapabilities{
		{
			Field: "open.file.path",
			Types: eval.ScalarValueType,
		},
	}

	if approvers, _ := rs.GetEventApprovers("open", caps); len(approvers) != 0 {
		t.Fatalf("shouldn't get any approver, got: %+v", approvers)
	}

	caps = FieldCapabilities{
		{
			Field: "open.file.path",
			Types: eval.ScalarValueType | eval.GlobValueType,
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
	addRuleExpr(t, rs, `open.file.name == "123456"`)

	caps := FieldCapabilities{
		{
			Field: "open.file.name",
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
			Field: "open.file.name",
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
	addRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT && open.file.path in ["/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType,
		},
		{
			Field:        "open.file.path",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["open.flags"]; exists {
		t.Fatal("shouldn't get an approver for `open.flags`")
	}
}

func TestRuleSetApprovers9(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT && open.file.path not in ["/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field: "open.flags",
			Types: eval.ScalarValueType,
		},
		{
			Field:        "open.file.path",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	if _, exists := approvers["open.file.path"]; exists {
		t.Fatal("shouldn't get an approver for `open.file.path`")
	}

	if _, exists := approvers["open.flags"]; !exists {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers10(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.file.path in [~"/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for `open.file.path`")
	}
}

func TestRuleSetApprovers11(t *testing.T) {
	rs := newRuleSet()
	addRuleExpr(t, rs, `open.file.path in [~"/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			Types:        eval.ScalarValueType | eval.GlobValueType,
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
		`open.file.path in ["/etc/passwd", "/etc/shadow"]`,
		`open.file.path in [~"/etc/httpd", "/etc/nginx"]`,
	}

	rs := newRuleSet()
	addRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			Types:        eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for `open.file.path`")
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
		t.Fatal("shouldn't get an approver for `open.file.flags`")
	}
}

func TestRuleSetApprovers14(t *testing.T) {
	exprs := []string{
		`open.file.path == "/etc/passwd"`,
		`open.file.path =~ "/etc/*/httpd"`,
	}

	rs := newRuleSet()
	addRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			Types:        eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventApprovers("open", caps)
	if len(approvers) != 1 || len(approvers["open.file.path"]) != 2 {
		t.Fatalf("shouldn't get an approver for `open.file.path`: %v", approvers)
	}
}

func TestGetRuleEventType(t *testing.T) {
	rule := eval.NewRule("aaa", `open.file.name == "test"`, &eval.Opts{})

	pc := ast.NewParsingContext()

	if err := rule.GenEvaluator(&model.Model{}, pc); err != nil {
		t.Fatal(err)
	}

	eventType, err := GetRuleEventType(rule)
	if err != nil {
		t.Fatalf("should get an event type: %s", err)
	}

	event := model.NewDefaultEvent()
	fieldEventType, err := event.GetFieldEventType("open.file.name")
	if err != nil {
		t.Fatal("should get a field event type")
	}

	if eventType != fieldEventType {
		t.Fatal("unexpected event type")
	}
}
