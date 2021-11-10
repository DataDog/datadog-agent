// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"container/list"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

func newOptsWithParams(constants map[string]interface{}, legacyFields map[Field]Field) *Opts {
	return &Opts{
		Constants:    constants,
		Macros:       make(map[MacroID]*Macro),
		LegacyFields: legacyFields,
		Variables: map[string]VariableValue{
			"pid": {
				IntFnc: func(ctx *Context) int {
					return os.Getpid()
				},
			},
			"str": {
				StringFnc: func(ctx *Context) string {
					return "aaa"
				},
			},
		},
	}
}

func parseRule(expr string, model Model, opts *Opts) (*Rule, error) {
	rule := &Rule{
		ID:         "id1",
		Expression: expr,
	}

	if err := rule.Parse(); err != nil {
		return nil, fmt.Errorf("parsing error: %v", err)
	}

	if err := rule.GenEvaluator(model, opts); err != nil {
		return rule, fmt.Errorf("compilation error: %v", err)
	}

	return rule, nil
}

func eval(t *testing.T, event *testEvent, expr string) (bool, *ast.Rule, error) {
	model := &testModel{}

	ctx := NewContext(unsafe.Pointer(event))

	opts := newOptsWithParams(testConstants, nil)
	rule, err := parseRule(expr, model, opts)
	if err != nil {
		return false, nil, err
	}
	r1 := rule.Eval(ctx)

	return r1, rule.GetAst(), nil
}

func TestStringError(t *testing.T) {
	model := &testModel{}

	rule, err := parseRule(`process.name != "/usr/bin/vipw" && process.uid != 0 && open.filename == 3`, model, &Opts{})
	if rule == nil {
		t.Fatal(err)
	}

	_, err = ruleToEvaluator(rule.GetAst(), model, &Opts{})
	if err == nil || err.(*ErrAstToEval).Pos.Column != 73 {
		t.Fatal("should report a string type error")
	}
}

func TestIntError(t *testing.T) {
	model := &testModel{}

	rule, err := parseRule(`process.name != "/usr/bin/vipw" && process.uid != "test" && Open.Filename == "/etc/shadow"`, model, &Opts{})
	if rule == nil {
		t.Fatal(err)
	}

	_, err = ruleToEvaluator(rule.GetAst(), model, &Opts{})
	if err == nil || err.(*ErrAstToEval).Pos.Column != 51 {
		t.Fatal("should report a string type error")
	}
}

func TestBoolError(t *testing.T) {
	model := &testModel{}

	rule, err := parseRule(`(process.name != "/usr/bin/vipw") == "test"`, model, &Opts{})
	if rule == nil {
		t.Fatal(err)
	}

	_, err = ruleToEvaluator(rule.GetAst(), model, &Opts{})
	if err == nil || err.(*ErrAstToEval).Pos.Column != 38 {
		t.Fatal("should report a bool type error")
	}
}

