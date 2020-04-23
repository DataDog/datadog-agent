package eval

import (
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

func parse(t *testing.T, expr string) (*RuleEvaluator, *ast.Rule, error) {
	rule, err := ast.ParseRule(expr)
	if err != nil {
		t.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	evaluator, err := RuleToEvaluator(rule, false)
	if err != nil {
		return nil, rule, err
	}

	return evaluator, rule, err
}

func eval(t *testing.T, event *model.Event, expr string) (bool, *ast.Rule, error) {
	evaluator, rule, err := parse(t, expr)
	if err != nil {
		return false, rule, err
	}

	ctx := &Context{Event: event}

	return evaluator.Eval(ctx), rule, nil
}

func TestStringError(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
			UID:  1,
		},
		Open: model.OpenSyscall{
			Filename: "/etc/shadow",
		},
	}

	_, _, err := eval(t, event, `process.name != "/usr/bin/vipw" && process.uid != 0 && open.filename == 3`)
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
			Filename: "/etc/shadow",
		},
	}

	_, _, err := eval(t, event, `process.name != "/usr/bin/vipw" && process.uid != "test" && Open.Filename == "/etc/shadow"`)
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
			Filename: "/etc/shadow",
		},
	}

	_, _, err := eval(t, event, `(process.name != "/usr/bin/vipw") == "test"`)
	if err == nil || err.(*AstToEvalError).Pos.Column != 38 {
		t.Fatal("should report a bool type error")
	}
}

func TestSimpleString(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "/usr/bin/cat",
			UID:  1,
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
		{Expr: `process.name =~ "/usr/bin/*"`, Expected: true},
		{Expr: `process.name =~ "/usr/sbin/*"`, Expected: false},
		{Expr: `process.name !~ "/usr/sbin/*"`, Expected: true},
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
	event := &model.Event{
		Process: model.Process{
			Name: "a",
			UID:  3,
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
	event := &model.Event{
		Open: model.OpenSyscall{
			Filename: "/var/lib/httpd/htpasswd",
			Flags:    syscall.O_CREAT | syscall.O_TRUNC | syscall.O_EXCL | syscall.O_RDWR | syscall.O_WRONLY,
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

func TestTags(t *testing.T) {
	expr := `process.name != "/usr/bin/vipw" && open.filename == "/etc/passwd"`
	evaluator, _, err := parse(t, expr)
	if err != nil {
		t.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	expected := []string{"fs", "process"}

	if !reflect.DeepEqual(evaluator.Tags, expected) {
		t.Errorf("tags expected not %+v != %+v", expected, evaluator.Tags)
	}
}

func TestPartial(t *testing.T) {
	event := &model.Event{
		Process: model.Process{
			Name: "abc",
		},
		Open: model.OpenSyscall{
			Filename: "xyz",
		},
	}

	tests := []struct {
		Expr          string
		Field         string
		IsDiscrimator bool
	}{
		{Expr: `true || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `false || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: true},
		{Expr: `true || process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `false || process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `true && process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: true},
		{Expr: `false && process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: true},
		{Expr: `true && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `false && process.name == "abc"`, Field: "process.name", IsDiscrimator: true},
		{Expr: `open.filename == "test1" && process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: true},
		{Expr: `open.filename == "test1" && process.name != "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename == "test1" || process.name == "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename == "test1" || process.name != "/usr/bin/cat"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename == "test1" && !(process.name == "/usr/bin/cat")`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename == "test1" && !(process.name != "/usr/bin/cat")`, Field: "process.name", IsDiscrimator: true},
		{Expr: `open.filename == "test1" && (process.name =~ "/usr/bin/*" )`, Field: "process.name", IsDiscrimator: true},
		{Expr: `open.filename == "test1" && process.name =~ "ab*" `, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename == "test1" && process.name == open.filename`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename =~ "test1" && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename in [ "test1", "test2" ] && (process.name == open.filename)`, Field: "process.name", IsDiscrimator: false},
		{Expr: `open.filename in [ "test1", "test2" ] && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `!(open.filename in [ "test1", "test2" ]) && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ]) && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && true) && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && false) && process.name == "abc"`, Field: "process.name", IsDiscrimator: false},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && false) && !(process.name == "abc")`, Field: "process.name", IsDiscrimator: true},
		{Expr: `!(open.filename in [ "test1", "xyz" ] && false) && !(process.name == "abc")`, Field: "open.filename", IsDiscrimator: false},
		{Expr: `(open.filename not in [ "test1", "xyz" ] && true) && !(process.name == "abc")`, Field: "open.filename", IsDiscrimator: true},
		{Expr: `open.filename == open.filename`, Field: "open.filename", IsDiscrimator: false},
		{Expr: `open.filename != open.filename`, Field: "open.filename", IsDiscrimator: true},
	}

	for _, test := range tests {
		evaluator, _, err := parse(t, test.Expr)
		if err != nil {
			t.Fatalf("error while evaluating `%s`: %s", test.Expr, err)
		}

		result, err := evaluator.IsDiscrimator(&Context{Event: event}, test.Field)
		if err != nil {
			t.Fatalf("error while partial evaluating `%s` for `%s`: %s", test.Expr, test.Field, err)
		}

		if result != test.IsDiscrimator {
			t.Fatalf("expected result `%t` for `%s`not found, got `%t`\n%s", test.IsDiscrimator, test.Field, result, test.Expr)
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

	base := `(process.name == "/usr/bin/ls" && process.uid == 1)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	rule, err := ast.ParseRule(expr)
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	evaluator, err := RuleToEvaluator(rule, false)
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	for i := 0; i < b.N; i++ {
		if evaluator.Eval(ctx) != true {
			b.Fatal("unexpected result")
		}
	}
}

func BenchmarkPartial(b *testing.B) {
	event := &model.Event{
		Process: model.Process{
			Name: "abc",
			UID:  1,
		},
	}

	ctx := &Context{
		Event: event,
	}

	base := `(process.name == "/usr/bin/ls" && process.uid != 0)`
	var exprs []string

	for i := 0; i != 100; i++ {
		exprs = append(exprs, base)
	}

	expr := strings.Join(exprs, " && ")

	rule, err := ast.ParseRule(expr)
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	evaluator, err := RuleToEvaluator(rule, false)
	if err != nil {
		b.Fatal(fmt.Sprintf("%s\n%s", err, expr))
	}

	for i := 0; i < b.N; i++ {
		if ok, _ := evaluator.IsDiscrimator(ctx, "process.name"); ok {
			b.Fatal("unexpected result")
		}
	}
}
