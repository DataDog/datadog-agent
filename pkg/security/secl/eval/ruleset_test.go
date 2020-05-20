package eval

import (
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

type testHandler struct {
	discarders map[string][]string
}

func (f *testHandler) RuleMatch(rule *Rule, event Event) {

}

func (f *testHandler) EventDiscarderFound(event Event, field string) {
	fields, ok := f.discarders[event.GetType()]
	if !ok {
		fields = []string{}
	}
	fields = append(fields, field)
	f.discarders[event.GetType()] = fields
}

func (f *testHandler) EventApproverFound(event Event, field string) {
}

func addRuleExpr(t *testing.T, rs *RuleSet, expr string) {
	astRule, err := ast.ParseRule(expr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.AddRule("", astRule); err != nil {
		t.Fatal(err)
	}
}

func TestRuleBuckets(t *testing.T) {
	rs := NewRuleSet(&testModel{}, Opts{Debug: true, Constants: testConstants})
	addRuleExpr(t, rs, `(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`)
	addRuleExpr(t, rs, `(mkdir.filename =~ "/sbin/*" || mkdir.filename =~ "/usr/sbin/*") && process.uid != 0`)

	if bucket, ok := rs.eventRuleBuckets["open"]; !ok || len(bucket.rules) != 1 {
		t.Fatal("unable to find `open` rules or incorrect number of rules")
	}
	if bucket, ok := rs.eventRuleBuckets["mkdir"]; !ok || len(bucket.rules) != 1 {
		t.Fatal("unable to find `mkdir` rules or incorrect number of rules")
	}
}

func TestRuleSetEval(t *testing.T) {
	handler := &testHandler{
		discarders: make(map[string][]string),
	}
	rs := NewRuleSet(&testModel{}, Opts{Debug: true, Constants: testConstants})
	rs.AddListener(handler)

	addRuleExpr(t, rs, `(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`)
	addRuleExpr(t, rs, `(open.filename =~ "/var/run/*") && open.flags & O_CREAT > 0`)
	addRuleExpr(t, rs, `open.flags & O_CREAT > 0`)
	addRuleExpr(t, rs, `mkdir.filename =~ "/etc/*" && process.uid != 0`)

	event := &testEvent{
		process: testProcess{
			name:   "abc",
			uid:    0,
			isRoot: true,
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

	if fields, ok := handler.discarders["open"]; !ok || len(fields) != 1 {
		t.Fatalf("unable to find a discarder for `open` or bad number of fields: %v", fields)
	}
	if fields, ok := handler.discarders["mkdir"]; !ok || len(fields) != 2 {
		t.Fatalf("unable to find a discarder for `mkdir` or bad number of fields: %v", fields)
	}
}
