// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// patternEvent is the event the differential runs against: a shell with a path, some
// arguments and an ancestor, which is enough to exercise a membership test on a scalar,
// on a list valued leaf and on an iterated field.
func patternEvent() *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.BaseEvent.ProcessContext.Process.Comm = "sh"
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/bash"
	event.BaseEvent.ProcessContext.Process.Credentials.UID = 1000
	event.BaseEvent.ProcessContext.Process.Argv = []string{"-l", "--color"}
	ancestry(event, []string{"zsh"}, []uint32{0})
	return event
}

// evalSECLEngine evaluates an expression through SECL's own compiler and evaluator,
// against the same model and the same constants the agent uses.
func evalSECLEngine(t *testing.T, event *model.Event, expr string) bool {
	t.Helper()

	var m model.Model
	rule, err := eval.NewRule("differential", expr, ast.NewParsingContext(false), &eval.Opts{
		Constants:    model.SECLConstants(),
		LegacyFields: model.SECLLegacyFields,
	})
	require.NoError(t, err, "SECL parsing %q", expr)
	require.NoError(t, rule.GenEvaluator(&m), "SECL compiling %q", expr)

	return rule.Eval(eval.NewContext(event))
}

// patternMembershipExpressions are the shapes a pattern value has to answer for. They
// are checked against SECL rather than against an expected verdict, because what the
// pattern types are for is agreeing with SECL: a member decides for itself whether it
// is compared or matched, and the list is where the two languages could quietly differ.
var patternMembershipExpressions = []string{
	// a scalar subject, with the members mixing all three kinds
	`process.comm in [ "sh", ~"/usr/*", r"^z" ]`,
	`process.comm in [ "zsh", ~"/usr/*", r"^z" ]`,
	`process.comm not in [ "sh", ~"/usr/*" ]`,
	`process.comm not in [ "zsh", ~"/x/*" ]`,

	// a pattern that matches and one that does not, on the field patterns are
	// actually written for
	`process.file.path in [ "/bin/sh", ~"/usr/bin/*" ]`,
	`process.file.path in [ "/bin/sh", ~"/etc/*" ]`,
	// a path field keeps to patterns both engines read the same way — the ones they
	// do not are TestPathGlobsDivergeFromSECL
	`process.file.path not in [ ~"/usr/bin/*", ~"/etc/*" ]`,
	`process.file.path not in [ ~"/sbin/*", ~"/etc/*" ]`,

	// a regexp member, which SECL anchors the way CEL does — not at all
	`process.file.name in [ "sh", r"^ba" ]`,
	`process.file.name in [ "sh", r"^ba$" ]`,

	// a list valued leaf: some element is a member, and every element is
	`process.argv in [ "-l", ~"--*" ]`,
	`process.argv in [ "-x", ~"--x*" ]`,
	`process.argv allin [ "-l", ~"--*" ]`,
	`process.argv allin [ ~"-*" ]`,
	`process.argv allin [ ~"--*" ]`,
	`process.argv not in [ ~"-x*" ]`,

	// an iterated field, where the quantifier is outside the membership
	`process.ancestors.file.name in [ "zsh", ~"/usr/*" ]`,
	`process.ancestors.comm in [ "sh", ~"z*" ]`,
	`process.ancestors.comm not in [ ~"z*" ]`,

	// a single member, which takes the comparison rather than a set of one
	`process.comm in [ ~"s*" ]`,
	`process.comm in [ ~"x*" ]`,
	`process.comm in [ r"^s" ]`,

	// a homogeneous list, which stays on CEL's own membership
	`process.comm in [ "sh", "bash" ]`,
	`process.uid in [ 0, 1000 ]`,
	`process.uid not in [ 0 ]`,
}

// TestPatternMembershipAgreesWithSECL is the differential the pattern values exist to
// pass: for every shape a list can take, both engines return the same verdict.
func TestPatternMembershipAgreesWithSECL(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := patternEvent()

	for _, expr := range patternMembershipExpressions {
		t.Run(expr, func(t *testing.T) {
			assert.Equal(t, evalSECLEngine(t, event, expr), evalSECL(t, env, event, expr),
				"the two engines disagree")
		})
	}
}

