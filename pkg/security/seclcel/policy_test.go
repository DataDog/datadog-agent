// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"net"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// policyMacros are the shapes the agent's own rule set uses: 17 plain string lists, 30
// lists holding patterns, 3 integer flag expressions — and, here, one that refers to
// another, which the real set does not yet do.
var policyMacros = []Macro{
	{ID: "SHELL_NAMES", Expression: `[ "sh", "bash", "zsh" ]`},
	{ID: "CREDENTIAL_PATHS", Expression: `[ ~"/home/*/.docker/config.json", "/root/.dockercfg" ]`},
	{ID: "WRITE_FLAGS", Expression: `O_WRONLY|O_RDWR|O_APPEND`},
	{ID: "SHELLS_AND_MORE", Expression: `[ "dash", "fish" ]`},
	{ID: "PORTS", Expression: `[ 22, 443 ]`},
}

func TestPolicyEnvDeclaresMacros(t *testing.T) {
	policy, err := NewPolicyEnv(Policy{Macros: policyMacros})
	require.NoError(t, err)
	require.Empty(t, policy.MacroFailures)
	env := policy.Env

	event := patternEvent() // comm sh, file.path /usr/bin/bash, argv -l --color

	tests := []struct {
		expr string
		want bool
	}{
		// a plain list, which the planner prepares into a set
		{`process.comm in SHELL_NAMES`, true},
		{`process.comm not in SHELL_NAMES`, false},
		{`process.comm in SHELLS_AND_MORE`, false},

		// a list holding patterns, where the pattern-ness is what a macro would
		// otherwise lose
		{`open.file.path in CREDENTIAL_PATHS`, false},

		// an integer expression, folded to one value
		{`open.flags & WRITE_FLAGS > 0`, false},

		// and a list of integers
		{`process.uid in PORTS`, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			assert.Equal(t, tt.want, evalSECL(t, env, event, tt.expr))
		})
	}
}

// TestMacroPatternsKeepTheirSemantics is the equivalence the whole approach rests on: a
// macro evaluated once at load answers what SECL answers when it inlines the same list
// into the rule — including the per-field matcher, which SECL resolves *after* inlining
// and which a single prepared value therefore has to serve both ways.
func TestMacroPatternsKeepTheirSemantics(t *testing.T) {
	const members = `[ ~"/usr/*", ~"/opt/**" ]`

	policy, err := NewPolicyEnv(Policy{Macros: []Macro{{ID: "PATHS", Expression: members}}})
	require.NoError(t, err)
	require.Empty(t, policy.MacroFailures)
	env := policy.Env

	event := patternEvent()

	for _, field := range []string{"process.file.path", "process.file.name", "process.comm"} {
		for _, op := range []string{"in", "not in"} {
			t.Run(field+" "+op, func(t *testing.T) {
				written := field + " " + op + " " + members
				through := field + " " + op + " PATHS"

				// A pattern field cannot take `**` at all, in either engine, so the
				// comparison to make is between what each engine does with it.
				secl, seclErr := evalSECLEngineOrError(t, event, written)
				cel, celErr := evalOrError(t, env, event, through)

				if seclErr != nil {
					assert.Error(t, celErr, "SECL refused %q with %v", written, seclErr)
					return
				}
				require.NoError(t, celErr)
				assert.Equal(t, secl, cel, "the macro and the list written out disagree")
			})
		}
	}
}

func TestPolicyEnvReportsWhatItCannotDeclare(t *testing.T) {
	policy, err := NewPolicyEnv(Policy{Macros: []Macro{
		{ID: "FINE", Expression: `[ "a" ]`},
		// reads an event, so it has no value at load
		{ID: "READS_A_FIELD", Expression: `process.comm`},
		// names something no policy declares
		{ID: "UNDECLARED", Expression: `NOT_A_MACRO`},
		// and a cycle, which is what the repetition has to stop on rather than
		// spin over
		{ID: "LEFT", Expression: `RIGHT`},
		{ID: "RIGHT", Expression: `LEFT`},
	}})
	require.NoError(t, err, "a policy load is never failed by a macro")

	reasons := map[string]string{}
	for _, failure := range policy.MacroFailures {
		reasons[failure.ID] = failure.Err.Error()
	}

	assert.Len(t, reasons, 4)
	assert.Contains(t, reasons["READS_A_FIELD"], "unsupported macro")
	assert.Contains(t, reasons["UNDECLARED"], "undeclared reference")
	assert.Contains(t, reasons, "LEFT")
	assert.Contains(t, reasons, "RIGHT")

	// What did compile is still declared, which is what lets a broken macro cost only
	// the rules that name it.
	assert.False(t, evalSECL(t, policy.Env, patternEvent(), `process.comm in FINE`))
}

// TestMacroReferringToAnotherMacro pins the ordering the repetition provides: a macro is
// declared once what it names is, whatever order the policy lists them in.
func TestMacroReferringToAnotherMacro(t *testing.T) {
	policy, err := NewPolicyEnv(Policy{Macros: []Macro{
		{ID: "OUTER", Expression: `"a" in INNER`},
		{ID: "INNER", Expression: `[ "a", "b" ]`},
	}})
	require.NoError(t, err)
	require.Empty(t, policy.MacroFailures)

	assert.True(t, evalSECL(t, policy.Env, patternEvent(), `OUTER && process.comm == "sh"`))
}

// evalOrError is evalSECL without the assertion, for a case where failing to compile is
// one of the answers.
func evalOrError(t *testing.T, env *cel.Env, event *model.Event, expr string) (bool, error) {
	t.Helper()

	rule, err := NewRule(env, expr, ModelFieldTypes{})
	if err != nil {
		return false, err
	}
	return rule.Eval(NewActivation(eval.NewContext(event)))
}

