// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/pass"
	"github.com/DataDog/datadog-agent/pkg/security/secl/utils"
)

const (
	maxRegisterIteration = 100
)

// RuleID - ID of a Rule
type RuleID = string

// RuleSetTagValue - Value of the "ruleset" tag
type RuleSetTagValue = string

// Rule - Rule object identified by an `ID` containing a SECL `Expression`
type Rule struct {
	ID          RuleID
	Expression  string
	Tags        []string
	Model       Model
	Opts        *Opts
	pprofLabels utils.LabelSet

	evaluator *RuleEvaluator
	ast       *ast.Rule
}

// RuleEvaluator - Evaluation part of a Rule
type RuleEvaluator struct {
	Eval      BoolEvalFnc
	EventType EventType

	// tree is the parsed tree after the compilation passes, in negation normal
	// form and its boolean literals folded. It is projected then compiled for
	// each of the partials of the rule.
	tree *ast.Rule

	fieldValues map[Field][]FieldValue
	fields      []Field
}

// NewRule returns a new rule
func NewRule(id string, expression string, parsingContext *parser.ParsingContext, opts *Opts, tags ...string) (*Rule, error) {
	if opts.MacroStore == nil {
		opts.WithMacroStore(&MacroStore{})
	}
	if opts.VariableStore == nil {
		opts.WithVariableStore(&VariableStore{})
	}

	labelSet, err := utils.NewLabelSet("rule_id", id)
	if err != nil {
		panic(err)
	}

	astRule, err := parsingContext.BuildRule(expression)
	if err != nil {
		return nil, err
	}

	return &Rule{
		ID:          id,
		Expression:  expression,
		Opts:        opts,
		Tags:        tags,
		pprofLabels: labelSet,
		ast:         astRule,
	}, nil
}

// GetFields - Returns all the Field that the RuleEvaluator handles
func (r *RuleEvaluator) GetFields() []Field {
	return r.fields
}

// Eval - Evaluates
func (r *Rule) Eval(ctx *Context) bool {
	return r.evaluator.Eval(ctx)
}

// GetFieldValues returns the values of the given field
func (r *Rule) GetFieldValues(field Field) []FieldValue {
	return r.evaluator.fieldValues[field]
}

// GetFields - Returns all the Field of the Rule including field of the Macro used
func (r *Rule) GetFields() []Field {
	return r.evaluator.GetFields()
}

// UsesAnyField returns whether the Rule uses at least one of the given Fields
func (r *Rule) UsesAnyField(fields []Field) bool {
	return slices.ContainsFunc(fields, func(field Field) bool { return slices.Contains(r.GetFields(), field) })
}

// GetPprofLabels returns the pprof labels
func (r *Rule) GetPprofLabels() utils.LabelSet {
	return r.pprofLabels
}

// GetEvaluator - Returns the RuleEvaluator of the Rule corresponding to the SECL `Expression`
func (r *Rule) GetEvaluator() *RuleEvaluator {
	return r.evaluator
}

// GetEventType - Returns the event type that the `Expression` handles
func (r *Rule) GetEventType() (EventType, error) {
	if r.evaluator == nil {
		return "", &ErrRuleNotCompiled{RuleID: r.ID}
	}

	return r.evaluator.EventType, nil
}

// GetAst - Returns the representation of the SECL `Expression`
func (r *Rule) GetAst() *ast.Rule {
	return r.ast
}

// GetResolvedAst returns the tree of the rule after the compilation passes, in
// negation normal form and its boolean literals folded, see
// pass.NegationNormalForm and pass.FoldBools.
func (r *Rule) GetResolvedAst() *ast.Rule {
	if r.evaluator == nil {
		return nil
	}
	return r.evaluator.tree
}

