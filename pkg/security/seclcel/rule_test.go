// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// agreementRules are shapes whose planning differs: a plain field read, a read
// through a handler, the three whose literals are bound when the rule is planned,
// and the comprehensions an iterated field becomes. Each is written twice so that
// agreeing is not simply both sides saying true.
var agreementRules = []string{
	`process.comm == "sh"`,
	`process.comm == "zsh"`,
	`process.file.path == "/usr/bin/bash"`,
	`process.file.path == "/usr/bin/zsh"`,
	`process.comm in [ "zsh", "bash", "sh" ]`,
	`process.comm in [ "zsh", "bash" ]`,
	`process.file.name == r"^ba.*"`,
	`process.file.name == r"^zs.*"`,
	`process.file.path =~ "/usr/bin/*"`,
	`process.file.path =~ "/opt/*"`,
	`process.ancestors.comm == "sshd"`,
	`process.ancestors.comm == "nothing"`,
	`process.ancestors.comm allin [ "bash", "sshd" ]`,
	`process.ancestors[A].comm == "bash" && process.ancestors[A].uid == 1000`,
	`process.ancestors[A].comm == "bash" && process.ancestors[A].uid == 0`,
}

// TestRuleAgreesWithCELProgram is the guard on the planner NewRule rolls by hand.
//
// planRule reproduces what cel.Env.Program does for the options a rule is planned
// with, so that a rule can be evaluated as an interpretable rather than through
// the cel.Program wrapper. Reproducing it is only safe while the two stay the
// same, and a cel-go bump is what would part them — newProgram gaining a step
// that planRule does not take would show up as a rule quietly evaluating to
// something else. This is where it shows up instead.
func TestRuleAgreesWithCELProgram(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.Comm = "sh"
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/bash"
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"
	ancestry(event, []string{"bash", "sshd"}, []uint32{1000, 0})

	for _, expr := range agreementRules {
		t.Run(expr, func(t *testing.T) {
			checked, err := CompileWithTypes(env, expr, ModelFieldTypes{})
			require.NoError(t, err)

			// The pass is not optional: it is what turns a field chain into a read,
			// and the type provider serves no getters for the chain it would leave.
			optimized, _, err := Optimize(env, checked)
			require.NoError(t, err)

			// The program options newProgram derives plannerOptions from, which is
			// the pairing planRule reproduces — readOptimization included, since
			// readDecorator is the same hook in the form a hand-rolled planner takes.
			program, err := env.Program(optimized,
				cel.EvalOptions(cel.OptOptimize), cel.OptimizeRegex(globOptimization), readOptimization())
			require.NoError(t, err)

			ctx := eval.NewContext(event)
			want, _, err := program.Eval(NewActivation(ctx))
			require.NoError(t, err)

			rule, err := NewRule(env, expr, ModelFieldTypes{})
			require.NoError(t, err)

			ctx.Reset()
			ctx.SetEvent(event)
			got, err := rule.Eval(NewActivation(ctx))
			require.NoError(t, err)

			assert.Equal(t, want == types.True, got, "the two planners disagree")
		})
	}
}

// TestRuleRejectsANonBooleanResult covers the last arm of Rule.Eval. A rule is a
// boolean expression, but nothing about the translation enforces that, so a bare
// field name reaches the planner and evaluates to what it holds.
func TestRuleRejectsANonBooleanResult(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	rule, err := NewRule(env, `process.comm`, ModelFieldTypes{})
	require.NoError(t, err, "it translates and type-checks")

	_, err = rule.Eval(NewActivation(eval.NewContext(model.NewFakeEvent())))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a boolean")
}