// evalSECLEngineOrError is evalSECLEngine without the assertion, for the same reason.
func evalSECLEngineOrError(t *testing.T, event *model.Event, expr string) (bool, error) {
	t.Helper()

	var m model.Model
	rule, err := eval.NewRule("differential", expr, ast.NewParsingContext(false), &eval.Opts{
		Constants:    model.SECLConstants(),
		LegacyFields: model.SECLLegacyFields,
	})
	if err != nil {
		return false, err
	}
	if err := rule.GenEvaluator(&m); err != nil {
		return false, err
	}
	return rule.Eval(eval.NewContext(event)), nil
}

// variableStore builds the store a policy's `set:` actions would have filled, one variable
// per shape SECL compiles them into.
func variableStore(t *testing.T) *eval.VariableStore {
	t.Helper()

	_, network, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	store := &eval.VariableStore{}
	store.Add("correlation_key", eval.NewStringVariable("abc", eval.VariableOpts{}))
	store.Add("attempts", eval.NewIntVariable(3, eval.VariableOpts{}))
	store.Add("flagged", eval.NewBoolVariable(true, eval.VariableOpts{}))
	store.Add("peer", eval.NewIPVariable(*network, eval.VariableOpts{}))
	store.Add("categories", eval.NewStringArrayVariable([]string{"a", "b"}, eval.VariableOpts{}))
	store.Add("ports", eval.NewIntArrayVariable([]int{22, 443}, eval.VariableOpts{}))
	store.Add("peers", eval.NewIPArrayVariable([]net.IPNet{*network}, eval.VariableOpts{}))
	return store
}

// TestPolicyEnvReadsVariables covers each shape a `${…}` variable can have, read through
// the closure SECL compiled for it rather than through anything of ours — which is what
// keeps the scoping, the inheritance and the TTL production's rather than a
// reimplementation.
func TestPolicyEnvReadsVariables(t *testing.T) {
	policy, err := NewPolicyEnv(Policy{Variables: variableStore(t)})
	require.NoError(t, err)
	require.Empty(t, policy.VariableFailures)

	event := patternEvent()

	tests := []struct {
		expr string
		want bool
	}{
		// a scalar of each shape
		{`${correlation_key} == "abc"`, true},
		{`${correlation_key} == "other"`, false},
		{`${attempts} > 2`, true},
		{`${flagged} == true`, true},
		{`${peer} == 10.0.0.0/8`, true},
		{`${peer} == 192.168.0.0/16`, false},

		// a variable holding a list is compared the way a field holding one is: some
		// element matches
		{`${categories} == "a"`, true},
		{`${categories} == "z"`, false},
		{`${categories} in [ "b", "c" ]`, true},
		{`${ports} == 443`, true},
		{`${peers} == 10.1.2.3`, true},

		// the length suffix, which SECL derives rather than stores
		{`${correlation_key.length} == 3`, true},
		{`${categories.length} == 2`, true},

		// a pattern against a variable, which needs the same per-element treatment
		{`${categories} =~ "a*"`, true},

		// and interpolation, which is what a rule correlating an event with stored
		// state is written with
		{`"proc-${correlation_key}" == "proc-abc"`, true},

		// against a field, which is the shape the real rules use
		{`process.comm == "sh" && ${flagged} == true`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			rule, err := NewRule(policy.Env, tt.expr, policy.FieldTypes)
			require.NoError(t, err)

			matched, err := rule.Eval(policy.Activation(eval.NewContext(event)))
			require.NoError(t, err)
			assert.Equal(t, tt.want, matched)
		})
	}
}

// TestVariablesAreTypeChecked is what declaring a variable's type buys over leaving it
// dynamic: a comparison that could never hold is a compile error rather than a rule that
// never fires.
//
// It is also a divergence, and the stricter direction: SECL accepts `${some_string} == 1`
// and answers false.
func TestVariablesAreTypeChecked(t *testing.T) {
	policy, err := NewPolicyEnv(Policy{Variables: variableStore(t)})
	require.NoError(t, err)

	for _, expr := range []string{
		`${correlation_key} == 1`,
		`${attempts} == "three"`,
		`${categories} == 1`,
		// and a name no policy declares, which SECL's compiler also refuses
		`${not_a_variable} == "x"`,
	} {
		t.Run(expr, func(t *testing.T) {
			_, err := NewRule(policy.Env, expr, policy.FieldTypes)
			require.Error(t, err)
		})
	}
}

// TestVariablesAreReadPerEvaluation pins that a variable is read when the rule runs, not
// when it is planned: the store is filled by actions between events, and a rule holding
// the value it had at load would be wrong from the first write.
func TestVariablesAreReadPerEvaluation(t *testing.T) {
	store := &eval.VariableStore{}
	variable := eval.NewStringVariable("before", eval.VariableOpts{})
	store.Add("state", variable)

	policy, err := NewPolicyEnv(Policy{Variables: store})
	require.NoError(t, err)

	rule, err := NewRule(policy.Env, `${state} == "after"`, policy.FieldTypes)
	require.NoError(t, err)

	activation := policy.Activation(eval.NewContext(patternEvent()))

	matched, err := rule.Eval(activation)
	require.NoError(t, err)
	assert.False(t, matched)

	require.NoError(t, variable.Set(nil, "after"))

	matched, err = rule.Eval(activation)
	require.NoError(t, err)
	assert.True(t, matched, "the same rule sees the new value")
}
