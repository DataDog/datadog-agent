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
	"strings"

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

// PartialEvalState describes the compiled state of a rule's partial evaluator for a field set.
type PartialEvalState uint8

const (
	// PartialEvalMissing indicates the rule does not reference any of the target fields.
	PartialEvalMissing PartialEvalState = iota
	// PartialEvalConstTrue indicates the projected rule folds to constant true.
	PartialEvalConstTrue
	// PartialEvalConstFalse indicates the projected rule folds to constant false.
	PartialEvalConstFalse
	// PartialEvalDynamic indicates the projected rule requires evaluating a dynamic BoolEvalFnc.
	PartialEvalDynamic
)

type partialEvalEntry struct {
	state   PartialEvalState
	eval    BoolEvalFnc
	missing error
}

// RuleEvaluator - Evaluation part of a Rule
type RuleEvaluator struct {
	Eval      BoolEvalFnc
	EventType EventType

	// tree is the parsed tree after the compilation passes. It is compiled for the
	// rule, and projected then compiled for each of its partials.
	tree *ast.Rule

	fieldValues map[Field][]FieldValue
	fields      []Field

	partialEvals map[string]partialEvalEntry
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

// partialKey identifies the partial of a set of fields.
// PartialKey returns the lookup key for a partial evaluator over fields.
func PartialKey(fields []Field) string {
	return partialKey(fields)
}

func partialKey(fields []Field) string {
	if len(fields) == 1 {
		return fields[0]
	}
	return strings.Join(fields, " ")
}

// IsPartialAvailable checks if partial have been generated for the given Fields
func (r *RuleEvaluator) IsPartialAvailable(fields ...Field) bool {
	entry, exists := r.partialEvals[partialKey(fields)]
	return exists && entry.state != PartialEvalMissing
}

// PartialEval partially evaluation of the Rule with the given Fields.
func (r *RuleEvaluator) PartialEval(ctx *Context, fields ...Field) (bool, error) {
	return r.partialEval(ctx, fields, partialKey(fields))
}

func (r *RuleEvaluator) partialEval(ctx *Context, fields []Field, key string) (bool, error) {
	entry, ok := r.partialEvals[key]
	if !ok || entry.state == PartialEvalMissing {
		if entry.missing != nil {
			return false, entry.missing
		}
		return false, &ErrFieldNotFound{Field: strings.Join(fields, ", ")}
	}

	return entry.eval(ctx), nil
}

func (r *RuleEvaluator) setPartialEntry(key string, entry partialEvalEntry) {
	if r.partialEvals == nil {
		r.partialEvals = make(map[string]partialEvalEntry)
	}
	r.partialEvals[key] = entry
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

// PartialEval - Partial evaluation with the given Fields
func (r *Rule) PartialEval(ctx *Context, fields ...Field) (bool, error) {
	return r.PartialEvalWithKey(ctx, fields, partialKey(fields))
}

func (r *Rule) resolvePartialEntry(fields []Field, key string) (partialEvalEntry, error) {
	entry, exists := r.evaluator.partialEvals[key]
	if !exists {
		if err := r.genPartials(fields, key); err != nil {
			return partialEvalEntry{}, err
		}
		entry = r.evaluator.partialEvals[key]
	}
	return entry, nil
}

// PartialEvalWithKey evaluates the partial evaluator for fields using a precomputed PartialKey.
func (r *Rule) PartialEvalWithKey(ctx *Context, fields []Field, key string) (bool, error) {
	entry, err := r.resolvePartialEntry(fields, key)
	if err != nil {
		return false, err
	}
	switch entry.state {
	case PartialEvalMissing:
		return false, entry.missing
	case PartialEvalConstTrue:
		return true, nil
	case PartialEvalConstFalse:
		return false, nil
	default:
		return entry.eval(ctx), nil
	}
}

// GetPartialEvalState returns the compiled PartialEvalState and evaluator for fields using a precomputed key.
func (r *Rule) GetPartialEvalState(fields []Field, key string) (PartialEvalState, BoolEvalFnc, error) {
	entry, err := r.resolvePartialEntry(fields, key)
	if err != nil {
		return PartialEvalMissing, nil, err
	}
	if entry.state == PartialEvalMissing {
		return PartialEvalMissing, nil, entry.missing
	}
	return entry.state, entry.eval, nil
}

// GetPartialEval - Returns the Partial RuleEvaluator for the given Fields
func (r *Rule) GetPartialEval(fields ...Field) BoolEvalFnc {
	entry, err := r.resolvePartialEntry(fields, partialKey(fields))
	if err != nil {
		return nil
	}
	return entry.eval
}

// GetFields - Returns all the Field of the Rule including field of the Macro used
func (r *Rule) GetFields() []Field {
	return r.evaluator.GetFields()
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

// GetResolvedAst returns the tree of the rule after the compilation passes
func (r *Rule) GetResolvedAst() *ast.Rule {
	if r.evaluator == nil {
		return nil
	}
	return r.evaluator.tree
}

// NewRuleEvaluator returns a new evaluator for a parsed rule: it runs the
// compilation passes over the tree and compiles the result.
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

	return &RuleEvaluator{
		Eval:        evalBool.EvalFnc,
		EventType:   eventType,
		tree:        rule,
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

// genPartials compiles the partial of the rule for a set of fields, see
// pass.ProjectFields. A rule that uses none of the fields has no partial.
func (r *Rule) genPartials(fields []Field, key string) error {
	if !slices.ContainsFunc(fields, func(field Field) bool { return slices.Contains(r.GetFields(), field) }) {
		r.evaluator.setPartialEntry(key, partialEvalEntry{
			state:   PartialEvalMissing,
			missing: &ErrFieldNotFound{Field: strings.Join(fields, ", ")},
		})
		return nil
	}

	node, err := pass.Run(r.evaluator.tree, pass.ProjectFields(fields), pass.FoldBools())
	if err != nil {
		return fmt.Errorf("couldn't generate partial for fields %v and rule %s: %w", fields, r.ID, err)
	}
	projected := node.(*ast.Rule)

	state := NewState(r.Model)
	pEval, _, err := compileExpr(projected.Root, r.Opts, state)
	if err != nil {
		return fmt.Errorf("couldn't generate partial for fields %v and rule %s: %w", fields, r.ID, err)
	}

	pEvalBool, ok := pEval.(*BoolEvaluator)
	if !ok {
		return NewTypeError(projected.Pos, reflect.Bool)
	}

	partialState := PartialEvalDynamic
	if pEvalBool.EvalFnc == nil {
		if pEvalBool.Value {
			partialState = PartialEvalConstTrue
		} else {
			partialState = PartialEvalConstFalse
		}
		pEvalBool.EvalFnc = func(_ *Context) bool {
			return pEvalBool.Value
		}
	}

	r.evaluator.setPartialEntry(key, partialEvalEntry{
		state: partialState,
		eval:  pEvalBool.EvalFnc,
	})

	return nil
}
