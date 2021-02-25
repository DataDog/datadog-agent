// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"reflect"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

// RuleID - ID of a Rule
type RuleID = string

// Rule - Rule object identified by an `ID` containing a SECL `Expression`
type Rule struct {
	ID         RuleID
	Expression string
	Tags       []string
	Opts       *Opts
	Model      Model

	evaluator *RuleEvaluator
	ast       *ast.Rule
}

// RuleEvaluator - Evaluation part of a Rule
type RuleEvaluator struct {
	Eval        BoolEvalFnc
	EventTypes  []EventType
	FieldValues map[Field][]FieldValue

	partialEvals map[Field]BoolEvalFnc
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
	fields := make([]Field, len(r.FieldValues))
	i := 0
	for key := range r.FieldValues {
		fields[i] = key
		i++
	}
	return fields
}

// Eval - Evaluates
func (r *Rule) Eval(ctx *Context) bool {
	return r.evaluator.Eval(ctx)
}

// GetFieldValues returns the values of the given field
func (r *Rule) GetFieldValues(field Field) []FieldValue {
	return r.evaluator.FieldValues[field]
}

// PartialEval - Partial evaluation with the given Field
func (r *Rule) PartialEval(ctx *Context, field Field) (bool, error) {
	return r.evaluator.PartialEval(ctx, field)
}

// GetPartialEval - Returns the Partial RuleEvaluator for the given Field
func (r *Rule) GetPartialEval(field Field) BoolEvalFnc {
	return r.evaluator.partialEvals[field]
}

// GetFields - Returns all the Field of the Rule including field of the Macro used
func (r *Rule) GetFields() []Field {
	fields := r.evaluator.GetFields()

	for _, macro := range r.Opts.Macros {
		fields = append(fields, macro.GetFields()...)
	}

	return fields
}

// GetEvaluator - Returns the RuleEvaluator of the Rule corresponding to the SECL `Expression`
func (r *Rule) GetEvaluator() *RuleEvaluator {
	return r.evaluator
}

// GetEventTypes - Returns a list of all the event that the `Expression` handles
func (r *Rule) GetEventTypes() []EventType {
	eventTypes := r.evaluator.EventTypes

	for _, macro := range r.Opts.Macros {
		eventTypes = append(eventTypes, macro.GetEventTypes()...)
	}

	return eventTypes
}

// GetAst - Returns the representation of the SECL `Expression`
func (r *Rule) GetAst() *ast.Rule {
	return r.ast
}

// Parse - Transforms the SECL `Expression` into its AST representation
func (r *Rule) Parse() error {
	astRule, err := ast.ParseRule(r.Expression)
	if err != nil {
		return err
	}
	r.ast = astRule
	return nil
}

func combineRegisters(combinations []Registers, regID RegisterID, values []unsafe.Pointer) []Registers {
	var combined []Registers

	if len(combinations) == 0 {
		for _, value := range values {
			registers := make(Registers)
			registers[regID] = &Register{
				Value: value,
			}
			combined = append(combined, registers)
		}

		return combined
	}

	for _, combination := range combinations {
		for _, value := range values {
			regs := combination.Clone()
			regs[regID] = &Register{
				Value: value,
			}
			combined = append(combined, regs)
		}
	}

	return combined
}

func handleRegisters(evalFnc BoolEvalFnc, registersInfo map[RegisterID]*registerInfo) BoolEvalFnc {
	return func(ctx *Context) bool {
		ctx.Registers = make(Registers)

		// start with the head of all register
		for id, info := range registersInfo {
			ctx.Registers[id] = &Register{
				Value:    info.iterator.Front(ctx),
				iterator: info.iterator,
			}
		}

		// capture all the values for each register
		registerValues := make(map[RegisterID][]unsafe.Pointer)

		for id, reg := range ctx.Registers {
			values := []unsafe.Pointer{}
			for reg.Value != nil {
				// short cut if we find a solution while constructing the combinations
				if evalFnc(ctx) {
					return true
				}
				values = append(values, reg.Value)

				reg.Value = reg.iterator.Next()
			}
			registerValues[id] = values

			// restore the head value
			reg.Value = reg.iterator.Front(ctx)
		}

		// no need to combine there is only one registers used
		if len(registersInfo) == 1 {
			return false
		}

		// generate all the combinations
		var combined []Registers
		for id, values := range registerValues {
			combined = combineRegisters(combined, id, values)
		}

		// eval the combinations
		for _, registers := range combined {
			ctx.Registers = registers
			if evalFnc(ctx) {
				return true
			}
		}

		return false
	}
}

