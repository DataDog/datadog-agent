// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// TestOptimizedReadsMatchTheLayout is the guard on the index: for every field in
// the model, what a planned expression reads must be what the field's own reader
// reads.
//
// It is the link the rewrite adds, and the only one not covered elsewhere.
// TestGeneratedReadersAgreeWithEvaluators ties each reader to the accessor for the
// same field; this ties each field name to that reader through the index the pass
// resolved it to. A layout that shifted under the pass, or a chain resolved to the
// wrong path, shows up here as a value read from the wrong field.
func TestOptimizedReadsMatchTheLayout(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := populatedEvent()
	ctx := eval.NewContext(event)

	var direct, iterated int
	for _, field := range sortedKeys(celReaderIndex) {
		reader := celReaders[celReaderIndex[field]]

		// A field of an iterated element is reached through a subscript, which is
		// also the shape that keeps a chain standing on another rewrite.
		expr, element := "evt."+field, any(nil)
		if prefix := (ModelFieldTypes{}).ListPrefix(field); prefix != "" {
			element = celIterators[celIteratorIndex[prefix]](ctx).next()
			require.NotNil(t, element, "field %q is iterated over nothing", field)

			expr = fmt.Sprintf("evt.%s[0].%s", prefix, strings.TrimPrefix(field, prefix+"."))
			iterated++
		} else {
			direct++
		}

		out := optimizedPlan(t, env, expr).Exec(NewActivation(ctx).frame)
		require.False(t, types.IsError(out), "evaluating %q: %s", expr, out)

		assertSameRead(t, expr, field, reader(ctx, element), out)
	}

	require.Greater(t, direct, 1000, "expected the whole field set to be covered")
	require.Greater(t, iterated, 100, "expected the iterated fields to be covered")
}

// TestNoFieldChainSurvivesThePass is the invariant that lets the type provider
// serve types without getters: a select over a SECL type never reaches the
// interpreter, because the pass turned every one of them into a read.
//
// Checked over the corpus, so it is the real rules that say the pass covers the
// shapes SECL produces rather than the shapes the tests thought of.
func TestNoFieldChainSurvivesThePass(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	var optimized int
	for _, expr := range corpusWithTypes(t, env) {
		checked, err := CompileWithTypes(env, expr, ModelFieldTypes{})
		require.NoError(t, err)

		result, _, err := Optimize(env, checked)
		if !assert.NoError(t, err, "%s\n  does not optimize", expr) {
			continue
		}
		optimized++

		for _, node := range celast.MatchDescendants(celast.NavigateAST(result.NativeRep()),
			celast.KindMatcher(celast.SelectKind)) {
			operand := node.Children()[0]
			_, isField := modelPaths[operand.Type().TypeName()]
			assert.False(t, isField, "%s\n  still selects %q on a %s",
				expr, node.AsSelect().FieldName(), operand.Type())
		}
	}

	require.Greater(t, optimized, 250, "expected the real-model expressions to optimize")
}

// TestCorpusEvaluates runs the corpus rather than only planning it, which is what
// catches a rewrite that plans but reads nothing, or reads the wrong shape.
//
// Two kinds of expression cannot get that far, and both are accounted for rather
// than skipped, so that a third kind appearing is a failure.
func TestCorpusEvaluates(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := populatedEvent()
	activation := NewActivation(eval.NewContext(event))

	var evaluated int
	for _, expr := range corpusWithTypes(t, env) {
		rule, err := NewRule(env, expr, ModelFieldTypes{})
		if err != nil {
			// A glob is compiled when the rule is planned, which is what makes it
			// affordable, and the compiler rejects a pattern SECL's own parser
			// accepts.
			assert.Contains(t, err.Error(), "`**` is not allowed in patterns",
				"%s\n  does not plan", expr)
			continue
		}

		// The plan rather than Rule.Eval, because the corpus holds macro bodies as
		// well as rules — `3 & 3` and `"{{.Root}}/x"` evaluate to an integer and a
		// string — and what is under test is that the pass handles them, not that
		// they answer a verdict.
		out := rule.plan.Exec(activation.frame)
		if types.IsError(out) {
			// SECL's ${…} variables are declared but not populated — see
			// TestProgramVariablesAreNotWired.
			assert.Contains(t, out.(*types.Err).Error(), VariablesRoot, "%s\n  does not evaluate", expr)
			continue
		}

		assert.NotNil(t, out)
		evaluated++
	}

	require.Greater(t, evaluated, 250, "expected the real-model expressions to evaluate")
}

