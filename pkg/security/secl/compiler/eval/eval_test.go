// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"container/list"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

func newOptsWithParams(constants map[string]interface{}, legacyFields map[Field]Field) *Opts {
	opts := &Opts{
		Constants:    constants,
		LegacyFields: legacyFields,
	}

	variables := map[string]SECLVariable{
		"pid": NewScopedIntVariable(func(_ *Context, _ bool) (int, bool) {
			return os.Getpid(), true
		}, nil),
		"str": NewScopedStringVariable(func(_ *Context, _ bool) (string, bool) {
			return "aaa", true
		}, nil),
	}

	return opts.WithVariables(variables).WithMacroStore(&MacroStore{})
}

func parseRule(expr string, model Model, opts *Opts) (*Rule, error) {
	pc := ast.NewParsingContext(false)
	rule, err := NewRule("id1", expr, pc, opts)
	if err != nil {
		return nil, fmt.Errorf("parsing error: %v", err)
	}

	if err := rule.GenEvaluator(model); err != nil {
		return rule, fmt.Errorf("compilation error: %v", err)
	}

	return rule, nil
}

func eval(ctx *Context, expr string) (bool, *ast.Rule, error) {
	model := &testModel{}

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

	opts := newOptsWithParams(nil, nil)
	rule, err := parseRule(`process.name != "/usr/bin/vipw" && process.uid != 0 && open.filename == 3`, model, opts)
	if rule == nil {
		t.Fatal(err)
	}

	_, err = NewRuleEvaluator(rule.GetAst(), model, opts)
	if err == nil || err.(*ErrAstToEval).Pos.Column != 73 {
		t.Fatal("should report a string type error")
	}
}

func TestIntError(t *testing.T) {
	model := &testModel{}

	opts := newOptsWithParams(nil, nil)
	rule, err := parseRule(`process.name != "/usr/bin/vipw" && process.uid != "test" && Open.Filename == "/etc/shadow"`, model, opts)
	if rule == nil {
		t.Fatal(err)
	}

	_, err = NewRuleEvaluator(rule.GetAst(), model, opts)
	if err == nil || err.(*ErrAstToEval).Pos.Column != 51 {
		t.Fatal("should report a string type error")
	}
}

func TestBoolError(t *testing.T) {
	model := &testModel{}

	opts := newOptsWithParams(nil, nil)
	rule, err := parseRule(`(process.name != "/usr/bin/vipw") == "test"`, model, opts)
	if rule == nil {
		t.Fatal(err)
	}

	_, err = NewRuleEvaluator(rule.GetAst(), model, opts)
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
		{Expr: `process.name != ""`, Expected: true},
		{Expr: `process.name != "/usr/bin/vipw"`, Expected: true},
		{Expr: `process.name != "/usr/bin/cat"`, Expected: false},
		{Expr: `process.name == "/usr/bin/cat"`, Expected: true},
		{Expr: `process.name == "/usr/bin/vipw"`, Expected: false},
		{Expr: `(process.name == "/usr/bin/cat" && process.uid == 0) && (process.name == "/usr/bin/cat" && process.uid == 0)`, Expected: false},
		{Expr: `(process.name == "/usr/bin/cat" && process.uid == 1) && (process.name == "/usr/bin/cat" && process.uid == 1)`, Expected: true},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`", test.Expr)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestStringMatcher(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name:  "/usr/bin/c$t",
			argv0: "http://example.com",
		},
		open: testOpen{
			filename: "dGVzdA==.dGVzdA==.dGVzdA==.dGVzdA==.dGVzdA==.example.com",
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.name =~ "/usr/bin/c$t/test/*"`, Expected: false},
		{Expr: `process.name =~ "/usr/bin/c$t/test/**"`, Expected: false},
		{Expr: `process.name =~ "/usr/bin/c$t/*"`, Expected: false},
		{Expr: `process.name =~ "/usr/bin/c$t/**"`, Expected: false},
		{Expr: `process.name =~ "/usr/bin/c$t*"`, Expected: true},
		{Expr: `process.name =~ "/usr/bin/c*"`, Expected: true},
		{Expr: `process.name =~ "/usr/bin/l*"`, Expected: false},
		{Expr: `process.name =~ "/usr/bin/**"`, Expected: true},
		{Expr: `process.name =~ "/usr/**"`, Expected: true},
		{Expr: `process.name =~ "/**"`, Expected: true},
		{Expr: `process.name =~ "/etc/**"`, Expected: false},
		{Expr: `process.name =~ ""`, Expected: false},
		{Expr: `process.name =~ "*"`, Expected: false},
		{Expr: `process.name =~ "/*"`, Expected: false},
		{Expr: `process.name =~ "*/*"`, Expected: false},
		{Expr: `process.name =~ "/*/*/*"`, Expected: true},
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
		{Expr: `process.argv0 =~ "http://*"`, Expected: true},
		{Expr: `process.argv0 =~ "*example.com"`, Expected: true},
		{Expr: `open.filename == r"^((?:[A-Za-z\d+]{4})*(?:[A-Za-z\d+]{3}=|[A-Za-z\d+]{2}==)\.)*(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$"`, Expected: true},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		{Expr: `process.name == "/proc/${pid}/maps/${str}3"`, Expected: false},
		{Expr: `process.name == "/proc/${pid}/maps/${str"`, Expected: false},
		{Expr: `process.name == "/proc/${pid/maps/${str"`, Expected: false},
		{Expr: `process.pid == ${pid}`, Expected: true},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		retval: int(syscall.EACCES),
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
		{Expr: `retval in [ EPERM, EACCES, EPFNOSUPPORT ]`, Expected: true},
		{Expr: `retval in [ EPERM, EPIPE, EPFNOSUPPORT ]`, Expected: false},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s: %s`", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestPartial(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name:   "abc",
			uid:    123,
			isRoot: true,
		},
		open: testOpen{
			filename: "xyz",
		},
	}

	variables := make(map[string]SECLVariable)
	variables["var"] = NewScopedBoolVariable(
		func(_ *Context, _ bool) (bool, bool) {
			return false, true
		},
		func(_ *Context, _ interface{}) error {
			return nil
		},
	)

	tests := []struct {
		Expr        string
		Field       Field
		IsDiscarder bool
	}{
		{Expr: `true || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: false},
		{Expr: `false || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: true},
		{Expr: `1 != 1 || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscarder: true},
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
		{Expr: `open.filename =~ "*test1*"`, Field: "open.filename", IsDiscarder: true},
		{Expr: `process.uid & (1 | 1024) == 1`, Field: "process.uid", IsDiscarder: false},
		{Expr: `process.uid & (1 | 2) == 1`, Field: "process.uid", IsDiscarder: true},
		{Expr: `(open.filename not in [ "test1", "xyz" ] && true) && !(process.name == "abc")`, Field: "open.filename", IsDiscarder: true},
		{Expr: `process.uid == 123 && ${var} == true`, Field: "process.uid", IsDiscarder: false},
		{Expr: `process.uid == 123 && ${var} != true`, Field: "process.uid", IsDiscarder: false},
		{Expr: `process.uid == 678 && ${var} == true`, Field: "process.uid", IsDiscarder: true},
		{Expr: `process.uid == 678 && ${var} != true`, Field: "process.uid", IsDiscarder: true},
		{Expr: `process.name == "abc" && ^process.uid != 0`, Field: "process.name", IsDiscarder: false},
		{Expr: `process.name == "abc" && ^process.uid == 0`, Field: "process.uid", IsDiscarder: true},
		{Expr: `process.name == "abc" && ^process.uid != 0`, Field: "process.uid", IsDiscarder: false},
		{Expr: `process.name == "abc" || ^process.uid == 0`, Field: "process.uid", IsDiscarder: false},
		{Expr: `process.name == "abc" || ^process.uid != 0`, Field: "process.uid", IsDiscarder: false},
		{Expr: `process.name =~ "/usr/sbin/*" && process.uid == 0 && process.is_root`, Field: "process.uid", IsDiscarder: true},
	}

	for _, test := range tests {
		model := &testModel{}

		opts := newOptsWithParams(testConstants, nil)
		opts.WithVariables(variables)

		rule, err := parseRule(test.Expr, model, opts)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		ctx := NewContext(event)

		result, err := rule.PartialEval(ctx, test.Field)
		if err != nil {
			t.Fatalf("error while partial evaluating `%s` for `%s`: %s", test.Expr, test.Field, err)
		}

		if !result != test.IsDiscarder {
			t.Fatalf("expected result `%t` for `%s`, got `%t`\n%s", test.IsDiscarder, test.Field, !result, test.Expr)
		}
	}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		Expr    string
		OK      bool
		Message string
	}{
		{Expr: `retval in [ EPERM, EACCES ]`, OK: true},
		{Expr: `open.filename in [ my_constant_1, my_constant_2 ]`, OK: true},
		{Expr: `process.is_root in [ true, false ]`, OK: true},
		{Expr: `open.filename in [ EPERM, EACCES ]`, OK: false, Message: "Int array shouldn't be allowed for string field"},
		{Expr: `retval in [ EPERM, true ]`, OK: false, Message: "Constants of different types can't be mixed in an array"},
	}

	for _, test := range tests {
		model := &testModel{}

		opts := newOptsWithParams(testConstants, nil)

		_, err := parseRule(test.Expr, model, opts)
		if !test.OK {
			if err == nil {
				var msg string
				if len(test.Message) > 0 {
					msg = ": reason: " + test.Message
				}
				t.Fatalf("expected an error for `%s`%s", test.Expr, msg)
			}
		} else {
			if err != nil {
				t.Fatalf("error while parsing `%s`: %v", test.Expr, err)
			}
		}
	}
}

