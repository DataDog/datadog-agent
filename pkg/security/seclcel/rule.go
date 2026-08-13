// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/interpreter"
)

// Rule is a SECL rule planned for evaluation against an event.
//
// It holds the interpretable the planner produced rather than the cel.Program
// that would wrap it, and is evaluated by handing that interpretable the
// execution frame an Activation carries. That is the whole difference from
// env.Program, and it is worth 60 to 90 ns of every evaluation — see the note on
// planRule.
type Rule struct {
	expr   string
	fields []string
	plan   interpreter.InterpretableV2
}

// NewRule translates a SECL expression, type-checks it against env, resolves its
// field reads and plans it for evaluation.
//
// Pair env with the field types it was built from — NewModelEnv and
// ModelFieldTypes against the real model — so that the translation and the check
// agree about which fields hold several values.
func NewRule(env *cel.Env, expr string, fieldTypes FieldTypes) (*Rule, error) {
	checked, err := CompileWithTypes(env, expr, fieldTypes)
	if err != nil {
		return nil, err
	}

	optimized, fields, err := Optimize(env, checked)
	if err != nil {
		return nil, fmt.Errorf("optimizing %q: %w", expr, err)
	}

	plan, err := planRule(env, optimized)
	if err != nil {
		return nil, fmt.Errorf("planning %q: %w", expr, err)
	}
	return &Rule{expr: expr, fields: fields, plan: plan}, nil
}

// Expr returns the SECL expression the rule was built from.
func (r *Rule) Expr() string { return r.expr }

// Fields returns the SECL fields the rule reads, which the optimization pass knows
// because resolving them is what it does.
//
// It is the counterpart of SECL's RuleEvaluator.GetFields: what a rule reads is
// what decides which bucket it belongs to and which approvers it can support. An
// iterated node counts as a field of its own, since reading it is a read.
func (r *Rule) Fields() []string { return r.fields }

// Eval evaluates the rule against the event the activation's context holds.
//
// A panic raised below this point is not recovered, where cel.Program.Eval turns
// one into an error: a reader that panics is a bug in the generated code rather
// than a rule that cannot be evaluated, and SECL's own Rule.Eval does not recover
// either. The deferred recover is part of what the wrapper costs.
func (r *Rule) Eval(a *Activation) (bool, error) {
	switch out := r.plan.Exec(a.frame).(type) {
	case types.Bool:
		return bool(out), nil
	case *types.Err:
		return false, fmt.Errorf("evaluating %q: %w", r.expr, out)
	default:
		// A rule is a boolean expression, so anything else is a rule that should not
		// have been planned or an unknown from a partial evaluation, which this path
		// does not offer — see planRule.
		return false, fmt.Errorf("evaluating %q: %w: %s is not a boolean", r.expr, errUnsupportedValue, out.Type())
	}
}

