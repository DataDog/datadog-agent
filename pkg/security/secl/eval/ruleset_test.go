package eval

import (
	"reflect"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

type testFieldValues map[string][]interface{}

type testHandler struct {
	model   *testModel
	filters map[string]testFieldValues
}

func (f *testHandler) RuleMatch(rule *Rule, event Event) {
}

func (f *testHandler) EventDiscarderFound(event Event, field string) {
	values, ok := f.filters[event.GetType()]
	if !ok {
		values = make(testFieldValues)
		f.filters[event.GetType()] = values
	}

	discarders, ok := values[field]
	if !ok {
		discarders = []interface{}{}
	}
	evaluator, _ := f.model.GetEvaluator(field)

	value := evaluator.(Evaluator).Value(&Context{})

	found := false
	for _, d := range discarders {
		if d == value {
			found = true
		}
	}

	if !found {
		discarders = append(discarders, evaluator.(Evaluator).Value(&Context{}))
	}
	values[field] = discarders
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
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})
	addRuleExpr(t, rs, `(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`)
	addRuleExpr(t, rs, `(mkdir.filename =~ "/sbin/*" || mkdir.filename =~ "/usr/sbin/*") && process.uid != 0`)

	if bucket, ok := rs.eventRuleBuckets["open"]; !ok || len(bucket.rules) != 1 {
		t.Fatal("unable to find `open` rules or incorrect number of rules")
	}
	if bucket, ok := rs.eventRuleBuckets["mkdir"]; !ok || len(bucket.rules) != 1 {
		t.Fatal("unable to find `mkdir` rules or incorrect number of rules")
	}
}

func TestRuleSetDiscarders(t *testing.T) {
	model := &testModel{}

	handler := &testHandler{
		model:   model,
		filters: make(map[string]testFieldValues),
	}
	rs := NewRuleSet(model, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})
	rs.AddListener(handler)

	addRuleExpr(t, rs, `open.filename == "/etc/passwd" && process.uid != 0`)
	addRuleExpr(t, rs, `(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && process.uid != 0 && open.flags & O_CREAT > 0`)
	addRuleExpr(t, rs, `(open.filename =~ "/var/run/*") && open.flags & O_CREAT > 0 && process.uid != 0`)
	addRuleExpr(t, rs, `(mkdir.filename =~ "/var/run/*") && process.uid != 0`)

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
		"open": testFieldValues{
			"open.filename": []interface{}{
				"/usr/local/bin/rootkit",
			},
			"process.uid": []interface{}{
				0,
			},
		},
		"mkdir": testFieldValues{
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

func TestRuleSetScalarFilters(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `(open.filename == "/etc/passwd" || open.filename == "/etc/shadow") && process.uid != 0`)
	addRuleExpr(t, rs, `open.flags & O_CREAT > 0 && process.uid != 0`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType,
		},
		{
			Field: "open.flags",
			Types: ScalarValueType,
		},
		{
			Field: "process.uid",
			Types: ScalarValueType,
		},
	}

	approvers, err := rs.GetEventFilters("open", capabilities...)
	if err != nil {
		t.Fatal(err)
	}

	if len(approvers["open.filename"]) != 2 {
		t.Fatalf("wrong number of approver: %+v", approvers)
	}
	if len(approvers["process.uid"]) != 1 {
		t.Fatalf("wrong number of approver: %+v", approvers)
	}
}

func TestRuleSetFilteringCapabilities(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename == "/etc/passwd" && process.uid != 0`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType,
		},
		{
			Field: "process.uid",
			Types: PatternValueType,
		},
	}

	_, err := rs.GetEventFilters("open", capabilities...)
	if err == nil {
		t.Fatal("should return an error because of capabilities mismatch")
	}

	if _, ok := err.(*CapabilityMismatch); !ok {
		t.Fatal("incorrect error type")
	}

	// add missing capability
	capabilities[1].Types |= ScalarValueType

	if _, err := rs.GetEventFilters("open", capabilities...); err != nil {
		t.Fatalf("should return approvers: %s", err)
	}
}

func TestRuleSetTwoFieldFilter(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename == "/etc/passwd" && process.uid == process.gid`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType,
		},
		{
			Field: "process.uid",
			Types: ScalarValueType,
		},
		{
			Field: "process.gid",
			Types: ScalarValueType,
		},
	}

	_, err := rs.GetEventFilters("open", capabilities...)
	if err == nil {
		t.Fatal("should return an error because of comparison of 2 field")
	}

	if _, ok := err.(*NoValue); !ok {
		t.Fatal("incorrect error type")
	}
}

