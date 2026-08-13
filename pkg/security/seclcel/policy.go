// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"errors"
	"fmt"
	"net"
	"sort"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// A rule is compiled against the model *and* against what its policy declares: the
// macros it refers to by name, and the `${…}` variables its actions maintain.
//
// A variable is declared from the store the rule engine already keeps, so its type and
// its accessor both come from the closure SECL compiled for it — see Variables.
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

	// Variables is the store the rule engine keeps for a rule set —
	// rules.RuleSet.GetVariableStore. Nil declares none, and a rule reading one then
	// fails to compile rather than at evaluation.
	Variables *eval.VariableStore
}

// PolicyEnv is the environment a policy's rules are compiled and evaluated against,
// together with the field types that go with it and what the policy could not contribute.
//
// The failures are counted rather than fatal: a policy load that dropped every rule
// because one macro is malformed would be worse than one that drops the rules naming it.
type PolicyEnv struct {
	// Env is what CompileWithTypes and NewRule take.
	Env *cel.Env
	// FieldTypes is what to translate the policy's rules with. It answers for the
	// policy's variables as well as for the model's fields, which is what makes a
	// comparison against a variable holding a list mean "some element", as it does for
	// a field holding one.
	FieldTypes FieldTypes
	// Variables is the table Activation reads a `${…}` variable through.
	Variables *Variables

	// MacroFailures are the macros that could not become constants, and
	// VariableFailures the variables whose shape has no CEL type.
	MacroFailures    []MacroFailure
	VariableFailures []VariableFailure
}

// VariableFailure is a variable that could not be declared, and why.
type VariableFailure struct {
	Name string
	Err  error
}

func (f VariableFailure) String() string { return f.Name + ": " + f.Err.Error() }

// Activation returns the activation a rule compiled against this environment is
// evaluated with. It is what carries the policy's variables, so a rule reading one has to
// be evaluated through here rather than through NewActivation.
//
// An activation may be kept for as long as the context it is built on — see the note on
// the Activation type — so the rule engine's own pooling is what decides how often this
// is called, not the event rate.
func (p *PolicyEnv) Activation(ctx *eval.Context) *Activation {
	return newActivation(ctx, p.Variables)
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
func NewPolicyEnv(policy Policy, opts ...cel.EnvOption) (*PolicyEnv, error) {
	variables, variableFailures := declareVariables(policy.Variables)

	env, err := NewModelEnv(append(opts, variables.declarations...)...)
	if err != nil {
		return nil, err
	}

	pending := make(map[string]string, len(policy.Macros))
	for _, macro := range policy.Macros {
		pending[macro.ID] = macro.Expression
	}

	macroFailures := map[string]error{}
	for len(pending) > 0 {
		declared := 0

		for _, id := range sortedMacroIDs(pending) {
			constant, err := macroConstant(env, id, pending[id])
			if err != nil {
				// It may only be waiting for another macro, so the reason is kept and
				// overwritten on the next round rather than reported now.
				macroFailures[id] = err
				continue
			}

			next, err := env.Extend(constant)
			if err != nil {
				macroFailures[id] = err
				continue
			}

			env = next
			delete(pending, id)
			delete(macroFailures, id)
			declared++
		}

		if declared == 0 {
			break
		}
	}

	return &PolicyEnv{
		Env:              env,
		FieldTypes:       policyFieldTypes{lists: variables.lists},
		Variables:        variables.table,
		MacroFailures:    sortedFailures(macroFailures),
		VariableFailures: variableFailures,
	}, nil
}

// declaredVariables is what a store yields: the declarations to build the environment
// with, the table to read through, and which of them hold several values.
type declaredVariables struct {
	declarations []cel.EnvOption
	table        *Variables
	lists        map[string]bool
}

// policyFieldTypes is ModelFieldTypes extended with the policy's variables, which are
// fields as far as the array semantics are concerned: `${some_list} == "x"` means some
// element equals it, exactly as `process.argv == "x"` does.
//
// Only IsListLeaf answers differently. A variable is never an iterated node, has no legacy
// name and no pseudo field of its own — `${foo.length}` is translated where it is read —
// and it is not rooted at the event.
type policyFieldTypes struct {
	ModelFieldTypes
	lists map[string]bool
}

// IsListLeaf implements FieldTypes.
func (p policyFieldTypes) IsListLeaf(field string) bool {
	return p.lists[field] || p.ModelFieldTypes.IsListLeaf(field)
}

// declareVariables turns SECL's variable store into CEL declarations and readers.
//
// A variable is declared under its dotted name — `vars.foo`, which cel-go resolves as one
// ident rather than as a select on `vars` — because that is the shape `${foo}` already
// translates to. What it is declared *as* comes from the evaluator SECL compiled: six
// shapes, each an ordinary evaluator over a closure that already applies the scoping,
// inheritance and TTL.
//
// Declaring the type rather than leaving it dynamic is what makes a variable comparison
// checked: `${some_string} == 1` is a compile error here, where SECL accepts it and
// answers false.
func declareVariables(store *eval.VariableStore) (declaredVariables, []VariableFailure) {
	declared := declaredVariables{
		table: &Variables{readers: map[string]func(*eval.Context) ref.Val{}},
		lists: map[string]bool{},
	}
	if store == nil {
		return declared, nil
	}

	var failures []VariableFailure
	for _, name := range sortedVariableNames(store.Variables) {
		celType, read, err := variableShape(store.Variables[name].GetEvaluator())
		if err != nil {
			failures = append(failures, VariableFailure{Name: name, Err: err})
			continue
		}

		rooted := VariablesRoot + "." + name
		declared.declarations = append(declared.declarations, cel.Variable(rooted, celType))
		declared.table.readers[rooted] = read
		if celType.Kind() == types.ListKind {
			declared.lists[rooted] = true
		}
	}
	return declared, failures
}

// variableShape gives the CEL type and the reader for one of the six shapes a SECL
// variable's evaluator can take (variables.go: GetEvaluator).
func variableShape(evaluator any) (*cel.Type, func(*eval.Context) ref.Val, error) {
	switch evaluator := evaluator.(type) {
	case *eval.StringEvaluator:
		return cel.StringType, func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).(string)
			return types.String(value)
		}, nil

	case *eval.IntEvaluator:
		return cel.IntType, func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).(int)
			return types.Int(value)
		}, nil

	case *eval.BoolEvaluator:
		return cel.BoolType, func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).(bool)
			return types.Bool(value)
		}, nil

	case *eval.CIDREvaluator:
		return ext.CIDRType, func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).(net.IPNet)
			return cidrToVal(value)
		}, nil

	case *eval.StringArrayEvaluator:
		return cel.ListType(cel.StringType), func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).([]string)
			return stringsToVal(value)
		}, nil

	case *eval.IntArrayEvaluator:
		return cel.ListType(cel.IntType), func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).([]int)
			return intsToVal(value)
		}, nil

	case *eval.CIDRArrayEvaluator:
		return cel.ListType(ext.CIDRType), func(ctx *eval.Context) ref.Val {
			value, _ := evaluator.Eval(ctx).([]net.IPNet)
			return cidrsToVal(value)
		}, nil
	}

	return nil, nil, fmt.Errorf("%w: %T is not a shape a variable can be read as", errUnsupportedValue, evaluator)
}

func sortedVariableNames(variables map[string]eval.SECLVariable) []string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