func TestMacroList(t *testing.T) {
	model := &testModel{}
	pc := ast.NewParsingContext(false)
	opts := newOptsWithParams(make(map[string]interface{}), nil)

	macro, err := NewMacro(
		"list",
		`[ "/etc/shadow", "/etc/password" ]`,
		model,
		pc,
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	opts.MacroStore.Add(macro)

	expr := `"/etc/shadow" in list`
	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := NewContext(&testEvent{})
	if !rule.Eval(ctx) {
		t.Fatalf("should return true")
	}
}

func TestMacroExpression(t *testing.T) {
	model := &testModel{}
	pc := ast.NewParsingContext(false)
	opts := newOptsWithParams(make(map[string]interface{}), nil)

	macro, err := NewMacro(
		"is_passwd",
		`open.filename in [ "/etc/shadow", "/etc/passwd" ]`,
		model,
		pc,
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	opts.MacroStore.Add(macro)

	event := &testEvent{
		process: testProcess{
			name: "httpd",
		},
		open: testOpen{
			filename: "/etc/passwd",
		},
	}

	expr := `process.name == "httpd" && is_passwd`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := NewContext(event)
	if !rule.Eval(ctx) {
		t.Fatalf("should return true")
	}
}

func TestMacroPartial(t *testing.T) {
	model := &testModel{}
	pc := ast.NewParsingContext(false)
	opts := newOptsWithParams(make(map[string]interface{}), nil)

	macro, err := NewMacro(
		"is_passwd",
		`open.filename in [ "/etc/shadow", "/etc/passwd" ]`,
		model,
		pc,
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	opts.MacroStore.Add(macro)

	event := &testEvent{
		process: testProcess{
			name: "httpd",
		},
		open: testOpen{
			filename: "/etc/passwd",
		},
	}

	expr := `process.name == "httpd" && is_passwd`

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", expr, err)
	}

	ctx := NewContext(event)

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
	event := &testEvent{
		open: testOpen{
			filename: "/etc/passwd",
		},
	}

	model := &testModel{}
	pc := ast.NewParsingContext(false)
	opts := newOptsWithParams(make(map[string]interface{}), nil)

	macro1, err := NewMacro(
		"sensitive_files",
		`[ "/etc/shadow", "/etc/passwd" ]`,
		model,
		pc,
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	opts.MacroStore.Add(macro1)

	macro2, err := NewMacro(
		"is_sensitive_opened",
		`open.filename in sensitive_files`,
		model,
		pc,
		opts,
	)
	if err != nil {
		t.Fatal(err)
	}
	opts.MacroStore.Add(macro2)

	rule, err := parseRule(macro2.ID, model, opts)
	if err != nil {
		t.Fatalf("error while evaluating `%s`: %s", macro2.ID, err)
	}

	ctx := NewContext(event)
	if !rule.Eval(ctx) {
		t.Fatalf("should return true")
	}
}

func TestFieldValidator(t *testing.T) {
	expr := `process.uid == -100 && open.filename == "/etc/passwd"`

	opts := newOptsWithParams(nil, nil)

	if _, err := parseRule(expr, &testModel{}, opts); err == nil {
		t.Error("expected an error on process.uid being negative")
	}
}

func TestLegacyField(t *testing.T) {
	model := &testModel{}

	opts := newOptsWithParams(nil, legacyFields)

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

	opts := newOptsWithParams(nil, nil)

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.list[_].key == 10 && process.list[_].value == "AAA"`, Expected: false},
		{Expr: `process.list[A].key == 10 && process.list[A].value == "AAA"`, Expected: true},
		{Expr: `process.list[A].key == 10 && process.list[B].value == "AAA"`, Expected: false},
		{Expr: `process.list[A].key == 10 && process.array[A].value == "AAA"`, Expected: false},
		{Expr: `process.list[].key == 10 && process.list.value == "AAA"`, Expected: false},
		{Expr: `process.list[A].key == 10 && process.list.value == "AAA"`, Expected: true},
		{Expr: `process.list.key[] == 10 && process.list.value == "AAA"`, Expected: false},
		{Expr: `process[].list.key == 10 && process.list.value == "AAA"`, Expected: false},
		{Expr: `[]process.list.key == 10 && process.list.value == "AAA"`, Expected: false},
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
		{Expr: `process.list[A].key == 10`, Expected: true},
		{Expr: `process.list[A].key == 9999`, Expected: false},
		{Expr: `process.list[A].key != 10`, Expected: true},
		{Expr: `process.list.key != 10`, Expected: false},
		{Expr: `process.list[A].key != 9999`, Expected: true},
		{Expr: `process.list[A].key >= 200`, Expected: true},
		{Expr: `process.list[A].key > 100`, Expected: true},
		{Expr: `process.list[A].key <= 200`, Expected: true},
		{Expr: `process.list[A].key < 100`, Expected: true},

		{Expr: `10 == process.list[A].key`, Expected: true},
		{Expr: `9999 == process.list[A].key`, Expected: false},
		{Expr: `10 != process.list[A].key`, Expected: true},
		{Expr: `9999 != process.list[A].key`, Expected: true},

		{Expr: `9999 in process.list[A].key`, Expected: false},
		{Expr: `9999 not in process.list[A].key`, Expected: true},
		{Expr: `10 in process.list[A].key`, Expected: true},
		{Expr: `10 not in process.list[A].key`, Expected: true},

		{Expr: `process.list[A].key > 10`, Expected: true},
		{Expr: `process.list[A].key > 9999`, Expected: false},
		{Expr: `process.list[A].key < 10`, Expected: false},
		{Expr: `process.list[A].key < 9999`, Expected: true},

		{Expr: `5 < process.list[A].key`, Expected: true},
		{Expr: `9999 < process.list[A].key`, Expected: false},
		{Expr: `10 > process.list[A].key`, Expected: false},
		{Expr: `9999 > process.list[A].key`, Expected: true},

		{Expr: `true in process.array[A].flag`, Expected: true},
		{Expr: `false not in process.array[A].flag`, Expected: false},

		{Expr: `process.array[A].flag == true`, Expected: true},
		{Expr: `process.array[A].flag != false`, Expected: false},

		{Expr: `"AAA" in process.list[A].value`, Expected: true},
		{Expr: `"ZZZ" in process.list[A].value`, Expected: false},
		{Expr: `"AAA" not in process.list[A].value`, Expected: true},
		{Expr: `"ZZZ" not in process.list[A].value`, Expected: true},

		{Expr: `~"AA*" in process.list[A].value`, Expected: true},
		{Expr: `~"ZZ*" in process.list[A].value`, Expected: false},
		{Expr: `~"AA*" not in process.list[A].value`, Expected: true},
		{Expr: `~"ZZ*" not in process.list[A].value`, Expected: true},

		{Expr: `r"[A]{1,3}" in process.list[A].value`, Expected: true},
		{Expr: `process.list[A].value in [r"[A]{1,3}", "nnnnn"]`, Expected: true},

		{Expr: `process.list[A].value == ~"AA*"`, Expected: true},
		{Expr: `process.list[A].value == ~"ZZ*"`, Expected: false},
		{Expr: `process.list[A].value != ~"AA*"`, Expected: true},
		{Expr: `process.list[A].value != ~"ZZ*"`, Expected: true},

		{Expr: `process.list[A].value =~ "AA*"`, Expected: true},
		{Expr: `process.list[A].value =~ "ZZ*"`, Expected: false},
		{Expr: `process.list[A].value !~ "AA*"`, Expected: true},
		{Expr: `process.list[A].value !~ "ZZ*"`, Expected: true},

		{Expr: `process.list[A].value in ["~zzzz", ~"AA*", "nnnnn"]`, Expected: true},
		{Expr: `process.list[A].value in ["~zzzz", ~"AA*", "nnnnn"]`, Expected: true},
		{Expr: `process.list[A].value in ["~zzzz", "AAA", "nnnnn"]`, Expected: true},
		{Expr: `process.list[A].value in ["~zzzz", "AA*", "nnnnn"]`, Expected: false},

		{Expr: `process.list[A].value in [~"ZZ*", "nnnnn"]`, Expected: false},
		{Expr: `process.list[A].value not in [~"AA*", "nnnnn"]`, Expected: true},
		{Expr: `process.list[A].value not in [~"ZZ*", "nnnnn"]`, Expected: true},
		{Expr: `process.list[A].value not in [~"ZZ*", "AAA", "nnnnn"]`, Expected: true},
		{Expr: `process.list[A].value not in [~"ZZ*", ~"AA*", "nnnnn"]`, Expected: true},

		// StringArrayEvaluator in/not in StringArrayEvaluator — previously unhandled, would fail at compile time
		{Expr: `process.list.value in process.array.value`, Expected: false}, // ["AAA","BBB","CCC"] ∩ ["EEEE","DDDD"] = ∅
		{Expr: `process.list.value not in process.array.value`, Expected: true},
		{Expr: `process.array.value in process.list.value`, Expected: false},
		{Expr: `process.array.value not in process.list.value`, Expected: true},

		{Expr: `process.list[A].key == 10 && process.list[A].value == "AAA"`, Expected: true},
		{Expr: `process.list[A].key == 9999 && process.list[A].value == "AAA"`, Expected: false},
		{Expr: `process.list[A].key == 100 && process.list[A].value == "BBB"`, Expected: true},
		{Expr: `process.list[A].key == 200 && process.list[A].value == "CCC"`, Expected: true},
		{Expr: `process.list.key == 200 && process.list.value == "AAA"`, Expected: true},
		{Expr: `process.list[A].key == 10 && process.list[A].value == "AAA"`, Expected: true},
		{Expr: `process.list[A].key == 10 && process.list[A].value == "BBB"`, Expected: false},
		{Expr: `process.list[A].key == 100 && process.list[A].value == "BBB"`, Expected: true},
		{Expr: `process.list.key == 10 && process.list.value == "BBB"`, Expected: true},

		{Expr: `process.array[A].key == 1000 && process.array[A].value == "EEEE"`, Expected: true},
		{Expr: `process.array[A].key == 1002 && process.array[A].value == "EEEE"`, Expected: false},

		{Expr: `process.array[A].key == 1000`, Expected: true},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		{Expr: `process.list[A].key == 10 && process.list[A].value == "AA"`, Field: "process.list.key", IsDiscarder: false},
		{Expr: `process.list[A].key == 55 && process.list[A].value == "AA"`, Field: "process.list.key", IsDiscarder: true},
		{Expr: `process.list[A].key in [55, 10] && process.list[A].value == "AA"`, Field: "process.list.key", IsDiscarder: false},
		{Expr: `process.list[A].key == 55 && process.list[A].value == "AA"`, Field: "process.list.value", IsDiscarder: false},
		{Expr: `process.list[A].key == 10 && process.list[A].value == "ZZZ"`, Field: "process.list.value", IsDiscarder: true},
	}

	for _, test := range tests {
		model := &testModel{}

		opts := newOptsWithParams(testConstants, nil)

		rule, err := parseRule(test.Expr, model, opts)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		ctx := NewContext(event)

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
			uid:  44,
			gid:  44,
			name: "aaa",
		},
	}

	event.process.list = list.New()
	event.process.list.PushBack(&testItem{key: 10, value: "AA"})

	tests := []struct {
		Expr      string
		Evaluated func() bool
	}{
		{Expr: `process.list.key == 44 && process.gid == 55`, Evaluated: func() bool { return event.listEvaluated }},
		{Expr: `process.gid == 55 && process.list[A].key == 44`, Evaluated: func() bool { return event.listEvaluated }},
		{Expr: `process.uid in [66, 77, 88] && process.gid == 55`, Evaluated: func() bool { return event.uidEvaluated }},
		{Expr: `process.gid == 55 && process.uid in [66, 77, 88]`, Evaluated: func() bool { return event.uidEvaluated }},
		{Expr: `process.list.value == "AA" && process.name == "zzz"`, Evaluated: func() bool { return event.listEvaluated }},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		_, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
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
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\nnow: %v, create_at: %v\n%s", test.Expected, result, time.Now().UnixNano(), event.process.createdAt, test.Expr)
		}
	}
}

func parseCIDR(t *testing.T, ip string) net.IPNet {
	ipnet, err := ParseCIDR(ip)
	if err != nil {
		t.Error(err)
	}
	return *ipnet
}

func TestIPv4(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("192.168.0.1/24")
	var cidrs []net.IPNet
	for _, cidrStr := range []string{"192.168.0.1/24", "10.0.0.1/16"} {
		_, cidrTmp, _ := net.ParseCIDR(cidrStr)
		cidrs = append(cidrs, *cidrTmp)
	}

	event := &testEvent{
		network: testNetwork{
			ip:    parseCIDR(t, "192.168.0.1"),
			ips:   []net.IPNet{parseCIDR(t, "192.168.0.1"), parseCIDR(t, "192.169.0.1")},
			cidr:  *cidr,
			cidrs: cidrs,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `192.168.0.1 == 192.168.0.1`, Expected: true},
		{Expr: `192.168.0.1 == 192.168.0.2`, Expected: false},
		{Expr: `192.168.0.15 in 192.168.0.1/24`, Expected: true},
		{Expr: `192.168.0.16 not in 192.168.1.1/24`, Expected: true},
		{Expr: `192.168.0.16/16 in 192.168.1.1/8`, Expected: true},
		{Expr: `192.168.0.16/16 allin 192.168.1.1/8`, Expected: true},
		{Expr: `193.168.0.16/16 in 192.168.1.1/8`, Expected: false},
		{Expr: `network.ip == 192.168.0.1`, Expected: true},
		{Expr: `network.ip == 127.0.0.1`, Expected: false},
		{Expr: `network.ip == ::ffff:192.168.0.1`, Expected: true},
		{Expr: `network.ip == ::ffff:127.0.0.1`, Expected: false},
		{Expr: `network.ip in 192.168.0.1/32`, Expected: true},
		{Expr: `network.ip in 0.0.0.0/0`, Expected: true},
		{Expr: `network.ip in ::1/0`, Expected: false},
		{Expr: `network.ip in 192.168.4.0/16`, Expected: true},
		{Expr: `network.ip in 192.168.4.0/24`, Expected: false},
		{Expr: `network.ip not in 192.168.4.0/16`, Expected: false},
		{Expr: `network.ip not in 192.168.4.0/24`, Expected: true},
		{Expr: `network.ip in ::ffff:192.168.4.0/112`, Expected: true},
		{Expr: `network.ip in ::ffff:192.168.4.0/120`, Expected: false},
		{Expr: `network.ip in [ 127.0.0.1, 192.168.0.1, 10.0.0.1 ]`, Expected: true},
		{Expr: `network.ip in [ 127.0.0.1, 10.0.0.1 ]`, Expected: false},
		{Expr: `network.ip in [ 192.168.4.1/16, 10.0.0.1/32 ]`, Expected: true},
		{Expr: `network.ip in [ 10.0.0.1, 127.0.0.1, 192.169.4.1/16 ]`, Expected: false},
		{Expr: `network.ip in [ 10.0.0.1, 127.0.0.1, 192.169.4.1/16, ::ffff:192.168.0.1/128 ]`, Expected: true},
		{Expr: `192.168.0.1 in [ 10.0.0.1, 127.0.0.1, 192.169.4.1/16, ::ffff:192.168.0.1/128 ]`, Expected: true},
		{Expr: `192.168.0.1/24 in [ 10.0.0.1, 127.0.0.1, 192.169.4.1/16, ::ffff:192.168.0.1/120 ]`, Expected: true},
		{Expr: `192.168.0.1/24 allin [ 10.0.0.1, 127.0.0.1, 192.169.4.1/16, ::ffff:192.168.0.1/120 ]`, Expected: true},

		{Expr: `network.ips in 192.168.0.0/16`, Expected: true},
		{Expr: `network.ips not in 192.168.0.0/16`, Expected: false},
		{Expr: `network.ips allin 192.168.0.0/16`, Expected: false},
		{Expr: `network.ips allin 192.168.0.0/8`, Expected: true},
		{Expr: `network.ips in [ 192.168.0.0/32, 193.168.0.0/16, ::ffff:192.168.0.1 ]`, Expected: true},
		{Expr: `network.ips not in [ 192.168.0.0/32, 193.168.0.0/16 ]`, Expected: true},
		{Expr: `network.ips allin [ 192.168.0.0/8, 0.0.0.0/0 ]`, Expected: true},
		{Expr: `network.ips allin [ 192.168.0.0/8, 1.0.0.0/8 ]`, Expected: false},
		{Expr: `192.0.0.0/8 allin network.ips`, Expected: true},

		{Expr: `network.cidr in 192.168.0.0/8`, Expected: true},
		{Expr: `network.cidr in 193.168.0.0/8`, Expected: false},
		{Expr: `network.cidrs in 10.0.0.1/8`, Expected: true},
		{Expr: `network.cidrs allin 10.0.0.1/8`, Expected: false},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%v` not found, got `%v`, expression: %s", test.Expected, result, test.Expr)
		}
	}
}

func TestIPv6(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("2001:0:0eab:dead::a0:abcd:4e/112")
	var cidrs []net.IPNet
	for _, cidrStr := range []string{"2001:0:0eab:dead::a0:abcd:4e/112", "2001:0:0eab:c00f::a0:abcd:4e/64"} {
		_, cidrTmp, _ := net.ParseCIDR(cidrStr)
		cidrs = append(cidrs, *cidrTmp)
	}
	event := &testEvent{
		network: testNetwork{
			ip:    parseCIDR(t, "2001:0:0eab:dead::a0:abcd:4e"),
			ips:   []net.IPNet{parseCIDR(t, "2001:0:0eab:dead::a0:abcd:4e"), parseCIDR(t, "2001:0:0eab:dead::a0:abce:4e")},
			cidr:  *cidr,
			cidrs: cidrs,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `2001:0:0eab:dead::a0:abcd:4e == 2001:0:0eab:dead::a0:abcd:4e`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e == 2001:0:0eab:dead::a0:abcd:4f`, Expected: false},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e in 2001:0:0eab:dead::a0:abcd:0/120`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e not in 2001:0:0eab:dead::a0:abcd:ab00/120`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e/64 in 2001:0:0eab:dead::a0:abcd:1b00/32`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e/64 allin 2001:0:0eab:dead::a0:abcd:1b00/32`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e/64 in [ 2001:0:0eab:dead::a0:abcd:1b00/32 ]`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e/32 in [ 2001:0:0eab:dead::a0:abcd:1b00/64 ]`, Expected: true},
		{Expr: `network.ip == 2001:0:0eab:dead::a0:abcd:4e`, Expected: true},
		{Expr: `network.ip == ::1`, Expected: false},
		{Expr: `network.ip == 127.0.0.1`, Expected: false},
		{Expr: `network.ip in 0.0.0.0/0`, Expected: false},
		{Expr: `network.ip in ::1/0`, Expected: true},
		{Expr: `network.ip in 2001:0:0eab:dead::a0:abcd:4e/128`, Expected: true},
		{Expr: `network.ip in 2001:0:0eab:dead::a0:abcd:0/112`, Expected: true},
		{Expr: `network.ip in 2001:0:0eab:dead::a0:0:0/112`, Expected: false},
		{Expr: `network.ip not in 2001:0:0eab:dead::a0:0:0/112`, Expected: true},
		{Expr: `network.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 2001:0:0eab:dead::a0:abcd:4f ]`, Expected: true},
		{Expr: `network.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4f ]`, Expected: false},
		{Expr: `network.ip in [ 2001:0:0eab:dead::a0:abcd:0/112, 2001:0:0eab:dead::a0:abcd:4f/128 ]`, Expected: true},
		{Expr: `network.ip in [ ::1, 2001:124:0eab:dead::a0:abcd:4f, 2001:0:0eab:dead::a0:abcd:0/112 ]`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e in [ 2001:0:0eab:dead::a0:abcd:4e, ::1, 2002:0:0eab:dead::/64, ::ffff:192.168.0.1/128 ]`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e/64 in [ 10.0.0.1, 127.0.0.1, 2001:0:0eab:dead::a0:abcd:1b00/32, ::ffff:192.168.0.1/120 ]`, Expected: true},
		{Expr: `2001:0:0eab:dead::a0:abcd:4e/64 allin [ 10.0.0.1, 127.0.0.1, 2001:0:0eab:dead::a0:abcd:1b00/32, ::ffff:192.168.0.1/120 ]`, Expected: true},

		{Expr: `network.ips in 2001:0:0eab:dead::a0:abcd:0/120`, Expected: true},
		{Expr: `network.ips not in 2001:0:0eab:dead::a0:abcd:0/120`, Expected: false},
		{Expr: `network.ips allin 2001:0:0eab:dead::a0:abcd:0/120`, Expected: false},
		{Expr: `network.ips allin 2001:0:0eab:dead::a0:abcd:0/104`, Expected: true},
		{Expr: `network.ips in [ 2001:0:0eab:dead::a0:abcd:0/128, 2001:0:0eab:dead::a0:abcd:0/120, 2001:0:0eab:dead::a0:abce:4e ]`, Expected: true},
		{Expr: `network.ips not in [ 2001:0:0eab:dead::a0:abcd:0/128, 2001:0:0eab:dead::a0:abcf:4e/120 ]`, Expected: true},
		{Expr: `network.ips allin [ 2001:0:0eab:dead::a0:abcd:0/104, 2001::1/16 ]`, Expected: true},
		{Expr: `network.ips allin [ 2001:0:0eab:dead::a0:abcd:0/104, 2002::1/16 ]`, Expected: false},
		{Expr: `2001:0:0eab:dead::a0:abcd:0/104 allin network.ips`, Expected: true},

		{Expr: `network.cidr in 2001:0:0eab:dead::a0:abcd:4e/112`, Expected: true},
		{Expr: `network.cidr in 2002:0:0eab:dead::a0:abcd:4e/72`, Expected: false},
		{Expr: `network.cidrs in 2001:0:0eab:dead::a0:abcd:4e/64`, Expected: true},
		{Expr: `network.cidrs allin 2001:0:0eab:dead::a0:abcd:4e/64`, Expected: false},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%v` not found, got `%v`, expression: %s", test.Expected, result, test.Expr)
		}
	}
}