// planRule plans a checked expression the way cel.Env.Program does, stopping at
// the interpretable rather than wrapping it in a cel.Program.
//
// The wrapper charges for itself on every call — a deferred recover, a
// types.IsError test, a three value return, and above all taking an
// ExecutionFrame from the pool and returning it. Measured over the shapes in
// bench_test.go that is 43 to 57 ns for the frame and a further 16 to 31 ns for
// the rest, which is over a quarter of what a rule in a bucket costs: both are
// per Eval, and a bucket of r rules pays them r times per event. Planning the
// interpretable here lets an Activation hold one frame for as long as its
// context and hand it to Exec directly.
//
// This reproduces newProgram (cel/program.go) for the options the rules use: a
// dispatcher holding the bindings of every declared function, the environment's
// own adapter and provider, and the planner options below. It can do that
// because NewEnv contributes no program options of its own — ext.Math and
// ext.Network both return none — so there is nothing an env.Program would have
// added that is dropped here. TestRuleAgreesWithCELProgram is what catches it if
// a cel-go bump changes that.
//
// What newProgram does that this does not is reach the features no rule uses
// today. None of them is out of reach, since they are all plan time choices that
// belong to whoever plans a rule, but each is a second interpretable rather than
// a flag flipped at evaluation:
//
//   - Partial evaluation, which CWS uses for approvers and discarders, is
//     interpreter.NewPartialAttributeFactory in place of the factory below, a
//     cel.PartialVars activation, and types.Unknown handled in Rule.Eval. Planning
//     it separately is what keeps the rules that do not ask for it from paying the
//     partial factory's unknown check on every attribute they resolve.
//   - State tracking, which is how the matching subexpressions SECL reports would
//     be recovered, is interpreter.EvalStateObserver() here and
//     ObservableInterpretable.ObserveExec in place of Exec. It works on a frame
//     that outlives the evaluation: InitState takes an evalContext from the pool
//     when the frame has none.
//   - Cost tracking is interpreter.CostObserver, the same shape. Estimating a
//     rule's cost before it runs is a checker concern and is unaffected either way.
//   - Interrupting an evaluation is the one that costs something:
//     InterruptableEval() plans the checks, but they only fire once
//     ExecutionFrame.SetContext has been called, which is per evaluation work a
//     frame held across events does not do. Nothing in SECL cancels an evaluation.
func planRule(env *cel.Env, checked *cel.Ast) (interpreter.InterpretableV2, error) {
	dispatcher := interpreter.NewDispatcher()
	for _, function := range env.Functions() {
		bindings, err := function.Bindings()
		if err != nil {
			return nil, err
		}
		if err := dispatcher.Add(bindings...); err != nil {
			return nil, err
		}
	}

	adapter, provider := env.CELTypeAdapter(), env.CELTypeProvider()
	interp := interpreter.NewInterpreter(dispatcher, env.Container, provider, adapter,
		interpreter.NewAttributeFactory(env.Container, adapter, provider))

	return interp.NewInterpretable(checked.NativeRep(), plannerOptions()...)
}

// plannerOptions are what a rule is planned with, and are all about doing at
// planning time what would otherwise be repeated for every event.
//
// A rule is planned once and evaluated for the lifetime of the agent, so the
// asymmetry is extreme: anything derived from a literal in the rule text is
// worth deriving once. SECL does the same when it compiles a rule, which is why
// its pattern and regexp comparisons cost nothing at match time.
//
// They are the pair newProgram derives from cel.OptOptimize and cel.OptimizeRegex,
// in the order it applies them (cel/program.go): fold constants first, so that
// the regex compilation below sees the folded literals.
func plannerOptions() []interpreter.PlannerOption {
	return []interpreter.PlannerOption{
		// Fold constant subexpressions, which is what turns a list literal into a
		// set membership test rather than a list rebuilt on every event.
		interpreter.Optimize(),
		// Compile the pattern of a glob or a matches() against a literal. Without
		// it a `r"…"` rule recompiles its regexp on every event.
		interpreter.CompileRegexConstants(globOptimization, pathGlobOptimization,
			interpreter.MatchesRegexOptimization),
		// Bind each field read to its reader, which is the same trick for the index
		// the optimization pass emitted. readDecorator is readOptimization in the
		// form a caller that plans the interpretable itself has to pass it.
		interpreter.CustomDecoratorV2(readDecorator()),
		// The same for a comparison against a path field's variants, whose reader is
		// named rather than indexed because the translation emits it before the reads
		// are resolved.
		interpreter.CustomDecoratorV2(pathMatchDecorator()),
		// Collapse an expression over literals — the flags idiom, a list of patterns —
		// into the one value it always evaluates to.
		interpreter.CustomDecoratorV2(constantFolding()),
		// Prepare the right hand side of a membership test, which the folding above has
		// just turned into a value wherever the rule named one.
		interpreter.CustomDecoratorV2(matchAnyDecorator()),
	}
}