// TestMacroConstantAgreesWithTheListWrittenOut is the equivalence a macro relies on: a
// list evaluated once at load and declared as a constant answers what the same list
// written into the rule does, patterns included.
//
// That is what makes a macro a constant rather than a front end concern — see the
// package doc — and it is checked against SECL, which is what inlines the macro on its
// own side.
func TestMacroConstantAgreesWithTheListWrittenOut(t *testing.T) {
	// The members are absolute and hold no regexp, because the same macro is used
	// against a path field below and SECL refuses both a relative path and a regexp
	// there. The regexp members are covered by the differential above.
	const members = `[ "/bin/dash", ~"/usr/bin/*" ]`

	env, err := NewModelEnv(cel.Constant("SHELL_NAMES", PatternsType,
		patternsValue(t, types.String("/bin/dash"), globValueOf(t, "/usr/bin/*"))))
	require.NoError(t, err)

	event := patternEvent()

	for _, field := range []string{"process.comm", "process.file.path", "process.argv", "process.ancestors.comm"} {
		for _, op := range []string{"in", "not in", "allin"} {
			expr := fmt.Sprintf("%s %s SHELL_NAMES", field, op)
			t.Run(expr, func(t *testing.T) {
				written := fmt.Sprintf("%s %s %s", field, op, members)
				assert.Equal(t, evalSECLEngine(t, event, written), evalSECL(t, env, event, expr),
					"the macro and the list written out disagree")
			})
		}
	}
}