func TestSimpleString(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name: "/usr/bin/cat",
			uid:  1,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.name != "/usr/bin/vipw"`, Expected: true},
		{Expr: `process.name != "/usr/bin/cat"`, Expected: false},
		{Expr: `process.name == "/usr/bin/cat"`, Expected: true},
		{Expr: `process.name == "/usr/bin/vipw"`, Expected: false},
		{Expr: `(process.name == "/usr/bin/cat" && process.uid == 0) && (process.name == "/usr/bin/cat" && process.uid == 0)`, Expected: false},
		{Expr: `(process.name == "/usr/bin/cat" && process.uid == 1) && (process.name == "/usr/bin/cat" && process.uid == 1)`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestSimpleInt(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			uid: 444,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `111 != 555`, Expected: true},
		{Expr: `process.uid != 555`, Expected: true},
		{Expr: `process.uid != 444`, Expected: false},
		{Expr: `process.uid == 444`, Expected: true},
		{Expr: `process.uid == 555`, Expected: false},
		{Expr: `--3 == 3`, Expected: true},
		{Expr: `3 ^ 3 == 0`, Expected: true},
		{Expr: `^0 == -1`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestSimpleBool(t *testing.T) {
	event := &testEvent{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `(444 == 444) && ("test" == "test")`, Expected: true},
		{Expr: `(444 == 444) and ("test" == "test")`, Expected: true},
		{Expr: `(444 != 444) && ("test" == "test")`, Expected: false},
		{Expr: `(444 != 555) && ("test" == "test")`, Expected: true},
		{Expr: `(444 != 555) && ("test" != "aaaa")`, Expected: true},
		{Expr: `(444 != 555) && # blah blah
		# blah blah
		("test" != "aaaa")`, Expected: true},
		{Expr: `(444 != 555) && # blah blah
		# blah blah
		("test" == "aaaa")`, Expected: false},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestPrecedence(t *testing.T) {
	event := &testEvent{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `false || (true != true)`, Expected: false},
		{Expr: `false || true`, Expected: true},
		{Expr: `false or true`, Expected: true},
		{Expr: `1 == 1 & 1`, Expected: true},
		{Expr: `not true && false`, Expected: false},
		{Expr: `not (true && false)`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestParenthesis(t *testing.T) {
	event := &testEvent{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `(true) == (true)`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestSimpleBitOperations(t *testing.T) {
	event := &testEvent{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `(3 & 3) == 3`, Expected: true},
		{Expr: `(3 & 1) == 3`, Expected: false},
		{Expr: `(2 | 1) == 3`, Expected: true},
		{Expr: `(3 & 1) != 0`, Expected: true},
		{Expr: `0 != 3 & 1`, Expected: true},
		{Expr: `(3 ^ 3) == 0`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`", test.Expr)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestRegexp(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name: "/usr/bin/c$t",
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.name =~ "/usr/bin/*"`, Expected: true},
		{Expr: `process.name =~ "/usr/sbin/*"`, Expected: false},
		{Expr: `process.name !~ "/usr/sbin/*"`, Expected: true},
		{Expr: `process.name =~ "/bin/"`, Expected: false},
		{Expr: `process.name =~ "/bin/*"`, Expected: false},
		{Expr: `process.name =~ "*/bin/*"`, Expected: true},
		{Expr: `process.name =~ "*/bin"`, Expected: false},
		{Expr: `process.name =~ "/usr/*/c$t"`, Expected: true},
		{Expr: `process.name =~ "/usr/*/bin/*"`, Expected: false},
		{Expr: `process.name == ~"/usr/bin/*"`, Expected: true},
		{Expr: `process.name == ~"/usr/sbin/*"`, Expected: false},
		{Expr: `process.name =~ ~"/usr/bin/*"`, Expected: true},
		{Expr: `process.name =~ "/usr/bin/c$t"`, Expected: true},
		{Expr: `process.name =~ "/usr/bin/c$taaa"`, Expected: false},
		{Expr: `process.name =~ r".*/bin/.*"`, Expected: true},
		{Expr: `process.name =~ r".*/[usr]+/bin/.*"`, Expected: true},
		{Expr: `process.name =~ r".*/[abc]+/bin/.*"`, Expected: false},
		{Expr: `process.name == r".*/bin/.*"`, Expected: true},
		{Expr: `r".*/bin/.*" == process.name`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestVariables(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name: fmt.Sprintf("/proc/%d/maps/aaa", os.Getpid()),
			pid:  os.Getpid(),
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.name == "/proc/${pid}/maps/${str}"`, Expected: true},
		{Expr: `process.name == "/proc/${pid}/maps/${str"`, Expected: false},
		{Expr: `process.name == "/proc/${pid/maps/${str"`, Expected: false},
		{Expr: `process.pid == ${pid}`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestInArray(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name: "aaa",
			uid:  3,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `"a" in [ "a", "b", "c" ]`, Expected: true},
		{Expr: `process.name in [ "c", "b", "aaa" ]`, Expected: true},
		{Expr: `"d" in [ "aaa", "b", "c" ]`, Expected: false},
		{Expr: `process.name in [ "c", "b", "z" ]`, Expected: false},
		{Expr: `"aaa" not in [ "aaa", "b", "c" ]`, Expected: false},
		{Expr: `process.name not in [ "c", "b", "aaa" ]`, Expected: false},
		{Expr: `"d" not in [ "aaa", "b", "c" ]`, Expected: true},
		{Expr: `process.name not in [ "c", "b", "z" ]`, Expected: true},
		{Expr: `3 in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `process.uid in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `4 in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `process.uid in [ 4, 2, 1 ]`, Expected: false},
		{Expr: `3 not in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `3 not in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `4 not in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `4 not in [ 3, 2, 1 ]`, Expected: true},
		{Expr: `process.name in [ ~"*a*" ]`, Expected: true},
		{Expr: `process.name in [ ~"*d*" ]`, Expected: false},
		{Expr: `process.name in [ ~"*d*", "aaa" ]`, Expected: true},
		{Expr: `process.name in [ ~"*d*", "aa*" ]`, Expected: false},
		{Expr: `process.name in [ ~"*d*", ~"aa*" ]`, Expected: true},
		{Expr: `process.name in [ r".*d.*", r"aa.*" ]`, Expected: true},
		{Expr: `process.name in [ r".*d.*", r"ab.*" ]`, Expected: false},
		{Expr: `process.name not in [ r".*d.*", r"ab.*" ]`, Expected: true},
		{Expr: `process.name in [ "bbb", "aaa" ]`, Expected: true},
		{Expr: `process.name not in [ "bbb", "aaa" ]`, Expected: false},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s: %s`", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestComplex(t *testing.T) {
	event := &testEvent{
		open: testOpen{
			filename: "/var/lib/httpd/htpasswd",
			flags:    syscall.O_CREAT | syscall.O_TRUNC | syscall.O_EXCL | syscall.O_RDWR | syscall.O_WRONLY,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `open.filename =~ "/var/lib/httpd/*" && open.flags & (O_CREAT | O_TRUNC | O_EXCL | O_RDWR | O_WRONLY) > 0`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s: %s`", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestPartial(t *testing.T) {
	event := testEvent{
		process: testProcess{
			name:   "abc",
			uid:    123,
			isRoot: true,
		},
		open: testOpen{
			filename: "xyz",
		},
	}

	tests := []struct {
		Expr        string
		Field       Field
		IsDiscarder bool
	}{
		{Expr: `true || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: false},
		{Expr: `false || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: true},
		{Expr: `true || process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `false || process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `true && process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: true},
		{Expr: `false && process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: true},
		{Expr: `true && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `false && process.name == "abc"`, Field: "process.name", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.name != "/usr/bin/cat"`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename == "test1" || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename == "test1" || process.name != "/usr/bin/cat"`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename == "test1" && !(process.name == "/usr/bin/cat")`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename == "test1" && !(process.name != "/usr/bin/cat")`, Field: "process.name", IsDiscarder: true},
		{Expr: `open.filename == "test1" && (process.name =~ "/usr/bin/*" )`, Field: "process.name", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.name =~ "ab*" `, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename == "test1" && process.name == open.filename`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename =~ "test1" && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename in [ "test1", "test2" ] && (process.name == open.filename)`, Field: "process.name", IsDiscarder: false},
		{Expr: `open.filename in [ "test1", "test2" ] && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `!(open.filename in [ "test1", "test2" ]) && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ]) && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ]) && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && true) && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && false) && process.name == "abc"`, Field: "process.name", IsDiscarder: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && false) && !(process.name == "abc")`, Field: "process.name", IsDiscarder: true},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && false) && !(process.name == "abc")`, Field: "open.filename", IsDiscarder: false},
		{Expr: `(open.filename not in [ "test1", "xyz" ] && true) && !(process.name == "abc")`, Field: "open.filename", IsDiscarder: true},
		{Expr: `open.filename == open.filename`, Field: "open.filename", IsDiscarder: false},
		{Expr: `open.filename != open.filename`, Field: "open.filename", IsDiscarder: false},
		{Expr: `open.filename == "test1" && process.uid == 456`, Field: "process.uid", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.uid == 123`, Field: "process.uid", IsDiscarder: false},
		{Expr: `open.filename == "test1" && !process.is_root`, Field: "process.is_root", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.is_root`, Field: "process.is_root", IsDiscarder: false},
		{Expr: `open.filename =~ "*test1*"`, Field: "open.filename", IsDiscarder: true},
	}

	ctx := NewContext(unsafe.Pointer(&event))

	for _, test := range tests {
		model := &testModel{}
		opts := &Opts{Constants: testConstants}

		rule, err := parseRule(test.Expr, model, opts)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}
		if err := rule.GenPartials(); err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		result, err := rule.PartialEval(ctx, test.Field)
		if err != nil {
			t.Fatalf("error while partial evaluating `%s` for `%s`: %s", test.Expr, test.Field, err)
		}

		if !result != test.IsDiscarder {
			t.Fatalf("expected result `%t` for `%s`, got `%t`\n%s", test.IsDiscarder, test.Field, result, test.Expr)
		}
	}
}

func TestMacroList(t *testing.T) {
	macro := &Macro{
		ID:         "list",
		Expression: `[ "/etc/shadow", "/etc/password" ]`,
	}

	if err := macro.Parse(); err != nil {
		t.Fatalf("%s\n%s", err, macro.Expression)
	}

	model := &testModel{}

	if err := macro.GenEvaluator(model, &Opts{}); err != nil {
		t.Fatalf("%s\n%s", err, macro.Expression)
	}

	opts := newOptsWithParams(make(map[string]interface{}), nil)
	opts.Macros = map[string]*Macro{
		"list": macro,
	}

	expr := `"/etc/shadow" in list`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := NewContext(unsafe.Pointer(&testEvent{}))

	if !rule.Eval(ctx) {
		t.Fatalf("should return true")
	}
}

func TestMacroExpression(t *testing.T) {
	macro := &Macro{
		ID:         "is_passwd",
		Expression: `open.filename in [ "/etc/shadow", "/etc/passwd" ]`,
	}

	if err := macro.Parse(); err != nil {
		t.Fatalf("%s\n%s", err, macro.Expression)
	}

	event := &testEvent{
		process: testProcess{
			name: "httpd",
		},
		open: testOpen{
			filename: "/etc/passwd",
		},
	}

	model := &testModel{}

	if err := macro.GenEvaluator(model, &Opts{}); err != nil {
		t.Fatalf("%s\n%s", err, macro.Expression)
	}

	opts := newOptsWithParams(make(map[string]interface{}), nil)
	opts.Macros = map[string]*Macro{
		"is_passwd": macro,
	}

	expr := `process.name == "httpd" && is_passwd`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := NewContext(unsafe.Pointer(event))
	if !rule.Eval(ctx) {
		t.Fatalf("should return true")
	}
}

func TestMacroPartial(t *testing.T) {
	macro := &Macro{
		ID:         "is_passwd",
		Expression: `open.filename in [ "/etc/shadow", "/etc/passwd" ]`,
	}

	if err := macro.Parse(); err != nil {
		t.Fatalf("%s\n%s", err, macro.Expression)
	}

	event := &testEvent{
		process: testProcess{
			name: "httpd",
		},
		open: testOpen{
			filename: "/etc/passwd",
		},
	}

	model := &testModel{}

	if err := macro.GenEvaluator(model, &Opts{}); err != nil {
		t.Fatalf("%s\n%s", err, macro.Expression)
	}

	opts := newOptsWithParams(make(map[string]interface{}), nil)
	opts.Macros = map[string]*Macro{
		"is_passwd": macro,
	}

	expr := `process.name == "httpd" && is_passwd`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	if err := rule.GenPartials(); err != nil {
		t.Fatalf("error while generating partials `%s`: %s", expr, err)
	}

	ctx := NewContext(unsafe.Pointer(event))

	result, err := rule.PartialEval(ctx, "open.filename")
	if err != nil {
		t.Fatalf("error while partial evaluating `%s` : %s", expr, err)
	}

	if !result {
		t.Fatal("open.filename should be a discarder")
	}

	event.open.filename = "abc"
	result, err = rule.PartialEval(ctx, "open.filename")
	if err != nil {
		t.Fatalf("error while partial evaluating `%s` : %s", expr, err)
	}

	if result {
		t.Fatal("open.filename should be a discarder")
	}
}

func TestNestedMacros(t *testing.T) {
	macro1 := &Macro{
		ID:         "sensitive_files",
		Expression: `[ "/etc/shadow", "/etc/passwd" ]`,
	}

	if err := macro1.Parse(); err != nil {
		t.Fatalf("%s\n%s", err, macro1.Expression)
	}

	macro2 := &Macro{
		ID:         "is_sensitive_opened",
		Expression: `open.filename in sensitive_files`,
	}

	if err := macro2.Parse(); err != nil {
		t.Fatalf("%s\n%s", err, macro2.Expression)
	}

	event := &testEvent{
		open: testOpen{
			filename: "/etc/passwd",
		},
	}

	model := &testModel{}

	opts := newOptsWithParams(make(map[string]interface{}), nil)
	opts.Macros = map[string]*Macro{
		"sensitive_files":     macro1,
		"is_sensitive_opened": macro2,
	}

	if err := macro1.GenEvaluator(model, opts); err != nil {
		t.Fatalf("%s\n%s", err, macro1.Expression)
	}

	if err := macro2.GenEvaluator(model, opts); err != nil {
		t.Fatalf("%s\n%s", err, macro2.Expression)
	}

	expr := `is_sensitive_opened`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := NewContext(unsafe.Pointer(event))
	if !rule.Eval(ctx) {
		t.Fatalf("should return true")
	}
}

func TestFieldValidator(t *testing.T) {
	expr := `process.uid == -100 && open.filename == "/etc/passwd"`
	if _, err := parseRule(expr, &testModel{}, &Opts{}); err == nil {
		t.Error("expected an error on process.uid being negative")
	}
}

func TestLegacyField(t *testing.T) {
	model := &testModel{}
	opts := newOptsWithParams(testConstants, legacyFields)

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.legacy_name == "/tmp/secrets"`, Expected: true},
		{Expr: `process.random_name == "/tmp/secrets"`, Expected: false},
		{Expr: `process.name == "/tmp/secrets"`, Expected: true},
	}

	for _, test := range tests {
		_, err := parseRule(test.Expr, model, opts)
		if err == nil != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, err == nil, test.Expr)
		}
	}
}