func ruleToEvaluator(rule *ast.Rule, model Model, opts *Opts) (*RuleEvaluator, error) {
	macros := make(map[MacroID]*MacroEvaluator)
	for id, macro := range opts.Macros {
		macros[id] = macro.evaluator
	}
	state := newState(model, "", macros)

	eval, _, _, err := nodeToEvaluator(rule.BooleanExpression, opts, state)
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

	// rule uses register replace the original eval function with the one handling registers
	if len(state.registersInfo) > 0 {
		evalBool.EvalFnc = handleRegisters(evalBool.EvalFnc, state.registersInfo)
	}

	return &RuleEvaluator{
		Eval:        evalBool.EvalFnc,
		EventTypes:  events,
		FieldValues: state.fieldValues,
	}, nil
}

// GenEvaluator - Compile and generates the RuleEvaluator
func (r *Rule) GenEvaluator(model Model, opts *Opts) error {
	r.Model = model
	r.Opts = opts

	evaluator, err := ruleToEvaluator(r.ast, model, opts)
	if err != nil {
		if err, ok := err.(*ErrAstToEval); ok {
			return errors.Wrapf(&ErrRuleParse{pos: err.Pos, expr: r.Expression}, "rule syntax error: %s", err)
		}
		return errors.Wrap(err, "rule compilation error")
	}
	r.evaluator = evaluator

	return nil
}

func (r *Rule) genMacroPartials() (map[Field]map[MacroID]*MacroEvaluator, error) {
	partials := make(map[Field]map[MacroID]*MacroEvaluator)
	for _, field := range r.GetFields() {
		for id, macro := range r.Opts.Macros {

			// NOTE(safchain) this is not working with nested macro. It will be removed once partial
			// will be generated another way
			evaluator, err := macroToEvaluator(macro.ast, r.Model, r.Opts, field)
			if err != nil {
				if err, ok := err.(*ErrAstToEval); ok {
					return nil, errors.Wrap(&ErrRuleParse{pos: err.Pos, expr: macro.Expression}, "macro syntax error")
				}
				return nil, errors.Wrap(err, "macro compilation error")
			}
			macroEvaluators, exists := partials[field]
			if !exists {
				macroEvaluators = make(map[MacroID]*MacroEvaluator)
				partials[field] = macroEvaluators
			}
			macroEvaluators[id] = evaluator
		}
	}

	return partials, nil
}

// GenPartials - Compiles and generates partial Evaluators
func (r *Rule) GenPartials() error {
	macroPartials, err := r.genMacroPartials()
	if err != nil {
		return err
	}

	for _, field := range r.GetFields() {
		state := newState(r.Model, field, macroPartials[field])
		pEval, _, _, err := nodeToEvaluator(r.ast.BooleanExpression, r.Opts, state)
		if err != nil {
			return errors.Wrapf(err, "couldn't generate partial for field %s and rule %s", field, r.ID)
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

		// rule uses register replace the original eval function with the one handling registers
		if len(state.registersInfo) > 0 {
			// generate register map for the given field only
			registersInfo := make(map[RegisterID]*registerInfo)
			for regID, info := range state.registersInfo {
				if _, exists := info.subFields[field]; exists {
					registersInfo[regID] = info
				}
			}

			pEvalBool.EvalFnc = handleRegisters(pEvalBool.EvalFnc, registersInfo)
		}

		r.evaluator.setPartial(field, pEvalBool.EvalFnc)
	}

	return nil
}