func TestOpOverrides(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			orName: "abc",
		},
	}

	// values that will be returned by the operator override
	event.process.orNameValues = func() *StringValues {
		var values StringValues
		values.AppendScalarValue("abc")

		if err := values.Compile(DefaultStringCmpOpts); err != nil {
			return nil
		}

		return &values
	}

	event.process.orArray = []*testItem{
		{key: 1000, value: "abc", flag: true},
	}

	// values that will be returned by the operator override
	event.process.orArrayValues = func() *StringValues {
		var values StringValues
		values.AppendScalarValue("abc")

		if err := values.Compile(DefaultStringCmpOpts); err != nil {
			return nil
		}

		return &values
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `process.or_name == "not"`, Expected: true},
		{Expr: `process.or_name != "not"`, Expected: false},
		{Expr: `process.or_name in ["not"]`, Expected: true},
		{Expr: `process.or_array.value == "not"`, Expected: true},
		{Expr: `process.or_array.value in ["not"]`, Expected: true},
		{Expr: `process.or_array.value not in ["not"]`, Expected: false},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func TestOpOverridePartials(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			orName: "abc",
		},
	}

	// values that will be returned by the operator override
	event.process.orNameValues = func() *StringValues {
		var values StringValues
		values.AppendScalarValue("abc")

		if err := values.Compile(DefaultStringCmpOpts); err != nil {
			return nil
		}

		return &values
	}

	event.process.orArray = []*testItem{
		{key: 1000, value: "abc", flag: true},
	}

	// values that will be returned by the operator override
	event.process.orArrayValues = func() *StringValues {
		var values StringValues
		values.AppendScalarValue("abc")

		if err := values.Compile(DefaultStringCmpOpts); err != nil {
			return nil
		}

		return &values
	}

	tests := []struct {
		Expr        string
		Field       string
		IsDiscarder bool
	}{
		{Expr: `process.or_name == "not"`, Field: "process.or_name", IsDiscarder: false},
		{Expr: `process.or_name != "not"`, Field: "process.or_name", IsDiscarder: true},
		{Expr: `process.or_name != "not" || true`, Field: "process.or_name", IsDiscarder: false},
		{Expr: `process.or_name in ["not"]`, Field: "process.or_name", IsDiscarder: false},
		{Expr: `process.or_array.value == "not"`, Field: "process.or_array.value", IsDiscarder: false},
		{Expr: `process.or_array.value in ["not"]`, Field: "process.or_array.value", IsDiscarder: false},
		{Expr: `process.or_array.value not in ["not"]`, Field: "process.or_array.value", IsDiscarder: true},
		{Expr: `process.or_array.value not in ["not"] || true`, Field: "process.or_array.value", IsDiscarder: false},
	}

	for _, test := range tests {
		model := &testModel{}

		opts := newOptsWithParams(testConstants, nil)

		rule, err := parseRule(test.Expr, model, opts)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		ctx := NewContext(event)

		result, err := rule.PartialEval(ctx, test.Field)
		if err != nil {
			t.Fatalf("error while partial evaluating `%s` for `%s`: %s", test.Expr, test.Field, err)
		}

		if !result != test.IsDiscarder {
			t.Fatalf("expected result `%t` for `%s`, got `%t`\n%s", test.IsDiscarder, test.Field, result, test.Expr)
		}
	}
}