func TestRegisterSyntaxError(t *testing.T) {
	model := &testModel{}
	opts := newOptsWithParams(testConstants, nil)

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.list[_].key == 10 && process.list[_].value == "AAA"`, Expected: true},
		{Expr: `process.list[].key == 10 && process.list.value == "AAA"`, Expected: false},
		{Expr: `process.list[_].key == 10 && process.list.value == "AAA"`, Expected: true},
		{Expr: `process.list.key[] == 10 && process.list.value == "AAA"`, Expected: false},
		{Expr: `process[].list.key == 10 && process.list.value == "AAA"`, Expected: false},
		{Expr: `[]process.list.key == 10 && process.list.value == "AAA"`, Expected: false},
		//{Expr: `process.list[A].key == 10 && process.list[A].value == "AAA" && process.array[B].key == 10 && process.array[B].value == "AAA"`, Expected: false},
	}

	for _, test := range tests {
		_, err := parseRule(test.Expr, model, opts)
		if err == nil != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, err == nil, test.Expr)
		}
	}
}

func TestRegister(t *testing.T) {
	event := &testEvent{
		process: testProcess{},
	}

	event.process.list = list.New()
	event.process.list.PushBack(&testItem{key: 10, value: "AAA"})
	event.process.list.PushBack(&testItem{key: 100, value: "BBB"})
	event.process.list.PushBack(&testItem{key: 200, value: "CCC"})

	event.process.array = []*testItem{
		{key: 1000, value: "EEEE", flag: true},
		{key: 1002, value: "DDDD", flag: false},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.list[_].key == 10`, Expected: true},
		{Expr: `process.list[_].key == 9999`, Expected: false},
		{Expr: `process.list[_].key != 10`, Expected: false},
		{Expr: `process.list[_].key != 9999`, Expected: true},

		{Expr: `process.list[_].key >= 200`, Expected: true},
		{Expr: `process.list[_].key > 100`, Expected: true},
		{Expr: `process.list[_].key <= 200`, Expected: true},
		{Expr: `process.list[_].key < 100`, Expected: true},

		{Expr: `10 == process.list[_].key`, Expected: true},
		{Expr: `9999 == process.list[_].key`, Expected: false},
		{Expr: `10 != process.list[_].key`, Expected: false},
		{Expr: `9999 != process.list[_].key`, Expected: true},

		{Expr: `9999 in process.list[_].key`, Expected: false},
		{Expr: `9999 not in process.list[_].key`, Expected: true},
		{Expr: `10 in process.list[_].key`, Expected: true},
		{Expr: `10 not in process.list[_].key`, Expected: false},

		{Expr: `process.list[_].key > 10`, Expected: true},
		{Expr: `process.list[_].key > 9999`, Expected: false},
		{Expr: `process.list[_].key < 10`, Expected: false},
		{Expr: `process.list[_].key < 9999`, Expected: true},

		{Expr: `5 < process.list[_].key`, Expected: true},
		{Expr: `9999 < process.list[_].key`, Expected: false},
		{Expr: `10 > process.list[_].key`, Expected: false},
		{Expr: `9999 > process.list[_].key`, Expected: true},

		{Expr: `true in process.array[_].flag`, Expected: true},
		{Expr: `false not in process.array[_].flag`, Expected: false},

		{Expr: `process.array[_].flag == true`, Expected: true},
		{Expr: `process.array[_].flag != false`, Expected: false},

		{Expr: `"AAA" in process.list[_].value`, Expected: true},
		{Expr: `"ZZZ" in process.list[_].value`, Expected: false},
		{Expr: `"AAA" not in process.list[_].value`, Expected: false},
		{Expr: `"ZZZ" not in process.list[_].value`, Expected: true},

		{Expr: `~"AA*" in process.list[_].value`, Expected: true},
		{Expr: `~"ZZ*" in process.list[_].value`, Expected: false},
		{Expr: `~"AA*" not in process.list[_].value`, Expected: false},
		{Expr: `~"ZZ*" not in process.list[_].value`, Expected: true},

		{Expr: `r"[A]{1,3}" in process.list[_].value`, Expected: true},
		{Expr: `process.list[_].value in [r"[A]{1,3}", "nnnnn"]`, Expected: true},

		{Expr: `process.list[_].value == ~"AA*"`, Expected: true},
		{Expr: `process.list[_].value == ~"ZZ*"`, Expected: false},
		{Expr: `process.list[_].value != ~"AA*"`, Expected: false},
		{Expr: `process.list[_].value != ~"ZZ*"`, Expected: true},

		{Expr: `process.list[_].value =~ "AA*"`, Expected: true},
		{Expr: `process.list[_].value =~ "ZZ*"`, Expected: false},
		{Expr: `process.list[_].value !~ "AA*"`, Expected: false},
		{Expr: `process.list[_].value !~ "ZZ*"`, Expected: true},

		{Expr: `process.list[_].value in [~"AA*", "nnnnn"]`, Expected: true},
		{Expr: `process.list[_].value in [~"ZZ*", "nnnnn"]`, Expected: false},
		{Expr: `process.list[_].value not in [~"AA*", "nnnnn"]`, Expected: false},
		{Expr: `process.list[_].value not in [~"ZZ*", "nnnnn"]`, Expected: true},

		{Expr: `process.list[_].key == 10 && process.list[_].value == "AAA"`, Expected: true},
		{Expr: `process.list[_].key == 9999 && process.list[_].value == "AAA"`, Expected: false},
		{Expr: `process.list[_].key == 100 && process.list[_].value == "BBB"`, Expected: true},
		{Expr: `process.list[_].key == 200 && process.list[_].value == "CCC"`, Expected: true},
		{Expr: `process.list.key == 200 && process.list.value == "AAA"`, Expected: true},
		{Expr: `process.list[_].key == 10 && process.list.value == "AAA"`, Expected: true},

		{Expr: `process.array[_].key == 1000 && process.array[_].value == "EEEE"`, Expected: true},
		{Expr: `process.array[_].key == 1002 && process.array[_].value == "EEEE"`, Expected: true},

		//{Expr: `process.list[A].key == 200 && process.list[A].value == "CCC"`, Expected: true},
		//{Expr: `process.list[A].key == 200 && process.list[B].value == "BBB"`, Expected: true},
		//{Expr: `process.list[A].key == 200 || process.list[B].value == "AA"`, Expected: true},
		//{Expr: `process.array[A].key == 1002 && process.array[B].value == "DDDD"`, Expected: true},
		//{Expr: `process.list[_].key == 10 && process.list[_].value == "AA" && process.array[A].key == 1002 && process.array[A].value == "DDDD"`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestRegisterPartial(t *testing.T) {
	event := &testEvent{
		process: testProcess{},
	}

	event.process.list = list.New()
	event.process.list.PushBack(&testItem{key: 10, value: "AA"})
	event.process.list.PushBack(&testItem{key: 100, value: "BBB"})
	event.process.list.PushBack(&testItem{key: 200, value: "CCC"})

	event.process.array = []*testItem{
		{key: 1000, value: "EEEE"},
		{key: 1002, value: "DDDD"},
	}

	tests := []struct {
		Expr        string
		Field       Field
		IsDiscarder bool
	}{
		{Expr: `process.list[_].key == 10 && process.list[_].value == "AA"`, Field: "process.list.key", IsDiscarder: false},
		{Expr: `process.list[_].key == 55 && process.list[_].value == "AA"`, Field: "process.list.key", IsDiscarder: true},
		{Expr: `process.list[_].key == 55 && process.list[_].value == "AA"`, Field: "process.list.value", IsDiscarder: false},
		{Expr: `process.list[_].key == 10 && process.list[_].value == "ZZZ"`, Field: "process.list.value", IsDiscarder: true},
		//{Expr: `process.list[A].key == 10 && process.list[B].value == "ZZZ"`, Field: "process.list.key", IsDiscarder: false},
		//{Expr: `process.list[A].key == 55 && process.list[B].value == "AA"`, Field: "process.list.key", IsDiscarder: true},
	}

	ctx := NewContext(unsafe.Pointer(event))

	for _, test := range tests {
		model := &testModel{}
		opts := &Opts{Constants: testConstants}

		rule, err := parseRule(test.Expr, model, opts)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}
		if err := rule.GenPartials(); err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		result, err := rule.PartialEval(ctx, test.Field)
		if err != nil {
			t.Fatalf("error while partial evaluating `%s` for `%s`: %s", test.Expr, test.Field, err)
		}

		if !result != test.IsDiscarder {
			t.Fatalf("expected result `%t` for `%s`, got `%t`\n%s", test.IsDiscarder, test.Field, result, test.Expr)
		}
	}
}

