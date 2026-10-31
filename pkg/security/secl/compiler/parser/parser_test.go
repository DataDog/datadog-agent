// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language.
package parser

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

func parseRule(rule string) (*ast.Rule, error) {
	pc := NewParsingContext(false)
	return pc.BuildRule(rule)
}

func parseMacro(macro string) (*ast.Macro, error) {
	pc := NewParsingContext(false)
	return pc.BuildMacro(macro)
}

func printTree(t *testing.T, node ast.Node) {
	t.Log(ast.Sprint(node))
}

func TestEmptyRule(t *testing.T) {
	_, err := parseRule(``)
	if err == nil {
		t.Error("Empty expression should not be valid")
	}
}

func TestCompareNumbers(t *testing.T) {
	rule, err := parseRule(`-3 > 1`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestCompareSimpleIdent(t *testing.T) {
	rule, err := parseRule(`process > 1`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestCompareCompositeIdent(t *testing.T) {
	rule, err := parseRule(`process.pid > 1`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestCompareString(t *testing.T) {
	rule, err := parseRule(`process.name == "/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestEscapedQuoteInString(t *testing.T) {
	rule, err := parseRule(`process.name == "a\"b"`)
	require.NoError(t, err)

	sc, ok := rule.Root.(*ast.ScalarCompareExpr)
	require.True(t, ok)
	str, ok := sc.Right.(*ast.StringLiteral)
	require.True(t, ok)
	require.Equal(t, `a\"b`, str.Value)
}

func TestEscapedNewlineInString(t *testing.T) {
	rule, err := parseRule(`process.name == "a\
b"`)
	require.NoError(t, err)

	sc, ok := rule.Root.(*ast.ScalarCompareExpr)
	require.True(t, ok)
	str, ok := sc.Right.(*ast.StringLiteral)
	require.True(t, ok)
	require.Equal(t, `a\
b`, str.Value)
}

func TestCompareComplex(t *testing.T) {
	rule, err := parseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestRegister(t *testing.T) {
	rule, err := parseRule(`process.ancestors[A].filename == "/usr/bin/vipw" && process.ancestors[A].pid == 44`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIntAnd(t *testing.T) {
	rule, err := parseRule(`3 & 3`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestBitwiseXor(t *testing.T) {
	rule, err := parseRule(`open.flags ^ 1 == 0`)
	require.NoError(t, err)

	sc, ok := rule.Root.(*ast.ScalarCompareExpr)
	require.True(t, ok)
	bitOp, ok := sc.Left.(*ast.BitExpr)
	require.True(t, ok)
	require.Equal(t, ast.BitXor, bitOp.Op)
}

func TestBoolAnd(t *testing.T) {
	rule, err := parseRule(`true and true`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestInArrayString(t *testing.T) {
	rule, err := parseRule(`"a" in [ "a", "b", "c" ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestInArrayInteger(t *testing.T) {
	rule, err := parseRule(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestMacroList(t *testing.T) {
	macro, err := parseMacro(`[ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, macro)
}

func TestMacroPrimary(t *testing.T) {
	macro, err := parseMacro(`true`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, macro)
}

func TestMacroExpression(t *testing.T) {
	macro, err := parseMacro(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, macro)
}

func TestMultiline(t *testing.T) {
	expr := `process.filename == "/usr/bin/vipw" &&
	process.pid == 44`

	if _, err := parseRule(expr); err != nil {
		t.Error(err)
	}

	expr = `process.filename in ["/usr/bin/vipw",
	"/usr/bin/test"]`

	if _, err := parseRule(expr); err != nil {
		t.Error(err)
	}

	expr = `process.filename == "/usr/bin/vipw" && (
	process.filename == "/usr/bin/test" || # blah blah
	# blah blah
	process.filename == "/ust/bin/false"
	)`

	if _, err := parseRule(expr); err != nil {
		t.Error(err)
	}
}

func TestCommentNoTrailingNewline(t *testing.T) {
	rule, err := parseRule("process.name == \"a\" # trailing comment, no newline")
	require.NoError(t, err)
	require.NotNil(t, rule)
}

func TestPattern(t *testing.T) {
	rule, err := parseRule(`process.name == ~"/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestArrayPattern(t *testing.T) {
	rule, err := parseRule(`process.name in [~"/usr/bin/ls", "/usr/sbin/ls"]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestRegexp(t *testing.T) {
	rule, err := parseRule(`process.name == r"/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)

	rule, err = parseRule(`process.name == r"^((?:[A-Za-z\d+]{4})*(?:[A-Za-z\d+]{3}=|[A-Za-z\d+]{2}==)\.)*(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z1-9])$" `)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestArrayRegexp(t *testing.T) {
	rule, err := parseRule(`process.name in [r"/usr/bin/ls", "/usr/sbin/ls"]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestPatternMismatchOperator(t *testing.T) {
	rule, err := parseRule(`process.name !~ r"a"`)
	require.NoError(t, err)

	sc, ok := rule.Root.(*ast.ScalarCompareExpr)
	require.True(t, ok)
	require.Equal(t, ast.ScalarNotMatch, sc.Op)
}

func TestDuration(t *testing.T) {
	rule, err := parseRule(`process.start > 10s`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestDurationHours(t *testing.T) {
	rule, err := parseRule(`process.start > 2h`)
	require.NoError(t, err)

	sc, ok := rule.Root.(*ast.ScalarCompareExpr)
	require.True(t, ok)
	duration, ok := sc.Right.(*ast.DurationLiteral)
	require.True(t, ok)
	require.Equal(t, int((2 * time.Hour).Nanoseconds()), duration.Value)
}

func TestNumberVariable(t *testing.T) {
	rule, err := parseRule(`process.pid == ${pid}`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv4(t *testing.T) {
	rule, err := parseRule(`network.source.ip == 127.0.0.1`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv4Raw(t *testing.T) {
	rule, err := parseRule(`127.0.0.2 == 127.0.0.1`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv6Localhost(t *testing.T) {
	rule, err := parseRule(`network.source.ip == ::1`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv6(t *testing.T) {
	rule, err := parseRule(`network.source.ip == 2001:0000:0eab:DEAD:0000:00A0:ABCD:004E`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv6Short(t *testing.T) {
	rule, err := parseRule(`network.source.ip == 2001:0:0eab:dead::a0:abcd:4e`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPArray(t *testing.T) {
	rule, err := parseRule(`network.source.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 127.0.0.1 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv4CIDR(t *testing.T) {
	rule, err := parseRule(`network.source.ip in 192.168.0.0/24`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPv6CIDR(t *testing.T) {
	rule, err := parseRule(`network.source.ip in ::1/128`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestCIDRArray(t *testing.T) {
	rule, err := parseRule(`network.source.ip in [ ::1/128, 2001:0:0eab:dead::a0:abcd:4e/24, 127.0.0.1/32 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestIPAndCIDRArray(t *testing.T) {
	rule, err := parseRule(`network.source.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 127.0.0.1, ::1/128, 2001:0:0eab:dead::a0:abcd:4e/24, 127.0.0.1/32 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestCIDRMatches(t *testing.T) {
	rule, err := parseRule(`network.source.cidr allin 192.168.0.0/24`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestCIDRArrayMatches(t *testing.T) {
	rule, err := parseRule(`network.source.cidr allin [ 192.168.0.0/24, ::1/128 ]`)
	if err != nil {
		t.Error(err)
	}

	printTree(t, rule)
}

func TestPrecedence(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{
			// `&&` binds tighter than `||`
			expr:     `process.uid == 1 && process.gid == 2 || process.pid == 3`,
			expected: `(|| (&& (== process.uid 1) (== process.gid 2)) (== process.pid 3))`,
		},
		{
			expr:     `process.uid == 1 || process.gid == 2 && process.pid == 3`,
			expected: `(|| (== process.uid 1) (&& (== process.gid 2) (== process.pid 3)))`,
		},
		{
			expr:     `(process.uid == 1 || process.gid == 2) && process.pid == 3`,
			expected: `(&& (|| (== process.uid 1) (== process.gid 2)) (== process.pid 3))`,
		},
		{
			// `&` binds tighter than `|`
			expr:     `open.flags | O_CREAT & O_TRUNC > 0`,
			expected: `(> (| open.flags (& O_CREAT O_TRUNC)) 0)`,
		},
		{
			// a unary operator binds tighter than any binary one
			expr:     `^open.flags == 0`,
			expected: `(== (^ open.flags) 0)`,
		},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			rule, err := parseRule(test.expr)
			require.NoError(t, err)
			require.Equal(t, test.expected, ast.Sprint(rule))
		})
	}
}

func TestSyntaxErrors(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{
			expr: `open.file.path ==`,
			expected: "1:18: expected operand after `==`, found end of expression\n" +
				"open.file.path ==\n" +
				"                 ^",
		},
		{
			expr: `open.file.path = "/etc/passwd"`,
			expected: "1:16: unexpected `=`, did you mean `==`?\n" +
				"open.file.path = \"/etc/passwd\"\n" +
				"               ^",
		},
		{
			// the error points to the opening quote
			expr: `open.file.path == "/etc/passwd && process.uid == 0`,
			expected: "1:19: unterminated string\n" +
				"open.file.path == \"/etc/passwd && process.uid == 0\n" +
				"                  ^",
		},
		{
			// columns count characters, not bytes: `é` is two bytes
			expr: `open.file.name == "é" process.uid == 0`,
			expected: "1:23: expected `&&`, `||` or end of expression, found identifier `process.uid`, missing `&&` or `||`?\n" +
				"open.file.name == \"é\" process.uid == 0\n" +
				"                      ^^^^^^^^^^^",
		},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			_, err := parseRule(test.expr)

			var errs ErrorList
			require.ErrorAs(t, err, &errs)
			require.Equal(t, test.expected, errs.Format(test.expr))
		})
	}
}
