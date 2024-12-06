// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"math"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type testFieldValues map[eval.Field][]interface{}

type testHandler struct {
	filters map[eval.EventType]testFieldValues
}

func (f *testHandler) Reset() {
	f.filters = make(map[eval.EventType]testFieldValues)
}

func (f *testHandler) RuleMatch(_ *Rule, _ eval.Event) bool {
	return true
}

func (f *testHandler) EventDiscarderFound(_ *RuleSet, event eval.Event, field eval.Field, _ eval.EventType) {
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

func newFakeEvent() eval.Event {
	return model.NewFakeEvent()
}

func newRuleSet() *RuleSet {
	ruleOpts, evalOpts := NewBothOpts(map[eval.EventType]bool{"*": true})
	return NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
}

func TestRuleBuckets(t *testing.T) {
	exprs := []string{
		`(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`,
		`(mkdir.filename =~ "/sbin/*" || mkdir.filename =~ "/usr/sbin/*") && process.uid != 0`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

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

	AddTestRuleExpr(t, rs, exprs...)

	ev1 := model.NewFakeEvent()
	ev1.Type = uint32(model.FileOpenEventType)
	ev1.SetFieldValue("open.file.path", "/usr/local/bin/rootkit")
	ev1.SetFieldValue("open.flags", syscall.O_RDONLY)
	ev1.SetFieldValue("process.uid", 0)

	ev2 := model.NewFakeEvent()
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
	AddTestRuleExpr(t, rs, `open.file.path in ["/etc/passwd", "/etc/shadow"] && (process.uid == 0 && process.gid == 0)`)

	caps := FieldCapabilities{
		{
			Field:        "process.uid",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 1,
		},
		{
			Field:        "process.gid",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 2,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
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
			Field:       "open.file.path",
			TypeBitmask: eval.ScalarValueType,
		},
	}

	approvers, _ = rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:       "open.file.path",
			TypeBitmask: eval.ScalarValueType,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get any approver")
	}

	caps = FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 3,
		},
		{
			Field:        "process.uid",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 2,
		},
	}

	approvers, _ = rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, `open.file.path in ["/etc/passwd", "/etc/shadow"] && (process.uid == process.gid)`)

	caps := FieldCapabilities{
		{
			Field:       "open.file.path",
			TypeBitmask: eval.ScalarValueType,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatal("should get only one field approver")
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers4(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `open.file.path =~ "/etc/passwd" && process.uid == 0`)

	caps := FieldCapabilities{
		{
			Field:       "open.file.path",
			TypeBitmask: eval.ScalarValueType,
		},
	}

	if approvers, _ := rs.GetEventTypeApprovers("open", caps); len(approvers) != 0 {
		t.Fatalf("shouldn't get any approver, got: %+v", approvers)
	}

	caps = FieldCapabilities{
		{
			Field:       "open.file.path",
			TypeBitmask: eval.ScalarValueType | eval.GlobValueType,
		},
	}

	if approvers, _ := rs.GetEventTypeApprovers("open", caps); len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers5(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `(open.flags & O_CREAT > 0 || open.flags & O_EXCL > 0) && open.flags & O_RDWR > 0`)

	caps := FieldCapabilities{
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, `open.file.name == "123456"`)

	caps := FieldCapabilities{
		{
			Field:       "open.file.name",
			TypeBitmask: eval.ScalarValueType,
			ValidateFnc: func(value FilterValue) bool {
				return strings.HasSuffix(value.Value.(string), "456")
			},
		},
	}

	if approvers, _ := rs.GetEventTypeApprovers("open", caps); len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	caps = FieldCapabilities{
		{
			Field:       "open.file.name",
			TypeBitmask: eval.ScalarValueType,
			ValidateFnc: func(value FilterValue) bool {
				return strings.HasSuffix(value.Value.(string), "777")
			},
		},
	}

	if approvers, _ := rs.GetEventTypeApprovers("open", caps); len(approvers) > 0 {
		t.Fatal("shouldn't get any approver")
	}
}

func TestRuleSetApprovers7(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT`)

	caps := FieldCapabilities{
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) == 0 {
		t.Fatal("expected approver not found")
	}

	if len(approvers["open.flags"]) != 1 || approvers["open.flags"][0].Value.(int)&syscall.O_CREAT == 0 {
		t.Fatal("expected approver not found")
	}
}

func TestRuleSetApprovers8(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT && open.file.path in ["/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType,
		},
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_CREAT && open.file.path not in ["/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, `open.file.path in [~"/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for `open.file.path`")
	}
}

func TestRuleSetApprovers11(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `open.file.path in [~"/etc/passwd", "/etc/shadow"]`)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver for `open.file.path`")
	}
}

func TestRuleSetApprovers13(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `open.flags & (O_CREAT | O_EXCL) == O_RDWR`)

	caps := FieldCapabilities{
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
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
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 || len(approvers["open.file.path"]) != 2 {
		t.Fatalf("should get an approver for `open.file.path`: %v", approvers)
	}
}

