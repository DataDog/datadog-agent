// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"errors"
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"
)

// A rule is compiled against the model *and* against what its policy declares: the
// macros it refers to by name, and the `${…}` variables its actions maintain.
//
// A macro needs nothing from the front end. All of them in the agent's own rule set are
// values — lists of strings, lists of patterns, integer flag expressions — so each is
// translated, evaluated once here, and declared as a cel.Constant. The rule that names it
// then reads a value out of its own expression, exactly as if the list had been written
// into it, and pays nothing per event: BenchmarkPatternMembership measures the two at the
// same figure.
//
// That is only possible because a pattern is a value — see patterns.go. Compiling a macro
// to an ordinary CEL list would turn `x in GLOB_MACRO` into equality against strings
// holding stars, which is a wrong verdict rather than an error.
//
// A macro that reads a *field* cannot be a constant, and none in the rule set does. It is
// reported here rather than mistranslated, and cel-go's NewInliningOptimizer is the answer
// if one ever appears.

// Macro is a macro as a policy declares it.
type Macro struct {
	ID         string
	Expression string
}

// MacroFailure is a macro that could not become a constant, and why.
type MacroFailure struct {
	ID  string
	Err error
}

func (f MacroFailure) String() string { return f.ID + ": " + f.Err.Error() }

// Policy is what a rule set contributes to the environment beyond the model.
type Policy struct {
	Macros []Macro
}

// NewPolicyEnv builds the environment a policy's rules are compiled against: the model,
// plus one constant per macro, plus whatever else the caller declares.
//
// Macros are resolved by repetition rather than by ordering, because one may refer to
// another: each round declares those that compile and evaluate against what is declared
// so far, and stops when a round adds nothing. What is left over is returned as failures —
// a macro that names something undeclared, that reads a field, or that is part of a cycle —
// and the environment is still usable without them. A policy load never fails here; a rule
// that needed a failed macro fails to compile, and is counted where the other unsupported
// rules are.
func NewPolicyEnv(policy Policy, opts ...cel.EnvOption) (*cel.Env, []MacroFailure, error) {
	env, err := NewModelEnv(opts...)
	if err != nil {
		return nil, nil, err
	}

	pending := make(map[string]string, len(policy.Macros))
	for _, macro := range policy.Macros {
		pending[macro.ID] = macro.Expression
	}

	failures := map[string]error{}
	for len(pending) > 0 {
		declared := 0

		for _, id := range sortedMacroIDs(pending) {
			constant, err := macroConstant(env, id, pending[id])
			if err != nil {
				// It may only be waiting for another macro, so the reason is kept and
				// overwritten on the next round rather than reported now.
				failures[id] = err
				continue
			}

			next, err := env.Extend(constant)
			if err != nil {
				failures[id] = err
				continue
			}

			env = next
			delete(pending, id)
			delete(failures, id)
			declared++
		}

		if declared == 0 {
			break
		}
	}

	return env, sortedFailures(failures), nil
}

// macroConstant evaluates a macro and declares it as the value it evaluates to.
//
// The type comes from the checker rather than from the value, so a macro is declared as
// what its expression means: a prepared set of patterns, a list of strings, an integer.
// That is what keeps a rule using it type-checked — `process.uid in SHELL_NAMES` against a
// set of patterns is a compile error.
func macroConstant(env *cel.Env, id, expression string) (cel.EnvOption, error) {
	translated, err := translateMacro(expression)
	if err != nil {
		return nil, err
	}

	checked, iss := env.Compile(translated)
	if iss.Err() != nil {
		return nil, iss.Err()
	}

	program, err := env.Program(checked)
	if err != nil {
		return nil, err
	}

	// No activation, so a macro that reads a field fails here rather than returning
	// something an event would have decided.
	value, _, err := program.Eval(map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("%w: evaluating it needs more than the policy: %w", errUnsupportedMacro, err)
	}

	return cel.Constant(id, checked.OutputType(), value), nil
}

// translateMacro renders a macro body as CEL source, which is what env.Compile takes.
func translateMacro(expression string) (string, error) {
	a, err := ParseMacro(expression, ModelFieldTypes{})
	if err != nil {
		return "", err
	}
	return cel.ExprToString(a.Expr(), a.SourceInfo())
}

// errUnsupportedMacro is what a macro that cannot be a constant is reported as.
var errUnsupportedMacro = errors.New("unsupported macro")

// sortedMacroIDs keeps each round of resolution in one order, so that a policy declaring
// several broken macros reports the same reasons every time.
func sortedMacroIDs(macros map[string]string) []string {
	ids := make([]string, 0, len(macros))
	for id := range macros {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedFailures(failures map[string]error) []MacroFailure {
	if len(failures) == 0 {
		return nil
	}

	ids := make([]string, 0, len(failures))
	for id := range failures {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]MacroFailure, 0, len(ids))
	for _, id := range ids {
		out = append(out, MacroFailure{ID: id, Err: failures[id]})
	}
	return out
}