func TestOptimizer(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			uid: 44,
			gid: 44,
		},
	}

	event.process.list = list.New()
	event.process.list.PushBack(&testItem{key: 10, value: "AA"})

	tests := []struct {
		Expr      string
		Evaluated func() bool
	}{
		{Expr: `process.list[_].key == 44 && process.gid == 55`, Evaluated: func() bool { return event.listEvaluated }},
		{Expr: `process.gid == 55 && process.list[_].key == 44`, Evaluated: func() bool { return event.listEvaluated }},
		{Expr: `process.uid in [66, 77, 88] && process.gid == 55`, Evaluated: func() bool { return event.uidEvaluated }},
		{Expr: `process.gid == 55 && process.uid in [66, 77, 88]`, Evaluated: func() bool { return event.uidEvaluated }},
	}

	for _, test := range tests {
		_, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating: %s", err)
		}

		if test.Evaluated() {
			t.Fatalf("not optimized: %s", test.Expr)
		}
	}
}

func TestDuration(t *testing.T) {
	// time reliability issue
	if runtime.GOARCH == "386" && runtime.GOOS == "windows" {
		t.Skip()
	}

	event := &testEvent{
		process: testProcess{
			createdAt: time.Now().UnixNano(),
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.created_at < 2s`, Expected: true},
		{Expr: `process.created_at > 2s`, Expected: false},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\nnow: %v, create_at: %v\n%s", test.Expected, result, time.Now().UnixNano(), event.process.createdAt, test.Expr)
		}
	}

	time.Sleep(4 * time.Second)

	tests = []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.created_at < 2s`, Expected: false},
		{Expr: `process.created_at > 2s`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\nnow: %v, create_at: %v\n%s", test.Expected, result, time.Now().UnixNano(), event.process.createdAt, test.Expr)
		}
	}
}