func TestRuleSetApprovers15(t *testing.T) {
	exprs := []string{
		`open.file.name =~ "*.dll"`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.name",
			TypeBitmask:  eval.ScalarValueType | eval.PatternValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 || len(approvers["open.file.name"]) != 1 {
		t.Fatalf("should get an approver for `open.file.name`: %v", approvers)
	}
}

func TestRuleSetApprovers16(t *testing.T) {
	exprs := []string{
		`open.file.path == "/etc/httpd.conf"`,
		`open.file.path != "" && open.retval == -1 && process.auid == 1000`,
	}

	handler := &testHandler{
		filters: make(map[string]testFieldValues),
	}

	rs := newRuleSet()
	rs.AddListener(handler)

	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.PatternValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver")
	}

	caps = FieldCapabilities{
		{
			Field:        "open.file.oath",
			TypeBitmask:  eval.ScalarValueType | eval.PatternValueType,
			FilterWeight: 3,
		},
		{
			Field:       "process.auid",
			TypeBitmask: eval.ScalarValueType,
		},
	}

	approvers, _ = rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver")
	}

	// shouldn't generate a discarder
	ev := model.NewFakeEvent()
	ev.Type = uint32(model.FileOpenEventType)
	ev.SetFieldValue("open.file.path", "/usr/local/bin/rootkit")

	if !rs.Evaluate(ev) {
		rs.EvaluateDiscarders(ev)
	}

	if _, ok := handler.filters["open.file.path"]; ok {
		t.Fatalf("shouldn't have a discarder for `open.file.path`")
	}

	// change the approver mode, now should have an approver + a discader
	caps = FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.PatternValueType,
			FilterWeight: 3,
		},
		{
			Field:       "process.auid",
			TypeBitmask: eval.ScalarValueType,
			FilterMode:  ApproverOnlyMode,
		},
	}

	approvers, _ = rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 2 || len(approvers["open.file.path"]) != 1 || len(approvers["process.auid"]) != 1 {
		t.Fatalf("should get an approver`: %v", approvers)
	}

	if !rs.Evaluate(ev) {
		rs.EvaluateDiscarders(ev)
	}

	if _, ok := handler.filters["open"]; !ok {
		t.Fatalf("should have a discarder for `open.file.path`: %v", handler.filters)
	}
}

