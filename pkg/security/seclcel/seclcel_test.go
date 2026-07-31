// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// translations pairs a SECL expression with the CEL source it translates to.
// Every entry is also type-checked and round-tripped through the CEL parser by
// the tests below.
var translations = []struct {
	secl string
	cel  string
}{
	// comparisons
	{`process.file.name == "sh"`, `process.file.name == "sh"`},
	{`process.file.name != "sh"`, `process.file.name != "sh"`},
	{`process.uid >= 1000`, `process.uid >= 1000`},
	{`process.is_kworker == true`, `process.is_kworker == true`},
	{`true`, `true`},

	// logical operators. SECL gives && and || the same precedence and makes
	// them right associative, so the grouping of a && b || c differs from CEL's
	// and has to be parenthesised.
	{`process.uid == 0 && process.file.name == "sh"`, `process.uid == 0 && process.file.name == "sh"`},
	{`process.uid == 0 or process.file.name == "sh"`, `process.uid == 0 || process.file.name == "sh"`},
	{`a && b || c`, `a && (b || c)`},
	{`a || b && c`, `a || b && c`},
	{`a && b && c`, `a && b && c`},
	{`not a`, `!a`},
	{`!a`, `!a`},
	{`a == b == c`, `a == (b == c)`},
	{`(process.uid == 0 || process.gid == 0) && process.file.name == "sh"`,
		`(process.uid == 0 || process.gid == 0) && process.file.name == "sh"`},
	{`process.file.name == "sh" # trailing comment`, `process.file.name == "sh"`},

	// glob patterns have no CEL equivalent; regexps are RE2 in both languages
	{`process.file.path =~ "/usr/*"`, `secl.glob(process.file.path, "/usr/*")`},
	{`process.file.path == ~"/usr/*"`, `secl.glob(process.file.path, "/usr/*")`},
	{`process.file.path !~ ~"/usr/*"`, `!(secl.glob(process.file.path, "/usr/*"))`},
	{`process.file.name == r"^sh.*"`, `process.file.name.matches("^sh.*")`},
	{`process.file.name =~ r"^sh.*"`, `process.file.name.matches("^sh.*")`},

	// membership
	{`process.file.name in [ "sh", "bash" ]`, `process.file.name in ["sh", "bash"]`},
	{`process.file.name not in [ "sh", "bash" ]`, `!(process.file.name in ["sh", "bash"])`},
	{`process.uid in [ 0, 1 ]`, `process.uid in [0, 1]`},
	{`process.file.name in MY_MACRO`, `process.file.name in MY_MACRO`},
	{`process.file.name in ${my.var}`, `process.file.name in vars.my.var`},
	// a list holding a matcher cannot use CEL's equality based `in`
	{`process.file.name in [ "sh", ~"/usr/*", r"z.*" ]`,
		`process.file.name == "sh" || secl.glob(process.file.name, "/usr/*") || process.file.name.matches("z.*")`},
	{`process.file.name not in [ "sh", ~"/usr/*" ]`,
		`!(process.file.name == "sh" || secl.glob(process.file.name, "/usr/*"))`},
	{`process.args allin [ "a", "b" ]`, `process.args.all(elem, elem in ["a", "b"])`},
	{`process.args allin [ ~"/a/*" ]`, `process.args.all(elem, secl.glob(elem, "/a/*"))`},

	// arithmetic and bitwise operators
	{`process.uid + 1 > 2`, `process.uid + 1 > 2`},
	{`1 + 2 - 3 > 0`, `1 + 2 - 3 > 0`},
	{`- process.uid > 0`, `-process.uid > 0`},
	{`open.flags & 512 > 0`, `math.bitAnd(open.flags, 512) > 0`},
	{`open.flags | 512 > 0`, `math.bitOr(open.flags, 512) > 0`},
	{`open.flags ^ 512 > 0`, `math.bitXor(open.flags, 512) > 0`},
	{`^open.flags > 0`, `math.bitNot(open.flags) > 0`},
	// SECL makes the bitwise operators right associative
	{`open.flags & 1 | 2 > 0`, `math.bitAnd(open.flags, math.bitOr(1, 2)) > 0`},

	// IP and CIDR
	{`network.destination.ip == 1.2.3.4`, `secl.cidrMatch(network.destination.ip, ip("1.2.3.4"))`},
	{`network.destination.ip == 10.0.0.0/8`, `secl.cidrMatch(network.destination.ip, cidr("10.0.0.0/8"))`},
	{`network.destination.ip != 10.0.0.0/8`, `!(secl.cidrMatch(network.destination.ip, cidr("10.0.0.0/8")))`},
	{`network.destination.ip in 10.0.0.0/8`, `secl.cidrMatch(network.destination.ip, cidr("10.0.0.0/8"))`},
	{`network.destination.ip in [ 10.0.0.0/8, 192.168.0.1 ]`,
		`secl.cidrMatch(network.destination.ip, [cidr("10.0.0.0/8"), ip("192.168.0.1")])`},
	{`network.ips allin 10.0.0.0/8`, `secl.cidrMatchAll(network.ips, cidr("10.0.0.0/8"))`},

	// durations. SECL reads `field < 10m` as "less than 10 minutes ago", but
	// compares an arithmetic result against the duration directly.
	{`process.created_at < 10m`, `secl.nanos(secl_now - process.created_at) < duration("10m")`},
	{`process.created_at >= 1h`, `secl.nanos(secl_now - process.created_at) >= duration("1h")`},
	{`process.created_at - process.parent.created_at < 10m`,
		`secl.nanos(process.created_at - process.parent.created_at) < duration("10m")`},
	// a duration used anywhere else is just a nanosecond count
	{`process.created_at == 10m + 1`, `process.created_at == 600000000000 + 1`},
	// SECL only gives the ordering operators and == duration semantics, so this
	// one is a small generalisation
	{`process.created_at != 10m`, `!(secl.nanos(secl_now - process.created_at) == duration("10m"))`},

	// variables and field references
	{`${my.var} == 1`, `vars.my.var == 1`},
	{`${my.var.length} > 3`, `size(vars.my.var) > 3`},
	{`%{process.file.name} == "sh"`, `process.file.name == "sh"`},
	{`%{process.args.length} > 3`, `size(process.args) > 3`},
	{`%{dns.question.name.root_domain} == "example.com"`, `secl.rootDomain(dns.question.name) == "example.com"`},

	// string interpolation becomes a CEL concatenation
	{`"prefix-${my.var}-suffix" == process.file.name`,
		`"prefix-" + secl.str(vars.my.var) + "-suffix" == process.file.name`},
	{`"%{process.file.name}" == "sh"`, `secl.str(process.file.name) == "sh"`},

	// subscripts: a numeric index stays an index, an iterator variable becomes
	// an exists() around the whole expression
	{`process.ancestors[0].file.name == "sh"`, `process.ancestors[0].file.name == "sh"`},
	{`process.ancestors[A].file.name == "sh"`, `process.ancestors.exists(A, A.file.name == "sh")`},
	{`process.ancestors[A].file.name == "sh" && process.ancestors[A].uid == 0`,
		`process.ancestors.exists(A, A.file.name == "sh" && A.uid == 0)`},
	{`process.ancestors[A] == "sh"`, `process.ancestors.exists(A, A == "sh")`},
}