func TestOpOverrides(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			ovName: "abc",
		},
	}

	event.process.array = []*testItem{
		{key: 1000, value: "abc", flag: true},
	}

	// values that will be returned by the operator override
	event.process.ovNameValues = func() StringValues {
		var values StringValues
		values.AppendValue("abc")
		return values
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.ov_name == "not"`, Expected: true},
		{Expr: `process.ov_name in ["not"]`, Expected: true},
		{Expr: `process.array.value == "not"]`, Expected: true},
		{Expr: `process.array.value in ["not"]`, Expected: true},
	}

	for _, test := range tests {
		result, _, err := eval(t, event, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func BenchmarkArray(b *testing.B) {
	event := &testEvent{
		process: testProcess{
			name: "/usr/bin/ls",
			uid:  1,
		},
	}

	var values []string
	for i := 0; i != 255; i++ {
		values = append(values, fmt.Sprintf(`~"/usr/bin/aaa-%d"`, i))
	}

	for i := 0; i != 255; i++ {
		values = append(values, fmt.Sprintf(`"/usr/bin/aaa-%d"`, i))
	}

	base := fmt.Sprintf(`(process.name in [%s, ~"/usr/bin/ls"])`, strings.Join(values, ","))
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	rule, err := parseRule(expr, &testModel{}, &Opts{})
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(unsafe.Pointer(event))
		if evaluator.Eval(ctx) != true {
			b.Fatal("unexpected result")
		}
	}
}

func BenchmarkComplex(b *testing.B) {
	event := &testEvent{
		process: testProcess{
			name: "/usr/bin/ls",
			uid:  1,
		},
	}

	base := `(process.name == "/usr/bin/ls" && process.uid == 1)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	rule, err := parseRule(expr, &testModel{}, &Opts{})
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(unsafe.Pointer(event))
		if evaluator.Eval(ctx) != true {
			b.Fatal("unexpected result")
		}
	}
}