// TestPatternsAreTypeChecked is why a prepared set is its own type rather than a
// list(dyn): a subject that could never be a member is a compile error, where a dynamic
// list would have accepted the rule and answered false for the lifetime of the agent.
func TestPatternsAreTypeChecked(t *testing.T) {
	env, err := NewModelEnv(
		cel.Constant("SHELL_NAMES", PatternsType, patternsValue(t, globValueOf(t, "/usr/bin/*"))),
	)
	require.NoError(t, err)

	tests := []struct{ expr, want string }{
		// an integer field against a set of patterns
		{`process.uid in SHELL_NAMES`, "no matching overload"},
		// and the other order, which CEL's own equality would have answered false to
		{`SHELL_NAMES == process.comm`, "no matching overload"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := CompileWithTypes(env, tt.expr, ModelFieldTypes{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}

	// What the type cannot reject is two sets compared with each other: CEL's
	// equality is declared `equals(A, A)` and unifies for any single type. Nothing
	// translates to that, and a set answers it with an error rather than a verdict.
	_, err = CompileWithTypes(env, `SHELL_NAMES == SHELL_NAMES`, ModelFieldTypes{})
	require.NoError(t, err, "cel-go's equality unifies for two values of one type")

	// The point of the type is not to reject everything, so the rule that means
	// something still compiles and still matches.
	assert.False(t, evalSECL(t, env, patternEvent(), `process.comm in SHELL_NAMES`))
	assert.True(t, evalSECL(t, env, patternEvent(), `process.file.path in SHELL_NAMES`))
}

// TestPatternMembershipIsOneCall pins the two properties the design is for, on the
// optimized AST rather than on the source: the subject is read once, and nothing reaches
// CEL's own equality carrying a pattern.
func TestPatternMembershipIsOneCall(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	checked, err := CompileWithTypes(env, `process.comm in [ "a", ~"/b/*", ~"/c/*", r"^d" ]`, ModelFieldTypes{})
	require.NoError(t, err)

	optimized, _, err := Optimize(env, checked)
	require.NoError(t, err)

	calls := map[string]int{}
	celast.MatchDescendants(celast.NavigateAST(optimized.NativeRep()), func(e celast.NavigableExpr) bool {
		if e.Kind() == celast.CallKind {
			calls[e.AsCall().FunctionName()]++
		}
		return false
	})

	assert.Equal(t, 1, calls[MatchAnyFunc], "one membership test")
	assert.Equal(t, 1, calls[ReadStringFunc], "the subject is read once")
	assert.Zero(t, calls[operators.LogicalOr], "no disjunction, whatever the list holds")
	assert.Zero(t, calls[operators.Equals], "nothing compares a pattern")
	assert.Zero(t, calls[operators.In], "nothing hands a pattern to CEL's own membership")
}

// TestPatternSetIsPreparedAtPlanTime is the other half: the list becomes a searchable
// set when the rule is planned, so an event pays neither the list construction nor the
// pattern compilation nor the overload dispatch.
//
// It is checked on the planned interpretable because that is the only place the
// difference shows — the AST is the same either way.
func TestPatternSetIsPreparedAtPlanTime(t *testing.T) {
	env, err := NewModelEnv(cel.Constant("SHELL_NAMES", PatternsType,
		patternsValue(t, types.String("dash"), globValueOf(t, "/usr/bin/*"))))
	require.NoError(t, err)

	for _, expr := range []string{
		// written out, which the folding turns into a value first
		`process.comm in [ "a", ~"/b/*", ~"/c/*" ]`,
		// named, where the value was already one
		`process.comm in SHELL_NAMES`,
	} {
		t.Run(expr, func(t *testing.T) {
			rule, err := NewRule(env, expr, ModelFieldTypes{})
			require.NoError(t, err)

			prepared, ok := rule.plan.(*matchAnyIn)
			require.True(t, ok, "planned as %T rather than a prepared membership", rule.plan)
			assert.NotEmpty(t, prepared.set.matchers, "the patterns are compiled")
		})
	}
}

// TestPatternsRejectTheImpossible covers the errors the constructors report, which are
// the rule author's own mistakes and have to reach them as such.
func TestPatternsRejectTheImpossible(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	// A regexp Go itself would reject. The pattern is compiled when the rule is
	// planned, so this is a rule that does not load rather than one that errors per
	// event — which is what the folding of the constructors buys.
	_, err = NewRule(env, `process.comm in [ "a", r"(" ]`, ModelFieldTypes{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a valid regexp")

	// And the same for a list SECL would not have brought here, which cannot be
	// searched for a string.
	_, err = newPatternSet(types.DefaultTypeAdapter.NativeToValue([]any{1, 2}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be a member of a pattern list")
}

// patternsValue prepares a set the way a macro's value reaches its declaration.
func patternsValue(t *testing.T, members ...ref.Val) ref.Val {
	t.Helper()

	set, err := newPatternSet(types.NewDynamicList(types.DefaultTypeAdapter, members))
	require.NoError(t, err)
	return set
}

func globValueOf(t *testing.T, pattern string) ref.Val {
	t.Helper()

	value, err := newGlobValue(pattern)
	require.NoError(t, err)
	return value
}

func regexpValueOf(t *testing.T, pattern string) ref.Val {
	t.Helper()

	value, err := newRegexpValue(pattern)
	require.NoError(t, err)
	return value
}

// TestPatternSetString covers the rendering, which is what an error message and the
// coverage tool show for a set.
func TestPatternSetString(t *testing.T) {
	set := patternsValue(t, types.String("dash"), globValueOf(t, "/usr/bin/*"), regexpValueOf(t, "^z"))

	rendered := set.ConvertToType(types.StringType).Value().(string)
	for _, want := range []string{`"dash"`, `/usr/bin/*`, `^z`} {
		assert.True(t, strings.Contains(rendered, want), "%s does not mention %s", rendered, want)
	}
}

// TestAllInIsMembershipLikeSECL pins the meaning `allin` has in SECL, which is not the
// one its name suggests: outside CIDR arrays, SECL's compiler routes it to the same
// evaluator as `in` and only `notin` is treated differently (eval.go, the
// *StringEvaluator and *StringArrayEvaluator cases). So a process whose arguments are
// `-l --color` satisfies `process.argv allin [ "-l" ]`.
//
// The translation reproduces that rather than the documented meaning, because what the
// shadow measures itself against is the engine in production. If SECL ever makes `allin`
// universal, this test is what says so.
func TestAllInIsMembershipLikeSECL(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := patternEvent()

	for _, expr := range []string{
		// one of two arguments is a member, which a universal quantifier would refuse
		`process.argv allin [ "-l" ]`,
		`process.argv allin [ ~"--*" ]`,
		// and a scalar subject, where the operator has no array to quantify at all
		`process.comm allin [ "sh", "zsh" ]`,
	} {
		t.Run(expr, func(t *testing.T) {
			assert.True(t, evalSECLEngine(t, event, expr), "SECL treats allin as in")
			assert.True(t, evalSECL(t, env, event, expr), "so the translation must too")
		})
	}
}

// TestPathGlobsDivergeFromSECL records a divergence the differential above is
// deliberately written around, and which the pattern values inherit rather than
// introduce.
//
// SECL compiles `~"…"` into one of two matchers depending on the *field*: a path field
// carries an operator override that turns a pattern into a glob (eval.GlobCmp, reached
// from OverlayFSPathname and ProcessSymlinkPathname on unix, from CaseInsensitiveCmp and
// WindowsPathCmp on windows), where `*` stops at a path separator and `**` is allowed.
// Every other string field keeps pattern semantics, where `*` crosses everything and
// `**` is refused.
//
// The translation always compiles a pattern, so a path glob is more permissive here than
// in SECL — CEL matches events SECL would not — and a rule written with `**`, which the
// real policies do use, fails to plan at all. Both directions are visible below.
func TestPathGlobsDivergeFromSECL(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := patternEvent() // process.file.path is /usr/bin/bash

	// A single star does not cross a separator in SECL, but does here.
	const permissive = `process.file.path =~ "/usr/*"`
	assert.False(t, evalSECLEngine(t, event, permissive), "SECL globs a path field")
	assert.True(t, evalSECL(t, env, event, permissive), "the translation patterns it")

	// And what SECL allows only for a path field is refused for every field here.
	_, err = NewRule(env, `process.file.path =~ "/usr/**"`, ModelFieldTypes{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`**` is not allowed in patterns")
	assert.True(t, evalSECLEngine(t, event, `process.file.path =~ "/usr/**"`),
		"SECL accepts it and matches")
}