// testEnv declares the roots the test expressions refer to. Real field
// declarations come from the model; DynType is enough to exercise the
// translation.
func testEnv(t *testing.T) *cel.Env {
	t.Helper()

	env, err := NewEnv(
		cel.Variable("process", cel.DynType),
		cel.Variable("open", cel.DynType),
		cel.Variable("network", cel.DynType),
		cel.Variable("dns", cel.DynType),
		cel.Variable("a", cel.BoolType),
		cel.Variable("b", cel.BoolType),
		cel.Variable("c", cel.BoolType),
		cel.Variable("MY_MACRO", cel.ListType(cel.StringType)),
	)
	require.NoError(t, err)
	return env
}

func TestTranslate(t *testing.T) {
	for _, tt := range translations {
		t.Run(tt.secl, func(t *testing.T) {
			got, err := Translate(tt.secl)
			require.NoError(t, err)
			assert.Equal(t, tt.cel, got)
		})
	}
}

// TestTranslateRoundTrip checks that the CEL source a SECL expression
// translates to parses back to the same expression. Since the translator builds
// the AST directly, this is what proves that SECL's grouping survives being
// written out as CEL text, where the operator precedence is not the same.
func TestTranslateRoundTrip(t *testing.T) {
	env := testEnv(t)

	for _, tt := range translations {
		t.Run(tt.secl, func(t *testing.T) {
			translated, err := Translate(tt.secl)
			require.NoError(t, err)

			parsed, iss := env.Parse(translated)
			require.NoError(t, iss.Err())

			reparsed, err := cel.AstToString(parsed)
			require.NoError(t, err)
			assert.Equal(t, translated, reparsed)
		})
	}
}