// NewRuleEvaluator returns a new evaluator for a parsed rule: it runs the
// compilation passes over the tree and compiles the result.
//
// The tree it keeps is normalized after the compilation, see GetResolvedAst.
// The errors and the fields of the rule are the ones of the tree as written: the
// normalization rewrites the operators and folds away the operands of
// `|| true`, a field of the rule can be missing from the kept tree.
func NewRuleEvaluator(rule *ast.Rule, model Model, opts *Opts) (*RuleEvaluator, error) {
	node, err := pass.Run(rule, opts.passes()...)
	if err != nil {
		return nil, err
	}
	rule = node.(*ast.Rule)
	state := NewState(model)

	eval, _, err := compileExpr(rule.Root, opts, state)
	if err != nil {
		return nil, err
	}

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
	}

	eventType, err := EventTypeFromState(model, state)
	if err != nil {
		return nil, err
	}

	// direct value, no bool evaluator, wrap value
	if evalBool.EvalFnc == nil {
		evalBool.EvalFnc = func(_ *Context) bool {
			return evalBool.Value
		}
	}

	// handle rule with iterator registers
	if len(state.registers) > 0 {
		// NOTE: limit to only one register for now to avoid computation and evaluation
		// of all the combination
		if len(state.registers) > 1 {
			return nil, &ErrIteratorVariable{Err: errors.New("iterator variable limit to one per rule")}
		}

		regID, field := state.registers[0].ID, state.registers[0].Field
		lenEval, err := model.GetEvaluator(field+".length", regID, 0)
		if err != nil {
			return nil, &ErrIteratorVariable{Err: err}
		}

		evalBoolFnc := evalBool.EvalFnc

		// eval with each possible value of the registers
		evalBool.EvalFnc = func(ctx *Context) bool {
			size := min(lenEval.Eval(ctx).(int), maxRegisterIteration)

			for i := 0; i != size; i++ {
				ctx.Registers[regID] = i
				if evalBoolFnc(ctx) {
					// invalidate the cache
					clear(ctx.RegisterCache)

					return true
				}

				// invalidate the cache
				clear(ctx.RegisterCache)
			}

			return false
		}
	}

	normalized, err := pass.Run(rule, pass.NegationNormalForm(), pass.FoldBools())
	if err != nil {
		return nil, err
	}

	return &RuleEvaluator{
		Eval:        evalBool.EvalFnc,
		EventType:   eventType,
		tree:        normalized.(*ast.Rule),
		fieldValues: state.fieldValues,
		fields:      KeysOfMap(state.fieldValues),
	}, nil
}

// GenEvaluator - Compile and generates the RuleEvaluator
func (r *Rule) GenEvaluator(model Model) error {
	r.Model = model

	evaluator, err := NewRuleEvaluator(r.ast, model, r.Opts)
	if err != nil {
		if err, ok := err.(*ErrAstToEval); ok && err.Pos.Source == "" {
			return fmt.Errorf("rule syntax error: %s: %w", err, &ErrRuleParse{pos: err.Pos, expr: r.Expression})
		}
		return fmt.Errorf("rule compilation error: %w", err)
	}
	r.evaluator = evaluator

	return nil
}

// CompilePartial compiles the partial of the rule over a set of fields, see
// pass.ProjectFields. A partial that doesn't depend on the event, like the one
// of a rule using none of the fields, is returned as its value with a nil
// evaluator.
func (r *Rule) CompilePartial(fields []Field) (BoolEvalFnc, bool, error) {
	node, err := pass.Run(r.evaluator.tree, pass.ProjectFields(fields), pass.FoldBools())
	if err != nil {
		return nil, false, fmt.Errorf("couldn't generate partial for fields %v and rule %s: %w", fields, r.ID, err)
	}
	projected := node.(*ast.Rule)

	state := NewState(r.Model)
	pEval, _, err := compileExpr(projected.Root, r.Opts, state)
	if err != nil {
		return nil, false, fmt.Errorf("couldn't generate partial for fields %v and rule %s: %w", fields, r.ID, err)
	}

	pEvalBool, ok := pEval.(*BoolEvaluator)
	if !ok {
		return nil, false, NewTypeError(projected.Pos, reflect.Bool)
	}

	return pEvalBool.EvalFnc, pEvalBool.Value, nil
}
