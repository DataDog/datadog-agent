// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package serializers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// The values matched in the environment variables of a process are never
// serialized, even for the keys of `envs_with_value`.
func TestRuleContextEnvValues(t *testing.T) {
	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	scrubber.WithEnvsWithValue([]string{"PATH"})

	for _, test := range []struct {
		name     string
		expr     string
		field    string
		value    interface{}
		expected string
	}{
		{"key-not-listed", `exec.env["AWS_SECRET_ACCESS_KEY"] =~ r".+"`, "exec.env", "secret", "********"},
		{"key-listed", `exec.env["PATH"] == "/bin"`, "exec.env", "/bin", "********"},
		{"key-listed-spaced", `exec.env [ "PATH" ] == "/bin"`, "exec.env", "/bin", "********"},
		{"key-evaluated", `exec.env[exec.file.name] == "x"`, "exec.env", "secret", "********"},
		{"iterated", `process.ancestors.env["TOKEN"] =~ "ghp_*"`, "process.ancestors.env", []string{"ghp_a", "ghp_b"}, "[******** ********]"},
		{"iterated-listed", `process.ancestors.env["PATH"] == "/bin"`, "process.ancestors.env", []string{"/bin", "/usr/bin"}, "[******** ********]"},
		{"not-env", `exec.file.name == "ls"`, "exec.file.name", "ls", "ls"},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := model.NewFakeEvent()
			offset := strings.Index(test.expr, test.field)
			event.RuleContext.MatchingSubExprs = eval.MatchingSubExprs{{
				Offset: offset,
				ValueA: eval.MatchingValue{Offset: offset, Field: test.field, Value: test.value},
			}}
			rule := &rules.Rule{Rule: &eval.Rule{Expression: test.expr}}

			ruleContext := newRuleContext(event, rule, scrubber)
			require.Len(t, ruleContext.MatchingSubExprs, 1)
			assert.Equal(t, test.expected, ruleContext.MatchingSubExprs[0].Value)
			assert.Equal(t, test.field, ruleContext.MatchingSubExprs[0].Field)
		})
	}
}

// A secret matched in an environment variable isn't serialized wherever it is
// written: in a macro, through an iterator, or after the value it is compared to,
// next to a key listed in `envs_with_value`.
func TestRuleContextEnvValuesEvaluated(t *testing.T) {
	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	scrubber.WithEnvsWithValue([]string{"PATH"})

	for _, expr := range []string{
		`process.env["SECRET"] =~ r".+" && process.env["PATH"] == "/bin"`,
		`secret_macro && process.env["PATH"] == "/bin"`,
		`process.ancestors.env["SECRET"] =~ r".+" && process.ancestors.env["PATH"] == "/bin"`,
		`process.ancestors[A].env["SECRET"] =~ r".+" && process.ancestors.env["PATH"] == "/bin"`,
	} {
		t.Run(expr, func(t *testing.T) {
			env := map[string]string{"SECRET": "secret", "PATH": "/bin"}
			event := model.NewFakeEvent()
			event.ProcessContext.Process.Env = env
			ancestor := &model.ProcessCacheEntry{}
			ancestor.Process.Env = env
			event.ProcessContext.Ancestor = ancestor

			m := &model.Model{}
			pc := parser.NewParsingContext(false)
			opts := (&eval.Opts{}).WithMacroStore(&eval.MacroStore{}).WithVariables(map[string]eval.SECLVariable{})
			macro, err := eval.NewMacro("secret_macro", `process.env["SECRET"] =~ r".+"`, m, pc, opts)
			require.NoError(t, err)
			opts.AddMacro(macro)

			evalRule, err := eval.NewRule("id", expr, pc, opts)
			require.NoError(t, err)
			require.NoError(t, evalRule.GenEvaluator(m))

			ctx := eval.NewContext(event)
			require.True(t, evalRule.Eval(ctx))
			event.RuleContext.MatchingSubExprs = ctx.GetMatchingSubExprs()

			ruleContext := newRuleContext(event, &rules.Rule{Rule: evalRule}, scrubber)
			require.NotEmpty(t, ruleContext.MatchingSubExprs)
			for _, subExpr := range ruleContext.MatchingSubExprs {
				assert.NotContains(t, subExpr.Value, "secret")
			}
		})
	}
}

// The variables read from environment variables are serialized only when they
// are all listed in `envs_with_value`, and none is evaluated per event.
func TestVariablesEnvValues(t *testing.T) {
	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	scrubber.WithEnvsWithValue([]string{"PATH"})

	store := &eval.VariableStore{}
	for name, opts := range map[string]eval.VariableOpts{
		"plain":     {},
		"listed":    {EnvKeys: []string{"PATH"}},
		"unlisted":  {EnvKeys: []string{"TOKEN"}},
		"mixed":     {EnvKeys: []string{"PATH", "TOKEN"}},
		"evaluated": {EnvKeyEvaluated: true},
	} {
		store.Add(name, eval.NewStringVariable("value", opts))
	}
	rule := &rules.Rule{Rule: &eval.Rule{Opts: &eval.Opts{VariableStore: store}}}

	variables := newVariablesContext(model.NewFakeEvent(), rule, "", scrubber)
	assert.Equal(t, Variables{"plain": "value", "listed": "value"}, variables)

	// without the list, none of them is serialized
	variables = newVariablesContext(model.NewFakeEvent(), rule, "", nil)
	assert.Equal(t, Variables{"plain": "value"}, variables)
}

// The variables holding envp entries copied as is are serialized entry by entry,
// with a value only for the keys of `envs_with_value`.
func TestVariablesEnvpValues(t *testing.T) {
	store := &eval.VariableStore{}
	store.Add("entries", eval.NewStringArrayVariable([]string{"PATH=/bin", "AWS_SECRET_ACCESS_KEY=secret", "PATHX=secret", "NO_VALUE"}, eval.VariableOpts{EnvFiltered: true}))
	store.Add("listed", eval.NewStringVariable("PATH=/bin", eval.VariableOpts{EnvFiltered: true}))
	store.Add("unlisted", eval.NewStringVariable("TOKEN=secret", eval.VariableOpts{EnvFiltered: true}))
	store.Add("hidden", eval.NewStringVariable("TOKEN=secret", eval.VariableOpts{EnvFiltered: true, EnvKeyEvaluated: true}))
	store.Add("plain", eval.NewStringVariable("TOKEN=value", eval.VariableOpts{}))
	rule := &rules.Rule{Rule: &eval.Rule{Opts: &eval.Opts{VariableStore: store}}}

	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	scrubber.WithEnvsWithValue([]string{"PATH"})

	for _, test := range []struct {
		name     string
		scrubber *utils.Scrubber
		entries  []string
		others   Variables
	}{
		{"listed", scrubber, []string{"PATH=/bin", "AWS_SECRET_ACCESS_KEY", "PATHX", "NO_VALUE"}, Variables{"listed": "PATH=/bin", "unlisted": "TOKEN", "plain": "TOKEN=value"}},
		// without the list, only the keys are serialized
		{"no-list", nil, []string{"PATH", "AWS_SECRET_ACCESS_KEY", "PATHX", "NO_VALUE"}, Variables{"listed": "PATH", "unlisted": "TOKEN", "plain": "TOKEN=value"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			variables := newVariablesContext(model.NewFakeEvent(), rule, "", test.scrubber)
			// the values of an array variable are unordered
			assert.ElementsMatch(t, test.entries, variables["entries"])
			delete(variables, "entries")
			assert.Equal(t, test.others, variables)
		})
	}
}