func TestRuleSetPatternFilter(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename =~ "/etc/*" && process.uid != 0`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType | PatternValueType,
		},
		{
			Field: "process.uid",
			Types: ScalarValueType,
		},
	}

	if _, err := rs.GetEventFilters("open", capabilities...); err != nil {
		t.Fatalf("should return approvers: %s", err)
	}
}

func TestRuleSetNotFilter(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename !~ "/etc/*" && process.uid != 0`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType | PatternValueType,
		},
		{
			Field: "process.uid",
			Types: ScalarValueType,
		},
	}

	filters, err := rs.GetEventFilters("open", capabilities...)
	if err != nil {
		t.Fatalf("should return filters: %s", err)
	}

	if !filters["open.filename"][0].Not {
		t.Fatal("filename should be a not filters")
	}

	if !filters["process.uid"][0].Not {
		t.Fatal("filename should be a not filter")
	}
}

func TestRuleSetInFilter(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename in ["/etc/passwd", "/etc/shadow"]`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType | PatternValueType,
		},
		{
			Field: "process.uid",
			Types: ScalarValueType,
		},
	}

	filters, err := rs.GetEventFilters("open", capabilities...)
	if err != nil {
		t.Fatalf("should return filters: %s", err)
	}

	if len(filters["open.filename"]) != 2 {
		t.Fatalf("wrong number of filter: %+v", filters)
	}
}

func TestRuleSetNotInFilter(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename not in ["/etc/passwd", "/etc/shadow"]`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType | PatternValueType,
		},
		{
			Field: "process.uid",
			Types: ScalarValueType,
		},
	}

	filters, err := rs.GetEventFilters("open", capabilities...)
	if err != nil {
		t.Fatalf("should return filter: %s", err)
	}

	if len(filters["open.filename"]) != 2 {
		t.Fatalf("wrong number of filter: %+v", filters)
	}

	if !filters["open.filename"][0].Not || !filters["open.filename"][1].Not {
		t.Fatal("filename should be a not filter")
	}
}

func TestRuleSetBitOpFilter(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.flags & O_CREAT > 0`)

	capabilities := []FilteringCapability{
		{
			Field: "open.flags",
			Types: ScalarValueType,
		},
	}

	filters, err := rs.GetEventFilters("open", capabilities...)
	if err != nil {
		t.Fatalf("should return filters: %s", err)
	}

	if len(filters["open.flags"]) != 1 {
		t.Fatalf("wrong number of filter: %+v", filters)
	}

	if filters["open.flags"][0].Value.(int) != syscall.O_CREAT {
		t.Fatal("wrong value for open.flags")
	}
}

func TestRuleSetOppositeFilters(t *testing.T) {
	rs := NewRuleSet(&testModel{}, func() Event { return &testEvent{} }, Opts{Debug: true, Constants: testConstants})

	addRuleExpr(t, rs, `open.filename == "/etc/passwd"`)
	addRuleExpr(t, rs, `open.filename != "/etc/passwd" && open.flags & O_CREAT > 0`)

	capabilities := []FilteringCapability{
		{
			Field: "open.filename",
			Types: ScalarValueType,
		},
		{
			Field: "open.flags",
			Types: ScalarValueType,
		},
	}

	_, err := rs.GetEventFilters("open", capabilities...)
	if _, ok := err.(*OppositeRule); !ok {
		t.Fatal("incorrect error type")
	}
}
