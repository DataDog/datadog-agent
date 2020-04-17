package eval

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

func eval(t *testing.T, event *model.Event, expr string) (bool, *ast.Rule, error) {
	rule, err := ast.ParseRule(expr)
	if err != nil {
		t.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	ctx := &Context{Event: event}

	eval, err := RuleToEvaluator(rule, true)
	if err != nil {
		return false, rule, err
	}

	return eval(ctx), rule, nil
}

func TestStringError(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
			UID:  1,
		},
		Open: model.OpenSyscall{
			Pathname: "/etc/shadow",
		},
	}

	_, _, err := eval(t, event, `Process.Name != "/usr/bin/vipw" && Process.UID != 0 && Open.Pathname == 3`)
	if err == nil || err.(*AstToEvalError).Pos.Column != 73 {
		t.Fatal("should report a string type error")
	}
}

func TestIntError(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
			UID:  1,
		},
		Open: model.OpenSyscall{
			Pathname: "/etc/shadow",
		},
	}

	_, _, err := eval(t, event, `Process.Name != "/usr/bin/vipw" && Process.UID != "test" && Open.Pathname == "/etc/shadow"`)
	if err == nil || err.(*AstToEvalError).Pos.Column != 51 {
		t.Fatal("should report a string type error")
	}
}

func TestBoolError(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
			UID:  1,
		},
		Open: model.OpenSyscall{
			Pathname: "/etc/shadow",
		},
	}

	_, _, err := eval(t, event, `(Process.Name != "/usr/bin/vipw") == "test"`)
	if err == nil || err.(*AstToEvalError).Pos.Column != 38 {
		t.Fatal("should report a bool type error")
	}
}

/*
func TestSimpleString(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `Process.Name != "/usr/bin/vipw"`, Expected: true},
		{Expr: `Process.Name != "/usr/bin/cat"`, Expected: false},
		{Expr: `Process.Name == "/usr/bin/cat"`, Expected: true},
		{Expr: `Process.Name == "/usr/bin/vipw"`, Expected: false},
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
*/

func TestSimpleInt(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			UID: 444,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `111 != 555`, Expected: true},
		{Expr: `Process.UID != 555`, Expected: true},
		{Expr: `Process.UID != 444`, Expected: false},
		{Expr: `Process.UID == 444`, Expected: true},
		{Expr: `Process.UID == 555`, Expected: false},
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
	event := &model.Event{}

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

func TestSyscallConst(t *testing.T) {
	event := &model.Event{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: fmt.Sprintf(`%d == S_IEXEC`, syscall.S_IEXEC), Expected: true},
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
	event := &model.Event{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `false || (true != true)`, Expected: false},
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
	event := &model.Event{}

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
	event := &model.Event{}

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
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `Process.Name =~ "/usr/bin/*"`, Expected: true},
		{Expr: `Process.Name =~ "/usr/sbin/*"`, Expected: false},
		{Expr: `Process.Name !~ "/usr/sbin/*"`, Expected: true},
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
	event := &model.Event{}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `"a" in [ "a", "b", "c" ]`, Expected: true},
		{Expr: `"a" in [ "c", "b", "a" ]`, Expected: true},
		{Expr: `"d" in [ "a", "b", "c" ]`, Expected: false},
		{Expr: `"d" in [ "c", "b", "a" ]`, Expected: false},
		{Expr: `"a" not in [ "a", "b", "c" ]`, Expected: false},
		{Expr: `"a" not in [ "c", "b", "a" ]`, Expected: false},
		{Expr: `"d" not in [ "a", "b", "c" ]`, Expected: true},
		{Expr: `"d" not in [ "c", "b", "a" ]`, Expected: true},
		{Expr: `3 in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `3 in [ 1, 2, 3 ]`, Expected: true},
		{Expr: `4 in [ 1, 2, 3 ]`, Expected: false},
		{Expr: `4 in [ 3, 2, 1 ]`, Expected: false},
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
	event := &model.Event{
		Open: model.OpenSyscall{
			Pathname: "/var/lib/httpd/htpasswd",
			Flags:    syscall.O_CREAT | syscall.O_TRUNC | syscall.O_EXCL | syscall.O_RDWR | syscall.O_WRONLY,
		},
	}

	tests := []struct {
		Expr     string
		Expected bool
	}{
		{Expr: `Open.Pathname =~ "/var/lib/httpd/*" && Open.Flags & (O_CREAT | O_TRUNC | O_EXCL | O_RDWR | O_WRONLY) > 0`, Expected: true},
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

func BenchmarkComplex(b *testing.B) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/ls",
			UID:  1,
		},
	}

	ctx := &Context{
		Event: event,
	}

	base := `(Process.Name == "/usr/bin/ls" && Process.UID != 0)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	rule, err := ast.ParseRule(expr)
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	eval, err := RuleToEvaluator(rule, false)
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	for i := 0; i < b.N; i++ {
		eval(ctx)
	}
}