func TestCompile(t *testing.T) {
	env := testEnv(t)

	for _, tt := range translations {
		t.Run(tt.secl, func(t *testing.T) {
			checked, err := Compile(env, tt.secl)
			require.NoError(t, err)
			assert.True(t, checked.IsChecked())
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{``, "unexpected <EOF>"},
		{`process.file.name ==`, "unexpected <EOF>"},
		{`(process.uid == 0`, "expected `)`"},
		{`process.uid in [1, 2`, "expected `]`"},
		{`process.file.name == "sh`, "unterminated string"},

		// the SECL lexer folds a signed integer into a single token, so the
		// binary minus needs to be separated from its operand
		{`process.uid-1 > 0`, `unexpected integer "-1"`},
		// `notin` is spelled `not in`
		{`process.uid notin [1]`, `unexpected identifier "notin"`},

		// a match needs a static pattern on the right
		{`process.file.name =~ process.parent.file.name`,
			"the right hand side of `=~` must be a string, a pattern or a regexp"},
		{`~"/a/*" == ~"/b/*"`, "cannot compare two patterns"},

		// the right hand side of a membership test has to be a list or a name
		{`process.file.name in "sh"`, "expected a list, a name or a CIDR"},
		{`process.created_at in 10m`, "expected a list, a name or a CIDR"},

		// iterator variables
		{`process.ancestors[A].uid == 0 && container.tags[B] == "x"`,
			"only one iterator variable is supported"},
		{`process.ancestors[A].uid == 0 && container.tags[A] == "x"`,
			"is used with two different fields"},
		{`process.ancestors[_].uid == 0`, "`_` cannot be used as an iterator variable name"},
		{`process.ancestors[A][B].uid == 0`, "more than one subscript"},

		// malformed literals
		{`network.destination.ip == 999.999.999.999`, `invalid IP "999.999.999.999"`},
		{`network.destination.ip == 10.0.0.0/99`, `invalid CIDR "10.0.0.0/99"`},
		{`process.file.name. == "sh"`, `malformed name "process.file.name."`},
		{`%{process.ancestors[0]} == "sh"`, "subscripts are not supported in the field reference"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := Parse(tt.expr)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseErrorPosition(t *testing.T) {
	_, err := Parse("process.uid == 0 &&\nprocess.uid-1 > 0")
	require.Error(t, err)

	var perr *ParseError
	require.ErrorAs(t, err, &perr)
	assert.Equal(t, "2:12: unexpected integer \"-1\"", perr.Error())
}

func TestLex(t *testing.T) {
	tests := []struct {
		expr  string
		kinds []tokenKind
		vals  []string
	}{
		// an IP is not four integers separated by dots, and a CIDR is not an IP
		// followed by a division
		{`10.0.0.1`, []tokenKind{tokIP}, []string{"10.0.0.1"}},
		{`10.0.0.0/8`, []tokenKind{tokCIDR}, []string{"10.0.0.0/8"}},
		{`::1/128`, []tokenKind{tokCIDR}, []string{"::1/128"}},
		{`2001:db8::1`, []tokenKind{tokIP}, []string{"2001:db8::1"}},

		// a duration is not an integer followed by an identifier
		{`10m`, []tokenKind{tokDuration}, []string{"10m"}},
		{`10ms`, []tokenKind{tokDuration}, []string{"10ms"}},
		{`10`, []tokenKind{tokInt}, []string{"10"}},

		// a regexp is not the identifier r followed by a string
		{`r"a.*"`, []tokenKind{tokRegexp}, []string{"a.*"}},
		{`~"a*"`, []tokenKind{tokPattern}, []string{"a*"}},
		{`"a*"`, []tokenKind{tokString}, []string{"a*"}},

		// a field name carries its dots and subscripts
		{`process.ancestors[A].file.name`, []tokenKind{tokIdent}, []string{"process.ancestors[A].file.name"}},

		{`${a.b}`, []tokenKind{tokVariable}, []string{"a.b"}},
		{`%{a.b}`, []tokenKind{tokFieldRef}, []string{"a.b"}},

		// escapes are left in place, as the SECL lexer leaves them
		{`"a\"b"`, []tokenKind{tokString}, []string{`a\"b`}},

		// operators are lexed one character at a time
		{`a >= 1`, []tokenKind{tokIdent, tokPunct, tokPunct, tokInt}, []string{"a", ">", "=", "1"}},

		// comments are dropped
		{"// leading\na # trailing", []tokenKind{tokIdent}, []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			toks, err := lex(tt.expr)
			require.Nil(t, err)

			require.Len(t, toks, len(tt.kinds)+1, "expected a trailing EOF")
			assert.Equal(t, tokEOF, toks[len(toks)-1].kind)

			for i, kind := range tt.kinds {
				assert.Equal(t, kind, toks[i].kind, "kind of token %d", i)
				assert.Equal(t, tt.vals[i], toks[i].val, "value of token %d", i)
			}
		})
	}
}