func TestRuleSetApprovers17(t *testing.T) {
	exprs := []string{
		`open.file.path in ["/etc/passwd", "/etc/shadow"] && open.file.path != ~"/var/*"`,
		`open.file.path == "/var/lib/httpd"`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 || len(approvers["open.file.path"]) != 3 {
		t.Fatalf("should get an approver for `open.file.path`: %v", approvers)
	}
}

func TestRuleSetApprovers18(t *testing.T) {
	exprs := []string{
		`open.file.path in ["/etc/passwd", "/etc/shadow"] && open.file.path != ~"/var/*"`,
		`open.flags == O_RDONLY`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 2 || len(approvers["open.file.path"]) != 2 || len(approvers["open.flags"]) != 1 {
		t.Fatalf("should get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers19(t *testing.T) {
	exprs := []string{
		`open.file.path in ["/etc/passwd", "/etc/shadow"] && open.file.path != ~"/var/*"`,
		`open.flags == O_RDONLY`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatal("shouldn't get an approver")
	}
}

func TestRuleSetApprovers20(t *testing.T) {
	exprs := []string{
		`open.file.path in ["/etc/passwd", "/etc/shadow"] && open.file.path != ~"/var/*"`,
		`unlink.file.name == "test"`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 || len(approvers["open.file.path"]) != 2 {
		t.Fatalf("should get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers21(t *testing.T) {
	exprs := []string{
		`open.flags&1 > 0 || open.flags&2 > 0`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.flags",
			TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatalf("should get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers22(t *testing.T) {
	exprs := []string{
		`open.flags&1 > 0 || open.flags > 0`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.flags",
			TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatalf("shouldn't get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers23(t *testing.T) {
	exprs := []string{
		`open.flags&1 > 0 && open.flags > 0`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.flags",
			TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatalf("should get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers24(t *testing.T) {
	exprs := []string{
		`open.flags&1 > 0 && open.flags > 2`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.flags",
			TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 0 {
		t.Fatalf("shouldn't get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers25(t *testing.T) {
	exprs := []string{
		`open.flags&(O_CREAT|O_WRONLY) == (O_CREAT|O_WRONLY)`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.flags",
			TypeBitmask:  eval.ScalarValueType | eval.BitmaskValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatalf("should get approvers: %v", approvers)
	}
}

func TestRuleSetApprovers26(t *testing.T) {
	exprs := []string{
		`open.file.path in [~"/proc/*/mem"] && open.file.path not in ["/proc/${process.pid}/mem", "/proc/self/mem"]`,
	}

	rs := newRuleSet()
	AddTestRuleExpr(t, rs, exprs...)

	caps := FieldCapabilities{
		{
			Field:        "open.file.path",
			TypeBitmask:  eval.ScalarValueType | eval.GlobValueType,
			FilterWeight: 3,
		},
	}

	approvers, _ := rs.GetEventTypeApprovers("open", caps)
	if len(approvers) != 1 {
		t.Fatalf("should get approvers: %v", approvers)
	}
}

func TestRuleSetAUDApprovers(t *testing.T) {
	caps := FieldCapabilities{
		{
			Field:       "open.file.path",
			TypeBitmask: eval.ScalarValueType | eval.PatternValueType,
		},
		{
			Field:       "open.flags",
			TypeBitmask: eval.ScalarValueType | eval.BitmaskValueType,
		},
		{
			Field:            "process.auid",
			TypeBitmask:      eval.ScalarValueType | eval.RangeValueType,
			FilterMode:       ApproverOnlyMode,
			RangeFilterValue: &RangeFilterValue{Min: 0, Max: model.AuditUIDUnset - 1},
			FilterWeight:     10,
			HandleNotApproverValue: func(fieldValueType eval.FieldValueType, value interface{}) (eval.FieldValueType, interface{}, bool) {
				if fieldValueType != eval.ScalarValueType {
					return fieldValueType, value, false
				}

				if i, ok := value.(int); ok && uint32(i) == model.AuditUIDUnset {
					return eval.RangeValueType, RangeFilterValue{Min: 0, Max: model.AuditUIDUnset - 1}, true
				}

				return fieldValueType, value, false
			},
		},
	}

	getApprovers := func(exprs []string) Approvers {
		handler := &testHandler{
			filters: make(map[string]testFieldValues),
		}

		rs := newRuleSet()
		rs.AddListener(handler)

		AddTestRuleExpr(t, rs, exprs...)

		approvers, _ := rs.GetEventTypeApprovers("open", caps)
		return approvers
	}

	t.Run("equal", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid == 1000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 1 || approvers["process.auid"][0].Value != 1000 {
			t.Fatalf("should get an approver`: %v", approvers)
		}
	})

	t.Run("not-equal", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid != 1000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 0 {
			t.Fatalf("shouldn't get an approver`: %v", approvers)
		}
	})

	t.Run("not-equal-unset", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid != AUDIT_AUID_UNSET`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 1 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 0 || rge.Max != model.AuditUIDUnset-1 {
			t.Fatalf("unexpected range")
		}
	})

	t.Run("lesser-equal-than", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid <= 1000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 1 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 0 || rge.Max != 1000 {
			t.Fatalf("unexpected range")
		}
	})

	t.Run("lesser-than", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid < 1000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 1 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 0 || rge.Max != 999 {
			t.Fatalf("unexpected range")
		}
	})

	t.Run("greater-equal-than", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid >= 1000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 1 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 1000 || rge.Max != math.MaxUint32-1 {
			t.Fatalf("unexpected range")
		}
	})

	t.Run("greater-than", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid > 1000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 1 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 1001 || rge.Max != math.MaxUint32-1 {
			t.Fatalf("unexpected range")
		}
	})

	t.Run("greater-equal-than-and", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid >= 1000 && process.auid != AUDIT_AUID_UNSET`,
			`open.flags&O_WRONLY > 0`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 2 || len(approvers["process.auid"]) != 2 && len(approvers["open.flags"]) != 1 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 1000 || rge.Max != math.MaxUint32-1 {
			t.Fatalf("unexpected range")
		}
	})

	t.Run("lesser-and-greater", func(t *testing.T) {
		exprs := []string{
			`open.file.path != "" && process.auid > 1000 && process.auid < 4000`,
		}

		approvers := getApprovers(exprs)
		if len(approvers) != 1 || len(approvers["process.auid"]) != 2 {
			t.Fatalf("should get an approver`: %v", approvers)
		}

		rge := approvers["process.auid"][0].Value.(RangeFilterValue)
		if rge.Min != 1001 || rge.Max != math.MaxUint32-1 {
			t.Fatalf("unexpected range")
		}

		rge = approvers["process.auid"][1].Value.(RangeFilterValue)
		if rge.Min != 0 || rge.Max != 3999 {
			t.Fatalf("unexpected range")
		}
	})
}

func TestGetRuleEventType(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		rule := eval.NewRule("aaa", `open.file.name == "test"`, &eval.Opts{})

		pc := ast.NewParsingContext(false)

		if err := rule.GenEvaluator(&model.Model{}, pc); err != nil {
			t.Fatal(err)
		}

		eventType, err := GetRuleEventType(rule)
		if err != nil {
			t.Fatalf("should get an event type: %s", err)
		}

		event := model.NewFakeEvent()
		fieldEventType, err := event.GetFieldEventType("open.file.name")
		if err != nil {
			t.Fatal("should get a field event type")
		}

		if eventType != fieldEventType {
			t.Fatal("unexpected event type")
		}
	})

	t.Run("ko", func(t *testing.T) {
		rule := eval.NewRule("aaa", `open.file.name == "test" && unlink.file.name == "123"`, &eval.Opts{})

		pc := ast.NewParsingContext(false)

		if err := rule.GenEvaluator(&model.Model{}, pc); err == nil {
			t.Fatalf("shouldn't get an evaluator, multiple event types: %s", err)
		}

		if _, err := GetRuleEventType(rule); err == nil {
			t.Fatalf("shouldn't get an event type: %s", err)
		}
	})
}
