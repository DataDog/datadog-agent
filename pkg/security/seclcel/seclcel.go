// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seclcel parses the SECL expression syntax directly into CEL.
//
// It is a front end, not a second evaluator: it reads SECL source text and
// builds a CEL AST, with its own lexer and recursive descent parser rather than
// the participle grammar in pkg/security/secl/compiler/ast. Rules keep their
// current syntax while the evaluation moves to CEL.
//
// # Operator mapping
//
// Most of SECL maps onto standard CEL. The rest maps onto the math and network
// extension libraries, and only what CEL has no equivalent for at all becomes a
// helper function, declared by NewEnv:
//
//	SECL                        CEL
//	---------------------------------------------------------------
//	a && b, a and b             a && b
//	a || b, a or b              a || b
//	!a, not a                   !a
//	a == b, a != b              a == b, a != b
//	a < b, a <= b, …            a < b, a <= b, …
//	a + b, a - b, -a            a + b, a - b, -a
//	a & b, a | b, a ^ b, ^a     math.bitAnd(a, b), … , math.bitNot(a)
//	a == r"re", a =~ r"re"      a.matches("re")
//	a == ~"gl", a =~ "gl"       secl.glob(a, "gl")
//	a in [1, 2]                 a in [1, 2]
//	a in ["x", ~"/y/*"]         a == "x" || secl.glob(a, "/y/*")
//	a not in b                  !(a in b)
//	a allin b                   a.all(elem, elem in b)
//	1.2.3.4, 10.0.0.0/8         ip("1.2.3.4"), cidr("10.0.0.0/8")
//	a == 10.0.0.0/8, a in …     secl.cidrMatch(a, cidr("10.0.0.0/8"))
//	a allin 10.0.0.0/8          secl.cidrMatchAll(a, cidr("10.0.0.0/8"))
//	a < 10m                     secl.nanos(secl_now - a) < duration("10m")
//	a - b < 10m                 secl.nanos(a - b) < duration("10m")
//	${foo}                      vars.foo
//	${foo.length}               size(vars.foo)
//	%{a.b}                      a.b
//	%{a.b.length}               size(a.b)
//	%{a.b.root_domain}          secl.rootDomain(a.b)
//	"x-${foo}"                  "x-" + secl.str(vars.foo)
//	a.b.c                       a.b.c
//	a.b[0].c                    a.b[0].c
//	a.b[X].c == "x"             a.b.exists(X, X.c == "x")
//
// # Array semantics
//
// SECL reads a comparison against an array field as "some element matches", and
// whether a field is an array is a property of the model rather than of the
// syntax. Given a FieldTypes — ModelFieldTypes answers from the CEL types
// generated into celtypes_unix.go and celtypes_windows.go — the translation
// writes that quantifier out:
//
//	SECL                                CEL
//	---------------------------------------------------------------
//	process.ancestors.file.name == "x"  process.ancestors.exists(e, e.file.name == "x")
//	process.ancestors.uid in [0, 1]     process.ancestors.exists(e, e.uid in [0, 1])
//	process.ancestors.file.name not in b  !process.ancestors.exists(e, e.file.name in b)
//	process.ancestors.file.name allin b   process.ancestors.all(e, e.file.name in b)
//	process.argv == "-l"                process.argv.exists(e, e == "-l")
//	process.ancestors.args_flags == "c" process.ancestors.exists(e, e.args_flags.exists(e2, e2 == "c"))
//	process.argv.length > 2             size(process.argv) > 2
//	process.ancestors.file.name.length  process.ancestors.exists(e, size(e.file.name) …)
//
// Without field types those comparisons are translated literally, which is valid
// CEL but means "the list equals the scalar". NewModelEnv pairs the same type
// tree with the environment, so CompileWithTypes type-checks against the real
// field types instead of treating every event as dynamic.
//
// # Divergences
//
// Some SECL behaviour cannot be reproduced by a translation that does not know
// the field types, and a few cases are deliberately not reproduced:
//
//   - SECL gives `&&` and `||` the same precedence and makes them, and the
//     comparison operators, right associative. The CEL AST preserves SECL's
//     grouping, so `a && b || c` becomes `a && (b || c)`; the two languages
//     disagree only about the *source text*, which is why Translate emits
//     explicit parentheses.
//
//   - Both sides of a comparison being array fields is translated literally
//     rather than quantified on both, since which one to iterate is ambiguous;
//     the checker reports it.
//
//   - `%{field}` resolves against event fields only, never against macros,
//     constants or variables, while a bare name tries all of them. CEL has one
//     namespace, so both spellings produce the same qualified name.
//
//   - A `[0]` subscript in the middle of a field name is read as an array index.
//     SECL reads a trailing `[0]` that way but treats the same subscript
//     mid-name as an iterator variable called "0".
//
//   - The network extension parses IP addresses strictly, so IPv4 mapped IPv6
//     literals such as `::ffff:192.168.0.1/120`, which SECL accepts, are
//     rejected when the translated expression is checked.
//
//   - A few forms are accepted that SECL rejects: a list of names
//     (`a in [FOO, BAR]`), a bare IP as the right hand side of `in`, and `!=`
//     against a duration, which SECL only supports for the ordering operators
//     and `==`. No expression that SECL accepts is affected.
//
//   - Expressions that SECL parses but its compiler then rejects, such as one
//     binding two iterator variables, are rejected here at parse time instead.
package seclcel

import (
	"github.com/google/cel-go/cel"
)

// Translate renders the CEL source text a SECL expression translates to. It is
// the readable form of Parse, useful for migrating rules and for debugging.
func Translate(expr string) (string, error) {
	return TranslateWithTypes(expr, nil)
}

// TranslateWithTypes is Translate against a set of field types, so that the
// array semantics SECL leaves implicit are written out.
func TranslateWithTypes(expr string, fieldTypes FieldTypes) (string, error) {
	a, err := ParseWithTypes(expr, fieldTypes)
	if err != nil {
		return "", err
	}
	return cel.ExprToString(a.Expr(), a.SourceInfo())
}

// Compile translates a SECL expression and type-checks it against env, which
// must declare the fields, macros and variables the expression uses on top of
// what NewEnv already provides.
//
// The type-checking goes through the CEL source form, because cel.Env only
// checks an AST that its own parser produced. That is not a translation of a
// translation: TestCorpus asserts over a corpus of real rules that the printed
// CEL parses back to the expression Parse built. Callers that do not need a
// checked expression can hand the result of Parse straight to
// cel.Env.PlanProgram.
func Compile(env *cel.Env, expr string) (*cel.Ast, error) {
	return CompileWithTypes(env, expr, nil)
}

// CompileWithTypes is Compile against a set of field types. Pair it with an
// environment built by NewModelEnv, which declares the same fields, so that the
// translation and the check agree about what holds several values.
func CompileWithTypes(env *cel.Env, expr string, fieldTypes FieldTypes) (*cel.Ast, error) {
	translated, err := TranslateWithTypes(expr, fieldTypes)
	if err != nil {
		return nil, err
	}

	checked, iss := env.Compile(translated)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	return checked, nil
}