// TestProgramReportsTheFieldsItReads pins what the pass knows on the way past:
// resolving a name is what it does, so the fields a rule reads come for free — the
// counterpart of SECL's RuleEvaluator.GetFields.
func TestProgramReportsTheFieldsItReads(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	tests := []struct {
		expr string
		want []string
	}{
		{`process.comm == "sh"`, []string{"process.comm"}},
		// a pseudo field is read through the field it derives from
		{`process.argv.length > 2`, []string{"process.argv"}},
		{`dns.question.name.root_domain == "example.com"`, []string{"dns.question.name"}},
		// an iterated node is a read of its own, and the leaf under it another
		{`process.ancestors.file.name == "sh"`,
			[]string{"process.ancestors", "process.ancestors.file.name"}},
		{`process.ancestors[A].comm == "sh" && process.ancestors[A].uid == 0`,
			[]string{"process.ancestors", "process.ancestors.comm", "process.ancestors.uid"}},
		// both sides, and each field once however often it is mentioned
		{`open.file.path == open.file.name || open.file.path == "/etc/passwd"`,
			[]string{"open.file.name", "open.file.path"}},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			rule, err := NewRule(env, tt.expr, ModelFieldTypes{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, rule.Fields())
		})
	}
}

// TestOptimizeRejectsWhatItCannotRead pins that an unreadable field fails the rule
// rather than being left for cel-go to resolve, which is what makes the read by
// index the only path a field is read through.
func TestOptimizeRejectsWhatItCannotRead(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	const field = "process.comm"
	index := celReaderIndex[field]
	delete(celReaderIndex, field)
	t.Cleanup(func() { celReaderIndex[field] = index })

	_, err = NewRule(env, `process.comm == "sh"`, ModelFieldTypes{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `field "process.comm" has no reader`)
}

// TestOptimizeLeavesVariablesAlone checks the other side of the rule: only a field
// is rewritten, and a name the model does not know is left for the environment to
// resolve. It is what keeps the pass out of the way of the macros and the SECL
// variables.
func TestOptimizeLeavesVariablesAlone(t *testing.T) {
	env, err := NewModelEnv(
		cel.Variable("my_macro", cel.ListType(cel.StringType)),
		cel.Variable(VariablesRoot+".my.var", cel.StringType),
	)
	require.NoError(t, err)

	checked, err := CompileWithTypes(env, `process.comm in my_macro && ${my.var} == "x"`, ModelFieldTypes{})
	require.NoError(t, err)

	optimized, fields, err := Optimize(env, checked)
	require.NoError(t, err)
	assert.Equal(t, []string{"process.comm"}, fields, "only the field is a field")

	source, err := cel.AstToString(optimized)
	require.NoError(t, err)
	assert.Equal(t, `secl.matchAny(secl.readString(evt, `+strconv.Itoa(celReaderIndex["process.comm"])+
		`), my_macro) && vars.my.var == "x"`, source)
}

// assertSameRead compares two field reads through CEL equality rather than as Go
// values: a list value holds the closure that indexes it, which no two of them
// share even when they hold the same elements.
func assertSameRead(t *testing.T, expr, field string, want, got ref.Val) {
	t.Helper()

	if types.IsError(want) || types.IsError(got) {
		assert.Equal(t, fmt.Sprint(want), fmt.Sprint(got),
			"%q does not fail the way %q does", expr, field)
		return
	}

	assert.Equal(t, types.True, want.Equal(got),
		"%q reads %s where %q reads %s", expr, got, field, want)
}

// optimizedPlan plans a CEL expression — not a SECL one — through the pass, for
// the tests that need to name a field directly.
//
// It stops at the interpretable, as NewRule does, since these expressions read a
// field rather than answering a boolean and so have no Rule to be evaluated as.
func optimizedPlan(t *testing.T, env *cel.Env, expr string) interpreter.InterpretableV2 {
	t.Helper()

	checked, iss := env.Compile(expr)
	require.NoError(t, iss.Err(), "compiling %q", expr)

	optimized, _, err := Optimize(env, checked)
	require.NoError(t, err, "optimizing %q", expr)

	plan, err := planRule(env, optimized)
	require.NoError(t, err, "planning %q", expr)
	return plan
}

// corpusWithTypes returns the corpus expressions that type-check against the real
// model, which are the ones the pass has to handle.
func corpusWithTypes(t *testing.T, env *cel.Env) []string {
	t.Helper()

	raw, err := os.ReadFile("testdata/corpus.json")
	require.NoError(t, err)

	var corpus []string
	require.NoError(t, json.Unmarshal(raw, &corpus))

	typed := make([]string, 0, len(corpus))
	for _, expr := range corpus {
		if _, err := CompileWithTypes(env, expr, ModelFieldTypes{}); err == nil {
			typed = append(typed, expr)
		}
	}
	return typed
}
