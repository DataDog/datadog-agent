package eval

import (
	"syscall"
	"testing"
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

func TestRuleBuckets(t *testing.T) {
	rs := NewRuleSet(&testModel{}, true)
	if _, err := rs.AddRule("", `(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.AddRule("", `(mkdir.filename =~ "/sbin/*" || mkdir.filename =~ "/usr/sbin/*") && process.uid != 0`); err != nil {
		t.Fatal(err)
	}

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
	rs := NewRuleSet(&testModel{}, true)
	rs.AddListener(handler)

	addRule := func(expr string) {
		if _, err := rs.AddRule("", expr); err != nil {
			t.Fatal(err)
		}
	}

	addRule(`(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`)
	addRule(`(open.filename =~ "/var/run/*") && open.flags & O_CREAT > 0`)
	addRule(`true && open.flags & O_CREAT > 0`)
	addRule(`mkdir.filename =~ "/etc/*" && process.uid != 0`)

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
	//rs.Evaluate(&ev2)

	if fields, ok := handler.discarders["open"]; !ok || len(fields) != 3 {
		t.Fatalf("unable to find a discarder for `open` or bad number of fields: %v", fields)
	}
	if fields, ok := handler.discarders["mkdir"]; !ok || len(fields) != 2 {
		t.Fatalf("unable to find a discarder for `mkdir` or bad number of fields: %v", fields)
	}
}
