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
// A rule goes through four stages, and NewRule is all four:
//
//   - Parse, which translates the SECL text into a CEL AST — Translate prints it;
//   - Check, against the generated field types, which is what rejects a misspelt
//     field or a comparison against the wrong type;
//   - Optimize, which rewrites every field read into a read by index and is where
//     a field name is resolved for the last time — see Optimize and optimize.go;
//   - Plan, which is cel-go's, with the options plannerOptions asks for. It stops
//     at the interpretable rather than a cel.Program — see planRule.
//
// The first two are what a rule author reads, the last two are what runs.
//
// # Operator mapping
//
// Most of SECL maps onto standard CEL. The rest maps onto the math and network
// extension libraries, and only what CEL has no equivalent for at all becomes a
// helper function, declared by NewEnv. Field names appear here as they are
// written, without the root the next section adds:
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
//	                            secl.pathGlob(a, "gl") for a path field
//	a in [1, 2]                 a in [1, 2]
//	a in [~"/y/*"]              secl.glob(a, "/y/*")
//	a in ["x", ~"/y/*"]         secl.matchAny(a, secl.patterns(["x", secl.glob("/y/*")]))
//	a in b                      secl.matchAny(a, b)
//	                            secl.matchAnyPath(a, b) for a path field
//	a not in b                  !(a in b)
//	a allin b                   a in b
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
// # Field rooting
//
// The whole field namespace hangs under one CEL variable, EventRoot, so a field
// is translated with `evt.` in front of it:
//
//	SECL                        CEL
//	---------------------------------------------------------------
//	process.file.path           evt.process.file.path
//	event.timestamp             evt.event.timestamp
//	${foo}                      vars.foo
//	MY_MACRO, S_IFREG           MY_MACRO, S_IFREG
//
// Only a field is rooted, which is the other thing FieldTypes is consulted for: a
// name whose first segment is not a top level segment of the namespace is a macro
// or a constant, and is left for the environment to resolve. Without field types
// nothing is known to be a field, so nothing is rooted and the untyped forms of
// Translate and Compile need an environment that declares the top level names
// itself.
//
// Rooting is what lets the top level segments be members of one object type rather
// than declarations of their own, and so what lets a whole chain of selects be
// resolved to the one field it reads — see EventRoot.
//
// # Reading a field
//
// None of those selects survive into what runs. Once the expression is checked, the
// optimization pass rewrites each chain into a call that reads the field by its
// place in the generated layout:
//
//	checked                                     planned
//	---------------------------------------------------------------
//	evt.process.comm                            secl.readString(evt, 770)
//	evt.process.argv                            secl.readStrings(evt, 758)
//	evt.network.destination.ip                  secl.readCIDR(evt, 556)
//	evt.process.ancestors                       secl.readProcessAncestors(evt, 1)
//	evt.process.ancestors.exists(A, A.uid == 0) secl.readProcessAncestors(evt, 1)
//	                                              .exists(A, secl.readInt(A, 740) == 0)
//
// A read takes the position it reads from and the index of the field it wants, and
// nothing else: the position is the event root or one element of an iterated field,
// and the index is where the field sits in celReaders. So a name is resolved once
// per rule and never while an event is being matched, and reading a field is a
// slice index and one call — see Optimize and read.go.
//
// The functions differ only in what they are declared to return, which is what
// keeps the rewrite type-preserving; cel-go re-checks the result, so a rewrite that
// changed a type would fail at load.
//
// # Array semantics
//
// SECL reads a comparison against an array field as "some element matches", and
// whether a field is an array is a property of the model rather than of the
// syntax. Given a FieldTypes — ModelFieldTypes answers from the CEL types
// generated into celtypes_unix.go and celtypes_windows.go — the translation
// writes that quantifier out:
//
//	SECL                                  CEL
//	---------------------------------------------------------------
//	process.ancestors.file.name == "x"    evt.process.ancestors.exists(e, e.file.name == "x")
//	process.ancestors.uid in [0, 1]       evt.process.ancestors.exists(e, e.uid in [0, 1])
//	process.ancestors.file.name not in b  !evt.process.ancestors.exists(e, e.file.name in b)
//	process.argv == "-l"                  evt.process.argv.exists(e, e == "-l")
//	process.ancestors.args_flags == "c"   evt.process.ancestors.exists(e, e.args_flags.exists(e2, e2 == "c"))
//	process.argv.length > 2               size(evt.process.argv) > 2
//	process.ancestors.file.name.length    evt.process.ancestors.exists(e, size(e.file.name) …)
//
// Without field types those comparisons are translated literally, which is valid
// CEL but means "the list equals the scalar". NewModelEnv pairs the same type
// tree with the environment, so CompileWithTypes type-checks against the real
// field types instead of treating every event as dynamic.
//
// # Pattern semantics
//
// The field types settle a second thing SECL leaves implicit: what a `~"…"` literal
// means. SECL compiles one into a *glob* when the field it is compared against carries
// an operator override that asks for it — `*` stops at a path separator, `**` crosses
// one — and into a *pattern* otherwise, where `*` crosses everything and `**` is
// refused. Which fields those are is generated into celGlobFields, from the overrides
// the model declares, and checked against SECL itself by TestGlobFieldsAgreeWithSECL.
//
// So `open.file.path =~ "/etc/*"` is a glob call and `process.comm =~ "sh*"` a pattern
// call, and it is the field rather than the literal that decides. A list carries both:
// a member compiles both forms, and the membership call — MatchAnyFunc against
// MatchAnyPathFunc — says which the set is searched with. That is what lets one macro
// be a single prepared value while still meaning what SECL means at each of its use
// sites, where SECL resolves the difference by inlining the macro and rewriting the
// value type per comparison.
//
// # Macros and variables
//
// What a policy declares is declared in the environment rather than handled by the front
// end — NewPolicyEnv does both. A macro is a value, so it is translated, evaluated once at
// load and declared as a constant: the rule that names it then reads a value out of its
// own expression, at the price the same list written into it would cost. A `${…}` variable
// is declared under its dotted name with the type SECL compiled it as, and read through
// SECL's own closure, so the scoping, inheritance and TTL are production's.
//
// Nothing is declared under `vars` itself, which means a rule naming a variable no policy
// maintains does not compile — as it does not in SECL.
//
// Writes stay with SECL: a `set:` action mutates a variable while the rule engine runs its
// actions, so a shadow evaluation reads a store only the other engine fills.
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
//     constants or variables, while a bare name tries all of them. Both spellings
//     are translated the same way, so a `%{…}` naming a macro reaches the macro
//     instead of being rejected.
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
//
//   - `allin` is translated as a second spelling of `in`, which is what SECL makes of
//     it: outside a CIDR array its compiler special cases `notin` and lets `allin`
//     fall through to the evaluator `in` uses, so `process.argv allin [ "-l" ]` holds
//     of a process whose arguments merely contain `-l`. Reproducing that rather than
//     the meaning the operator's name carries is deliberate — what the shadow measures
//     itself against is the engine in production — and TestAllInIsMembershipLikeSECL
//     is what reports it if SECL ever makes the operator universal.
//
//   - A path comparison reaches one value, where SECL reaches several. The operator
//     overrides those fields carry OR in a comparison against the overlayfs-relative
//     path, and for `exec.file.path` and `process.file.path` two more against the
//     symlink pathnames (oo_overlayfs_unix.go, oo_symlink_unix.go). So SECL matches a
//     file reached through a symlink or an overlay mount where the translation does
//     not, which makes CEL the *narrower* engine on those fields — and is most of why
//     a path rule costs SECL four times what it costs here (BenchmarkPatternMembership).
//
//   - The case insensitive and separator normalising comparisons SECL uses for a few
//     unix fields and for most windows ones are not reproduced: `secl.glob` and
//     `secl.pathGlob` always match case sensitively, and a windows rule written with
//     the other separator does not match.
//
//   - A comparison against a variable is type-checked, where SECL compares loosely:
//     `${some_string} == 1` is a compile error here and answers false there. This is the
//     stricter direction, so no rule that fires in production is lost — see
//     TestVariablesAreTypeChecked.
//
//   - `builtins.uuid4` yields a fresh value per read (model/variables.go), so a rule
//     reading it twice disagrees with itself — in either engine, and between them. A
//     shadow's disagreement counter should not be read as a translation bug when it fires
//     on one of those.
//
//   - A list mixing constant types, which SECL rejects outright, is accepted here:
//     CEL widens a heterogeneous list literal to list(dyn) and then matches nothing.
//     A rule SECL refuses to compile is one that never fires — see
//     TestMixedConstantListIsAccepted.
//
//   - Partial evaluation is not wired. SECL answers "could this rule still match if
//     one field had this value and nothing else were known" with a variant of the
//     rule compiled per field, which approvers and discarders rest on. cel-go's own
//     mechanism for it withholds an *attribute*, and after the pass a field is a
//     call rather than an attribute, so it no longer applies — see
//     TestPartialEvaluationIsNotWired, which records both the gap and the shape that
//     would close it: run the pass again with one known field, emitting a read that
//     returns an unknown for every other one.
package seclcel

import (
	"github.com/google/cel-go/cel"
)

// Translate renders the CEL source text a SECL expression translates to. It is
// the readable form of Parse, useful for migrating rules and for debugging.
//
// With no field types nothing is known to be a field, so names are translated as
// they were written: use TranslateWithTypes for the form a rule is compiled from.
func Translate(expr string) (string, error) {
	return TranslateWithTypes(expr, nil)
}

// TranslateWithTypes is Translate against a set of field types, so that the
// fields are rooted at EventRoot and the array semantics SECL leaves implicit are
// written out.
func TranslateWithTypes(expr string, fieldTypes FieldTypes) (string, error) {
	a, err := ParseWithTypes(expr, fieldTypes)
	if err != nil {
		return "", err
	}
	return cel.ExprToString(a.Expr(), a.SourceInfo())
}

// Compile translates a SECL expression and type-checks it against env, which
// must declare the fields, macros and variables the expression uses on top of
// what NewEnv already provides. Without field types the fields are not rooted, so
// env has to declare the top level names themselves rather than come from
// NewModelEnv; CompileWithTypes is the pairing to use against the real model.
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
