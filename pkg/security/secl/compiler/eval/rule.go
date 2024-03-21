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
	"github.com/DataDog/datadog-agent/pkg/security/secl/utils"
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
	Eval       BoolEvalFnc
	EventTypes []EventType

	fieldValues map[Field][]FieldValue
	fields      []Field

	partialEvals map[Field]BoolEvalFnc
}

// NewRule returns a new rule
func NewRule(id string, expression string, opts *Opts, tags ...string) *Rule {
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

	return &Rule{
		ID:          id,
		Expression:  expression,
		Opts:        opts,
		Tags:        tags,
		pprofLabels: labelSet,
	}
}

// PartialEval partially evaluation of the Rule with the given Field.
func (r *RuleEvaluator) PartialEval(ctx *Context, field Field) (bool, error) {
	eval, ok := r.partialEvals[field]
	if !ok {
		return false, &ErrFieldNotFound{Field: field}
	}

	return eval(ctx), nil
}

func (r *RuleEvaluator) setPartial(field string, partialEval BoolEvalFnc) {
	if r.partialEvals == nil {
		r.partialEvals = make(map[string]BoolEvalFnc)
	}
	r.partialEvals[field] = partialEval
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

// PartialEval - Partial evaluation with the given Field
func (r *Rule) PartialEval(ctx *Context, field Field) (bool, error) {
	result, err := r.evaluator.PartialEval(ctx, field)
	if err == nil {
		return result, nil
	}

	var errNotFound *ErrFieldNotFound
	if errors.As(err, &errNotFound) {
		if err = r.genPartials(field); err != nil {
			return false, err
		}
		result, err = r.evaluator.PartialEval(ctx, field)
	}
	return result, err
}

// GetPartialEval - Returns the Partial RuleEvaluator for the given Field
func (r *Rule) GetPartialEval(field Field) BoolEvalFnc {
	partial, exists := r.evaluator.partialEvals[field]
	if !exists {
		if err := r.genPartials(field); err != nil {
			return nil
		}
		partial = r.evaluator.partialEvals[field]
	}

	return partial
}

// GetFields - Returns all the Field of the Rule including field of the Macro used
func (r *Rule) GetFields() []Field {
	fields := r.evaluator.GetFields()

	for _, macro := range r.Opts.MacroStore.List() {
		fields = append(fields, macro.GetFields()...)
	}

	return fields
}

// GetPprofLabels returns the pprof labels
func (r *Rule) GetPprofLabels() utils.LabelSet {
	return r.pprofLabels
}

// GetEvaluator - Returns the RuleEvaluator of the Rule corresponding to the SECL `Expression`
func (r *Rule) GetEvaluator() *RuleEvaluator {
	return r.evaluator
}

// GetEventTypes - Returns a list of all the event that the `Expression` handles
func (r *Rule) GetEventTypes() ([]EventType, error) {
	if r.evaluator == nil {
		return nil, &ErrRuleNotCompiled{RuleID: r.ID}
	}

	eventTypes := r.evaluator.EventTypes

	for _, macro := range r.Opts.MacroStore.List() {
		eventTypes = append(eventTypes, macro.GetEventTypes()...)
	}

	return eventTypes, nil
}

// GetAst - Returns the representation of the SECL `Expression`
func (r *Rule) GetAst() *ast.Rule {
	return r.ast
}

// Parse - Transforms the SECL `Expression` into its AST representation
func (r *Rule) Parse(parsingContext *ast.ParsingContext) error {
	astRule, err := parsingContext.ParseRule(r.Expression)
	if err != nil {
		return err
	}
	r.ast = astRule
	return nil
}

// NewRuleEvaluator returns a new evaluator for a rule
func NewRuleEvaluator(rule *ast.Rule, model Model, opts *Opts) (*RuleEvaluator, error) {
	macros := make(map[MacroID]*MacroEvaluator)
	for _, macro := range opts.MacroStore.List() {
		macros[macro.ID] = macro.evaluator
	}
	state := NewState(model, "", macros)

	eval, _, err := nodeToEvaluator(rule.BooleanExpression, opts, state)
	if err != nil {
		return nil, err
	}

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
	}

	events, err := eventTypesFromFields(model, state)
	if err != nil {
		return nil, err
	}

	// direct value, no bool evaluator, wrap value
	if evalBool.EvalFnc == nil {
		evalBool.EvalFnc = func(ctx *Context) bool {
			return evalBool.Value
		}
	}

	return &RuleEvaluator{
		Eval:        evalBool.EvalFnc,
		EventTypes:  events,
		fieldValues: state.fieldValues,
		fields:      KeysOfMap(state.fieldValues),
	}, nil
}

// GenEvaluator - Compile and generates the RuleEvaluator
func (r *Rule) GenEvaluator(model Model, parsingCtx *ast.ParsingContext) error {
	r.Model = model

	if r.ast == nil {
		if err := r.Parse(parsingCtx); err != nil {
			return err
		}
	}

	evaluator, err := NewRuleEvaluator(r.ast, model, r.Opts)
	if err != nil {
		if err, ok := err.(*ErrAstToEval); ok {
			return fmt.Errorf("rule syntax error: %s: %w", err, &ErrRuleParse{pos: err.Pos, expr: r.Expression})
		}
		return fmt.Errorf("rule compilation error: %w", err)
	}
	r.evaluator = evaluator

	return nil
}

func (r *Rule) genMacroPartials(field Field) (map[MacroID]*MacroEvaluator, error) {
	// check that field in this rule fields
	if !slices.Contains(r.GetFields(), field) {
		return nil, nil
	}

	macroEvaluators := make(map[MacroID]*MacroEvaluator)
	for _, macro := range r.Opts.MacroStore.List() {
		var err error
		var evaluator *MacroEvaluator
		if macro.ast != nil {
			// NOTE(safchain) this is not working with nested macro. It will be removed once partial
			// will be generated another way
			evaluator, err = macroToEvaluator(macro.ast, r.Model, r.Opts, field)
			if err != nil {
				if err, ok := err.(*ErrAstToEval); ok {
					return nil, fmt.Errorf("macro syntax error: %w", &ErrRuleParse{pos: err.Pos})
				}
				return nil, fmt.Errorf("macro compilation error: %w", err)
			}
		} else {
			evaluator = macro.GetEvaluator()
		}

		macroEvaluators[macro.ID] = evaluator
	}

	return macroEvaluators, nil
}

// GenPartials - Compiles and generates partial Evaluators
func (r *Rule) genPartials(field Field) error {
	macroPartial, err := r.genMacroPartials(field)
	if err != nil {
		return err
	}

	if !slices.Contains(r.GetFields(), field) {
		return nil
	}

	state := NewState(r.Model, field, macroPartial)
	pEval, _, err := nodeToEvaluator(r.ast.BooleanExpression, r.Opts, state)
	if err != nil {
		return fmt.Errorf("couldn't generate partial for field %s and rule %s: %w", field, r.ID, err)
	}

	pEvalBool, ok := pEval.(*BoolEvaluator)
	if !ok {
		return NewTypeError(r.ast.Pos, reflect.Bool)
	}

	if pEvalBool.EvalFnc == nil {
		pEvalBool.EvalFnc = func(ctx *Context) bool {
			return pEvalBool.Value
		}
	}

	r.evaluator.setPartial(field, pEvalBool.EvalFnc)

	return nil
}