func BenchmarkPartial(b *testing.B) {
	event := &testEvent{
		process: testProcess{
			name: "abc",
			uid:  1,
		},
	}

	ctx := NewContext(unsafe.Pointer(event))

	base := `(process.name == "/usr/bin/ls" && process.uid != 0)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	model := &testModel{}

	rule, err := parseRule(expr, model, &Opts{})
	if err != nil {
		b.Fatal(err)
	}

	if err := rule.GenEvaluator(model, &Opts{}); err != nil {
		b.Fatal(err)
	}

	if err := rule.GenPartials(); err != nil {
		b.Fatal(err)
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if ok, _ := evaluator.PartialEval(ctx, "process.name"); ok {
			b.Fatal("unexpected result")
		}
	}
}

func BenchmarkPool(b *testing.B) {
	event := &testEvent{
		process: testProcess{
			name: "/usr/bin/ls",
			uid:  1,
		},
	}

	pool := NewContextPool()

	base := `(process.name == "/usr/bin/ls" && process.uid == 1)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	rule, err := parseRule(expr, &testModel{}, &Opts{})
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := pool.Get(unsafe.Pointer(event))
		if evaluator.Eval(ctx) != true {
			b.Fatal("unexpected result")
		}
		pool.pool.Put(ctx)
	}
}