func TestMultipleOpOverrides(t *testing.T) {
	event := &testEvent{
		title: "hello world",
	}

	testCases := []struct {
		expression     string
		expectedResult bool
	}{
		{expression: `event.title == "hello world"`, expectedResult: true},
		{expression: `event.title == "Hello World"`, expectedResult: true},
		{expression: `event.title == "HELLO WORLD"`, expectedResult: true},
		{expression: `event.title == "hellO worlD"`, expectedResult: false},
		{expression: `"hello world" == event.title`, expectedResult: true},
		{expression: `"Hello World" == event.title`, expectedResult: true},
		{expression: `"HELLO WORLD" == event.title`, expectedResult: true},
		{expression: `"hellO worlD" == event.title`, expectedResult: false},
	}

	for _, tc := range testCases {
		ctx := NewContext(event)
		result, _, err := eval(ctx, tc.expression)
		if err != nil {
			t.Errorf("error while evaluating `%s`: %s", tc.expression, err)
			continue
		}
		assert.Equal(t, tc.expectedResult, result, "expression: `%s`", tc.expression)
	}
}

func TestFieldValues(t *testing.T) {
	tests := []struct {
		Expr     string
		Field    string
		Expected FieldValue
	}{
		{Expr: `process.name == "/proc/1/maps"`, Field: "process.name", Expected: FieldValue{Value: "/proc/1/maps", Type: ScalarValueType}},
		{Expr: `process.name =~ "/proc/1/*"`, Field: "process.name", Expected: FieldValue{Value: "/proc/1/*", Type: GlobValueType}},
		{Expr: `process.name =~ r"/proc/1/.*"`, Field: "process.name", Expected: FieldValue{Value: "/proc/1/.*", Type: RegexpValueType}},
		{Expr: `process.name == "/proc/${pid}/maps"`, Field: "process.name", Expected: FieldValue{Value: "/proc/${pid}/maps", Type: VariableValueType}},
		{Expr: `open.filename =~ "/proc/1/*"`, Field: "open.filename", Expected: FieldValue{Value: "/proc/1/*", Type: PatternValueType}},
	}

	for _, test := range tests {
		model := &testModel{}

		opts := newOptsWithParams(testConstants, nil)

		rule, err := parseRule(test.Expr, model, opts)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}
		values := rule.GetFieldValues(test.Field)
		if len(values) != 1 {
			t.Fatalf("expected field value not found: %+v", test.Expected)
		}
		if values[0].Type != test.Expected.Type || values[0].Value != test.Expected.Value {
			t.Errorf("field values differ %+v != %+v", test.Expected, values[0])
		}
	}
}

