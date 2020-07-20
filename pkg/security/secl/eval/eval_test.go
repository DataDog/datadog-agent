// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

import (
	"fmt"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

func parseRule(expr string, model Model, opts *Opts) (*Rule, error) {
	rule := &Rule{
		ID:         "id1",
		Expression: expr,
	}

	if err := rule.Parse(); err != nil {
		return nil, err
	}

	if err := rule.GenEvaluator(model, opts); err != nil {
		return rule, err
	}

	return rule, nil
}

func eval(t *testing.T, event *testEvent, expr string) (bool, *ast.Rule, error) {
	model := &testModel{}

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(event))

	opts := NewOptsWithParams(false, testConstants)
	rule, err := parseRule(expr, model, opts)
	if err != nil {
		return false, nil, err
	}
	r1 := rule.Eval(ctx)

	opts = NewOptsWithParams(true, testConstants)
	rule, err = parseRule(expr, model, opts)
	if err != nil {
		return false, rule.GetAst(), err
	}
	r2 := rule.Eval(ctx)

	if r1 != r2 {
		t.Fatalf("different result for non-debug and debug evalutators with rule `%s`", expr)
	}

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
		{Expr: `(444 != 444) && ("test" == "test")`, Expected: false},
		{Expr: `(444 != 555) && ("test" == "test")`, Expected: true},
		{Expr: `(444 != 555) && ("test" != "aaaa")`, Expected: true},
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
		{Expr: `1 == 1 & 1`, Expected: true},
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
		{Expr: `process.name =~ ""`, Expected: false},
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
			name: "a",
			uid:  3,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `"a" in [ "a", "b", "c" ]`, Expected: true},
		{Expr: `process.name in [ "c", "b", "a" ]`, Expected: true},
		{Expr: `"d" in [ "a", "b", "c" ]`, Expected: false},
		{Expr: `process.name in [ "c", "b", "z" ]`, Expected: false},
		{Expr: `"a" not in [ "a", "b", "c" ]`, Expected: false},
		{Expr: `process.name not in [ "c", "b", "a" ]`, Expected: false},
		{Expr: `"d" not in [ "a", "b", "c" ]`, Expected: true},
		{Expr: `process.name not in [ "c", "b", "z" ]`, Expected: true},
		{Expr: `3 in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `process.uid in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `4 in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `process.uid in [ 4, 2, 1 ]`, Expected: false},
		{Expr: `3 not in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `3 not in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `4 not in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `4 not in [ 3, 2, 1 ]`, Expected: true},
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
		Field       string
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
		{Expr: `open.filename != open.filename`, Field: "open.filename", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.uid == 456`, Field: "process.uid", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.uid == 123`, Field: "process.uid", IsDiscarder: false},
		{Expr: `open.filename == "test1" && !process.is_root`, Field: "process.is_root", IsDiscarder: true},
		{Expr: `open.filename == "test1" && process.is_root`, Field: "process.is_root", IsDiscarder: false},
	}

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(&event))

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

	opts := NewOptsWithParams(false, make(map[string]interface{}))
	opts.Macros = map[string]*Macro{
		"list": macro,
	}

	expr := `"/etc/shadow" in list`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(&testEvent{}))

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

	opts := NewOptsWithParams(false, make(map[string]interface{}))
	opts.Macros = map[string]*Macro{
		"is_passwd": macro,
	}

	expr := `process.name == "httpd" && is_passwd`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(event))

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

	opts := NewOptsWithParams(false, make(map[string]interface{}))
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

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(event))

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

	opts := NewOptsWithParams(false, make(map[string]interface{}))
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

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(event))

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

func BenchmarkComplex(b *testing.B) {
	event := &testEvent{
		process: testProcess{
			name: "/usr/bin/ls",
			uid:  1,
		},
	}

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(event))

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

	for i := 0; i < b.N; i++ {
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

	ctx := &Context{}
	ctx.SetObject(unsafe.Pointer(event))

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

	for i := 0; i < b.N; i++ {
		if ok, _ := evaluator.PartialEval(ctx, "process.name"); ok {
			b.Fatal("unexpected result")
		}
	}
}