func TestFieldReferenceSyntax(t *testing.T) {
	// Test the new %{field} syntax for explicit field references
	event := &testEvent{
		process: testProcess{
			name: "test",
			uid:  1000,
			pid:  42,
		},
		open: testOpen{
			filename: "/tmp/test",
		},
	}

	tests := []struct {
		Expr        string
		Expected    bool
		ShouldError bool
		Desc        string
	}{
		{
			Expr:     `%{process.name} == "test"`,
			Expected: true,
			Desc:     "field reference with %{} syntax",
		},
		{
			Expr:     `%{process.uid} == 1000`,
			Expected: true,
			Desc:     "numeric field with %{} syntax",
		},
		{
			Expr:     `"%{process.name}" == "test"`,
			Expected: true,
			Desc:     "field reference in string interpolation",
		},
		{
			Expr:     `"%{process.name}:%{process.uid}" == "test:1000"`,
			Expected: true,
			Desc:     "multiple field references in string",
		},
		{
			Expr:        `${process.pid} > 0`,
			ShouldError: true,
			Desc:        "variable syntax without variable should fail (no fallback)",
		},
		{
			Expr:        `${nonexistent.variable} == "test"`,
			ShouldError: true,
			Desc:        "nonexistent variable should fail",
		},
		{
			Expr:        `%{nonexistent.field} == "test"`,
			ShouldError: true,
			Desc:        "nonexistent field should fail",
		},
	}

	for _, test := range tests {
		t.Run(test.Desc, func(t *testing.T) {
			ctx := NewContext(event)

			result, _, err := eval(ctx, test.Expr)
			if test.ShouldError {
				if err == nil {
					t.Fatalf("expected error for `%s` but got none", test.Expr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error while evaluating `%s`: %s", test.Expr, err)
			}

			if result != test.Expected {
				t.Fatalf("expected result `%t` for `%s`, got `%t`", test.Expected, test.Expr, result)
			}
		})
	}
}

func TestArithmeticOperation(t *testing.T) {
	// time reliability issue
	if runtime.GOARCH == "386" && runtime.GOOS == "windows" {
		t.Skip()
	}

	now := time.Now().UnixNano()

	event := &testEvent{
		process: testProcess{
			name:      "ls",
			createdAt: now,
		},
		open: testOpen{
			openedAt: now + int64(time.Second*2),
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `1 + 2 == 5 - 2 && process.name == "ls"`, Expected: true},
		{Expr: `1 + 2 != 3 && process.name == "ls"`, Expected: false},
		{Expr: `1 + 2 - 3 + 4  == 4 && process.name == "ls"`, Expected: true},
		{Expr: `1 - 2 + 3 - (1 - 4) - (1 - 5) == 9 &&  process.name == "ls"`, Expected: true},
		{Expr: `10s + 40s == 50s && process.name == "ls"`, Expected: true},
		{Expr: `process.created_at < 5s && process.name == "ls"`, Expected: true},
		{Expr: `open.opened_at - process.created_at + 3s <= 5s && process.name == "ls"`, Expected: true},
		{Expr: `open.opened_at - process.created_at + 3s <= 1s && process.name == "ls"`, Expected: false},
	}

	for _, test := range tests {
		ctx := NewContext(event)

		result, _, err := eval(ctx, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		if result != test.Expected {
			t.Errorf("expected result `%t` not found, got `%t`\n%s", test.Expected, result, test.Expr)
		}
	}
}

func decorateRuleExpr(m *MatchingSubExpr, expr string, before, after string) (string, error) {
	a, b := m.ValueA.getPosWithinRuleExpr(expr, m.Offset), m.ValueB.getPosWithinRuleExpr(expr, m.Offset)

	if a.Offset+a.Length > len(expr) || b.Offset+b.Length > len(expr) {
		return expr, errors.New("expression overflow")
	}

	if b.Offset < a.Offset {
		tmp := b
		b = a
		a = tmp
	}

	if a.Length == 0 {
		return expr[:b.Offset] + before + expr[b.Offset:b.Offset+b.Length] + after + expr[b.Offset+b.Length:], nil
	}

	if b.Length == 0 {
		return expr[0:a.Offset] + before + expr[a.Offset:a.Offset+a.Length] + after + expr[a.Offset+a.Length:], nil
	}

	return expr[0:a.Offset] + before + expr[a.Offset:a.Offset+a.Length] + after +
		expr[a.Offset+a.Length:b.Offset] + before + expr[b.Offset:b.Offset+b.Length] + after +
		expr[b.Offset+b.Length:], nil
}

func decorateRuleExprs(m *MatchingSubExprs, expr string, before, after string) (string, error) {
	var err error

	dejavu := make(map[int]bool)

	for _, mse := range *m {
		if dejavu[mse.Offset] {
			return "", errors.New("duplicate offset")
		}
		dejavu[mse.Offset] = true

		expr, err = decorateRuleExpr(&mse, expr, before, after)
		if err != nil {
			return expr, err
		}
	}
	return expr, nil
}

func TestMatchingSubExprs(t *testing.T) {
	event := &testEvent{
		process: testProcess{
			name:   "ls",
			argv0:  "-al",
			uid:    22,
			isRoot: true,
			pid:    os.Getpid(),
			gid:    3,
		},
		network: testNetwork{
			ip:  parseCIDR(t, "192.168.0.1"),
			ips: []net.IPNet{parseCIDR(t, "192.168.0.2"), parseCIDR(t, "192.168.0.3")},
		},
	}
	event.process.list = list.New()
	event.process.list.PushBack(&testItem{key: 10, value: "AAA"})
	event.process.list.PushBack(&testItem{key: 11, value: "BBB"})

	tests := []struct {
		Expr     string
		Expected string
	}{
		{Expr: `true && process.name == "ls"`, Expected: `true && <b>process.name</b> == <b>"ls"</b>`},
		{Expr: `true && "ls" == process.name`, Expected: `true && <b>"ls"</b> == <b>process.name</b>`},
		{Expr: `true && process.name == "gzip"`, Expected: `true && process.name == "gzip"`},
		{Expr: `true && process.name == process.name`, Expected: `true && <b>process.name</b> == <b>process.name</b>`},
		{Expr: `true && process.name in ["ls"]`, Expected: `true && <b>process.name</b> in [<b>"ls"</b>]`},
		{Expr: `true && process.name in ["touch", "ls", "date"]`, Expected: `true && <b>process.name</b> in ["touch", <b>"ls"</b>, "date"]`},
		{Expr: `true && process.name =~ "*ls*"`, Expected: `true && <b>process.name</b> =~ <b>"*ls*"</b>`},
		{Expr: `true && process.name == ~"*ls*"`, Expected: `true && <b>process.name</b> == ~<b>"*ls*"</b>`},
		{Expr: `true && process.name == "ls" && process.name == "date"`, Expected: `true && <b>process.name</b> == <b>"ls"</b> && process.name == "date"`},
		{Expr: `true && process.name == "ls" && process.name =~ "*ls*"`, Expected: `true && <b>process.name</b> == <b>"ls"</b> && <b>process.name</b> =~ <b>"*ls*"</b>`},
		{Expr: `true && process.name in [~"*ls*"]`, Expected: `true && <b>process.name</b> in [~<b>"*ls*"</b>]`},
		{Expr: `process.argv0 == "-al" && process.name in [~"*ls*"]`, Expected: `<b>process.argv0</b> == <b>"-al"</b> && <b>process.name</b> in [~<b>"*ls*"</b>]`},
		{Expr: `true && process.name == r".*ls.*"`, Expected: `true && <b>process.name</b> == r<b>".*ls.*"</b>`},
		{Expr: `true && process.name in [~"*ls*", "gzip", r".*ls"]`, Expected: `true && <b>process.name</b> in [~<b>"*ls*"</b>, "gzip", r".*ls"]`},
		{Expr: `true && process.name in ["gzip", r".*ls"]`, Expected: `true && <b>process.name</b> in ["gzip", r<b>".*ls"</b>]`},
		{Expr: `true && process.uid == 22`, Expected: `true && <b>process.uid</b> == <b>22</b>`},
		{Expr: `true && process.uid >= 22`, Expected: `true && <b>process.uid</b> >= <b>22</b>`},
		{Expr: `true && process.uid in [66, 22]`, Expected: `true && <b>process.uid</b> in [66, <b>22</b>]`},
		{Expr: `true && process.is_root`, Expected: `true && <b>process.is_root</b>`},
		{Expr: `true && process.is_root == true`, Expected: `true && <b>process.is_root</b> == <b>true</b>`},
		{Expr: `false || process.is_root == true`, Expected: `false || <b>process.is_root</b> == <b>true</b>`},
		{Expr: `false || process.is_root`, Expected: `false || <b>process.is_root</b>`},
		{Expr: `true && process.list.key == 10`, Expected: `true && <b>process.list.key</b> == <b>10</b>`},
		{Expr: `true && 10 == process.list.key`, Expected: `true && <b>10</b> == <b>process.list.key</b>`},
		{Expr: `true && 10 in process.list.key`, Expected: `true && <b>10</b> in <b>process.list.key</b>`},
		{Expr: `true && process.list.key in [10, 11]`, Expected: `true && <b>process.list.key</b> in [<b>10</b>, 11]`},
		{Expr: `true && process.list.value == "AAA"`, Expected: `true && <b>process.list.value</b> == <b>"AAA"</b>`},
		{Expr: `true && process.list.value in ["CCC", "BBB"]`, Expected: `true && <b>process.list.value</b> in ["CCC", <b>"BBB"</b>]`},
		{Expr: `true && process.list.value in ["CCC", ~"*BBB*"]`, Expected: `true && <b>process.list.value</b> in ["CCC", ~<b>"*BBB*"</b>]`},
		{Expr: `network.ip == 192.168.0.1`, Expected: `<b>network.ip</b> == <b>192.168.0.1</b>`},
		{Expr: `network.ip == 192.168.0.1/32`, Expected: `<b>network.ip</b> == <b>192.168.0.1/32</b>`},
		{Expr: `192.168.0.1 == network.ip`, Expected: `<b>192.168.0.1</b> == <b>network.ip</b>`},
		{Expr: `192.168.0.1/32 == network.ip`, Expected: `<b>192.168.0.1/32</b> == <b>network.ip</b>`},
		{Expr: `network.ip == 192.168.0.0/24`, Expected: `<b>network.ip</b> == <b>192.168.0.0/24</b>`},
		{Expr: `network.ip in [127.0.0.1, 192.168.0.1, 10.0.0.1]`, Expected: `<b>network.ip</b> in [127.0.0.1, <b>192.168.0.1</b>, 10.0.0.1]`},
		{Expr: `network.ips in [192.168.1.33, 192.168.0.3]`, Expected: `<b>network.ips</b> in [192.168.1.33, <b>192.168.0.3</b>]`},
		{Expr: `network.ips allin [192.168.0.0/16, 192.168.0.0/24]`, Expected: `<b>network.ips</b> allin [<b>192.168.0.0/16, 192.168.0.0/24</b>]`},
		{Expr: `process.name in [~"aaals*", ~"ls*"]`, Expected: `<b>process.name</b> in [~"aaals*", ~<b>"ls*"</b>]`},
		{Expr: `process.name in [~"aaals*", ~"ls*"]`, Expected: `<b>process.name</b> in [~"aaals*", ~<b>"ls*"</b>]`},

		// need to add varname in the evaluators
		//{Expr: `true && process.pid == ${pid}`, Expected: `true && <b>process.pid</b> == ${pid}`},

		// need to handle bitmask
		//{Expr: `process.gid & 1 > 0`, Expected: `<b>process.gid</b> & 1 > 0`},
	}

	for _, test := range tests {
		t.Run(test.Expr, func(t *testing.T) {
			ctx := NewContext(event)

			res, _, err := eval(ctx, test.Expr)
			if err != nil {
				t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
			}

			subExprs := ctx.GetMatchingSubExprs()

			decorated, err := decorateRuleExprs(&subExprs, test.Expr, "<b>", "</b>")
			if test.Expected != decorated {
				t.Errorf("rule decoration error : %s vs %s => %v : %v", test.Expected, decorated, res, err)
			}
		})
	}
}

func FuzzEval(f *testing.F) {
	// Add SECL expressions from all test cases as seeds

	// TestStringError
	f.Add(`process.name != "/usr/bin/vipw" && process.uid != 0 && open.filename == 3`)
	f.Add(`process.name != "/usr/bin/vipw" && process.uid != "test" && Open.Filename == "/etc/shadow"`)
	f.Add(`(process.name != "/usr/bin/vipw") == "test"`)

	// TestSimpleString
	f.Add(`process.name != ""`)
	f.Add(`process.name != "/usr/bin/vipw"`)
	f.Add(`process.name != "/usr/bin/cat"`)
	f.Add(`process.name == "/usr/bin/cat"`)
	f.Add(`process.name == "/usr/bin/vipw"`)
	f.Add(`(process.name == "/usr/bin/cat" && process.uid == 0) && (process.name == "/usr/bin/cat" && process.uid == 0)`)
	f.Add(`(process.name == "/usr/bin/cat" && process.uid == 1) && (process.name == "/usr/bin/cat" && process.uid == 1)`)

	// TestSimpleInt
	f.Add(`111 != 555`)
	f.Add(`process.uid != 555`)
	f.Add(`process.uid != 444`)
	f.Add(`process.uid == 444`)
	f.Add(`process.uid == 555`)
	f.Add(`--3 == 3`)
	f.Add(`3 ^ 3 == 0`)
	f.Add(`^0 == -1`)

	// TestSimpleBool
	f.Add(`(444 == 444) && ("test" == "test")`)
	f.Add(`(444 == 444) and ("test" == "test")`)
	f.Add(`(444 != 444) && ("test" == "test")`)
	f.Add(`(444 != 555) && ("test" == "test")`)
	f.Add(`(444 != 555) && ("test" != "aaaa")`)

	// TestPrecedence
	f.Add(`false || (true != true)`)
	f.Add(`false || true`)
	f.Add(`false or true`)
	f.Add(`1 == 1 & 1`)
	f.Add(`not true && false`)
	f.Add(`not (true && false)`)

	// TestParenthesis
	f.Add(`(true) == (true)`)

	// TestSimpleBitOperations
	f.Add(`(3 & 3) == 3`)
	f.Add(`(3 & 1) == 3`)
	f.Add(`(2 | 1) == 3`)
	f.Add(`(3 & 1) != 0`)
	f.Add(`0 != 3 & 1`)
	f.Add(`(3 ^ 3) == 0`)

	// TestStringMatcher
	f.Add(`process.name =~ "/usr/bin/c$t/test/*"`)
	f.Add(`process.name =~ "/usr/bin/c$t*"`)
	f.Add(`process.name =~ "/usr/bin/c*"`)
	f.Add(`process.name =~ "/usr/bin/**"`)
	f.Add(`process.name =~ "/usr/**"`)
	f.Add(`process.name =~ "/**"`)
	f.Add(`process.name =~ "/usr/bin/*"`)
	f.Add(`process.name !~ "/usr/sbin/*"`)
	f.Add(`process.name == ~"/usr/bin/*"`)
	f.Add(`process.name =~ ~"/usr/bin/*"`)
	f.Add(`process.name =~ r".*/bin/.*"`)
	f.Add(`process.name =~ r".*/[usr]+/bin/.*"`)
	f.Add(`process.name == r".*/bin/.*"`)
	f.Add(`r".*/bin/.*" == process.name`)
	f.Add(`process.argv0 =~ "http://*"`)
	f.Add(`process.argv0 =~ "*example.com"`)

	// TestVariables
	f.Add(`process.name == "/proc/${pid}/maps/${str}"`)
	f.Add(`process.pid == ${pid}`)

	// TestInArray
	f.Add(`"a" in [ "a", "b", "c" ]`)
	f.Add(`process.name in [ "c", "b", "aaa" ]`)
	f.Add(`"d" in [ "aaa", "b", "c" ]`)
	f.Add(`"aaa" not in [ "aaa", "b", "c" ]`)
	f.Add(`process.name not in [ "c", "b", "aaa" ]`)
	f.Add(`3 in [ 1, 2, 3 ]`)
	f.Add(`process.uid in [ 1, 2, 3 ]`)
	f.Add(`4 not in [ 1, 2, 3 ]`)
	f.Add(`process.name in [ ~"*a*" ]`)
	f.Add(`process.name in [ ~"*d*", "aaa" ]`)
	f.Add(`process.name in [ ~"*d*", ~"aa*" ]`)
	f.Add(`process.name in [ r".*d.*", r"aa.*" ]`)
	f.Add(`process.name not in [ r".*d.*", r"ab.*" ]`)
	f.Add(`retval in [ EPERM, EACCES, EPFNOSUPPORT ]`)

	// TestComplex
	f.Add(`open.filename =~ "/var/lib/httpd/*" && open.flags & (O_CREAT | O_TRUNC | O_EXCL | O_RDWR | O_WRONLY) > 0`)

	// TestPartial
	f.Add(`true || process.name == "/usr/bin/cat"`)
	f.Add(`false || process.name == "/usr/bin/cat"`)
	f.Add(`true && process.name == "abc"`)
	f.Add(`false && process.name == "abc"`)
	f.Add(`open.filename == "test1" && process.name == "/usr/bin/cat"`)
	f.Add(`open.filename == "test1" && process.name != "/usr/bin/cat"`)
	f.Add(`open.filename == "test1" || process.name == "/usr/bin/cat"`)
	f.Add(`open.filename == "test1" && !(process.name == "/usr/bin/cat")`)
	f.Add(`open.filename == "test1" && process.name =~ "ab*"`)
	f.Add(`open.filename == "test1" && process.name == open.filename`)
	f.Add(`open.filename in [ "test1", "test2" ] && process.name == "abc"`)
	f.Add(`!(open.filename in [ "test1", "test2" ]) && process.name == "abc"`)
	f.Add(`open.filename == open.filename`)
	f.Add(`open.filename != open.filename`)
	f.Add(`open.filename == "test1" && process.uid == 123`)
	f.Add(`open.filename == "test1" && process.is_root`)
	f.Add(`process.uid & (1 | 1024) == 1`)
	f.Add(`process.name == "abc" && ^process.uid != 0`)

	// TestConstants
	f.Add(`retval in [ EPERM, EACCES ]`)
	f.Add(`open.filename in [ my_constant_1, my_constant_2 ]`)
	f.Add(`process.is_root in [ true, false ]`)

	// TestRegister
	f.Add(`process.list[A].key == 10`)
	f.Add(`process.list[A].key != 10`)
	f.Add(`process.list[A].key >= 200`)
	f.Add(`process.list[A].key > 100`)
	f.Add(`10 == process.list[A].key`)
	f.Add(`10 != process.list[A].key`)
	f.Add(`10 in process.list[A].key`)
	f.Add(`10 not in process.list[A].key`)
	f.Add(`process.array[A].flag == true`)
	f.Add(`"AAA" in process.list[A].value`)
	f.Add(`"AAA" not in process.list[A].value`)
	f.Add(`~"AA*" in process.list[A].value`)
	f.Add(`process.list[A].value == ~"AA*"`)
	f.Add(`process.list[A].value =~ "AA*"`)
	f.Add(`process.list[A].value in ["~zzzz", ~"AA*", "nnnnn"]`)
	f.Add(`process.list[A].key == 10 && process.list[A].value == "AAA"`)
	f.Add(`process.array[A].key == 1000 && process.array[A].value == "EEEE"`)

	// TestDuration
	f.Add(`process.created_at < 2s`)
	f.Add(`process.created_at > 2s`)

	// TestIPv4
	f.Add(`192.168.0.1 == 192.168.0.1`)
	f.Add(`192.168.0.15 in 192.168.0.1/24`)
	f.Add(`192.168.0.16 not in 192.168.1.1/24`)
	f.Add(`192.168.0.16/16 allin 192.168.1.1/8`)
	f.Add(`network.ip == 192.168.0.1`)
	f.Add(`network.ip == ::ffff:192.168.0.1`)
	f.Add(`network.ip in 192.168.0.1/32`)
	f.Add(`network.ip in 0.0.0.0/0`)
	f.Add(`network.ip in [ 127.0.0.1, 192.168.0.1, 10.0.0.1 ]`)
	f.Add(`network.ips in 192.168.0.0/16`)
	f.Add(`network.ips allin 192.168.0.0/8`)
	f.Add(`network.cidr in 192.168.0.0/8`)

	// TestIPv6
	f.Add(`2001:0:0eab:dead::a0:abcd:4e == 2001:0:0eab:dead::a0:abcd:4e`)
	f.Add(`2001:0:0eab:dead::a0:abcd:4e in 2001:0:0eab:dead::a0:abcd:0/120`)
	f.Add(`2001:0:0eab:dead::a0:abcd:4e/64 allin 2001:0:0eab:dead::a0:abcd:1b00/32`)
	f.Add(`network.ip == 2001:0:0eab:dead::a0:abcd:4e`)
	f.Add(`network.ip in ::1/0`)
	f.Add(`network.ip in 2001:0:0eab:dead::a0:abcd:0/112`)

	// TestOpOverrides
	f.Add(`process.or_name == "not"`)
	f.Add(`process.or_name != "not"`)
	f.Add(`process.or_name in ["not"]`)
	f.Add(`process.or_array.value == "not"`)

	// TestArithmeticOperation
	f.Add(`1 + 2 == 5 - 2 && process.name == "ls"`)
	f.Add(`1 + 2 != 3 && process.name == "ls"`)
	f.Add(`1 + 2 - 3 + 4  == 4 && process.name == "ls"`)
	f.Add(`1 - 2 + 3 - (1 - 4) - (1 - 5) == 9 &&  process.name == "ls"`)
	f.Add(`10s + 40s == 50s && process.name == "ls"`)
	f.Add(`process.created_at < 5s && process.name == "ls"`)
	f.Add(`open.opened_at - process.created_at + 3s <= 5s && process.name == "ls"`)

	// TestMatchingSubExprs
	f.Add(`true && process.name == "ls"`)
	f.Add(`true && "ls" == process.name`)
	f.Add(`true && process.name == process.name`)
	f.Add(`true && process.name in ["ls"]`)
	f.Add(`true && process.name =~ "*ls*"`)
	f.Add(`true && process.name == ~"*ls*"`)
	f.Add(`true && process.name == r".*ls.*"`)
	f.Add(`true && process.uid == 22`)
	f.Add(`true && process.uid >= 22`)
	f.Add(`true && process.is_root`)
	f.Add(`false || process.is_root`)
	f.Add(`network.ip == 192.168.0.1`)
	f.Add(`192.168.0.1 == network.ip`)
	f.Add(`network.ips allin [192.168.0.0/16, 192.168.0.0/24]`)

	// Benchmark expressions
	f.Add(`process.name == "/usr/bin/ls" && process.uid == 1`)
	f.Add(`process.name == "/usr/bin/ls" && process.uid != 0`)

	f.Fuzz(func(_ *testing.T, expr string) {
		commonFuzzEval(expr)
	})
}

func commonFuzzEval(expr string) {
	model := &testModel{}
	opts := newOptsWithParams(testConstants, nil)

	// Attempt to parse and evaluate the rule
	rule, err := parseRule(expr, model, opts)
	if err != nil {
		return
	}
	_ = rule
}

func TestDiscoveredByFuzz(t *testing.T) {
	exprs := []string{
		`"!"!=A`,
	}

	for _, expr := range exprs {
		t.Run(expr, func(_ *testing.T) {
			commonFuzzEval(expr)
		})
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
	opts := newOptsWithParams(nil, nil)

	rule, err := parseRule(expr, &testModel{}, opts)
	if err != nil {
		b.Fatalf("%s\n%s", err, expr)
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(event)
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
	opts := newOptsWithParams(nil, nil)

	rule, err := parseRule(expr, &testModel{}, opts)
	if err != nil {
		b.Fatalf("%s\n%s", err, expr)
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(event)
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

	ctx := NewContext(event)

	base := `(process.name == "/usr/bin/ls" && process.uid != 0)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")
	model := &testModel{}
	opts := newOptsWithParams(nil, nil)

	rule, err := parseRule(expr, model, opts)
	if err != nil {
		b.Fatal(err)
	}

	if err := rule.GenEvaluator(model); err != nil {
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
	opts := newOptsWithParams(nil, nil)

	rule, err := parseRule(expr, &testModel{}, opts)
	if err != nil {
		b.Fatalf("%s\n%s", err, expr)
	}

	evaluator := rule.GetEvaluator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := pool.Get(event)
		if evaluator.Eval(ctx) != true {
			b.Fatal("unexpected result")
		}
		pool.pool.Put(ctx)
	}
}
