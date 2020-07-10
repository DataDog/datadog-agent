package eval

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/pkg/errors"
)

// Rule - rule object identified by an `ID` containing a SECL `Expression`
type Rule struct {
	ID         RuleID
	Expression string
	Tags       []string
	Opts       *Opts
	Model      Model

	evaluator *RuleEvaluator
	ast       *ast.Rule
}

type RuleEvaluator struct {
	Eval        func(ctx *Context) bool
	EventTypes  []EventType
	Tags        []string
	FieldValues map[Field][]FieldValue

	partialEvals map[Field]func(ctx *Context) bool
}

func (r *RuleEvaluator) PartialEval(ctx *Context, field Field) (bool, error) {
	eval, ok := r.partialEvals[field]
	if !ok {
		return false, errors.New("field not found")
	}

	return eval(ctx), nil
}

func (r *RuleEvaluator) setPartial(field string, partialEval func(ctx *Context) bool) {
	if r.partialEvals == nil {
		r.partialEvals = make(map[string]func(ctx *Context) bool)
	}
	r.partialEvals[field] = partialEval
}

func (r *RuleEvaluator) GetFields() []Field {
	fields := make([]Field, len(r.FieldValues))
	i := 0
	for key := range r.FieldValues {
		fields[i] = key
		i++
	}
	return fields
}

func (r *Rule) Eval(ctx *Context) bool {
	return r.evaluator.Eval(ctx)
}

func (r *Rule) PartialEval(ctx *Context, field Field) (bool, error) {
	return r.evaluator.PartialEval(ctx, field)
}

func (r *Rule) GetPartialEval(field Field) func(ctx *Context) bool {
	return r.evaluator.partialEvals[field]
}

func (r *Rule) GetFields() []Field {
	fields := r.evaluator.GetFields()

	for _, macro := range r.Opts.Macros {
		fields = append(fields, macro.GetFields()...)
	}

	return fields
}

// GetEvaluator returns the RuleEvaluator of the Rule corresponding to the SECL `Expression`
func (r *Rule) GetEvaluator() *RuleEvaluator {
	return r.evaluator
}

// GetEventTypes returns a list of all the event that the `Expression` handles
func (r *Rule) GetEventTypes() []EventType {
	eventTypes := r.evaluator.EventTypes

	for _, macro := range r.Opts.Macros {
		eventTypes = append(eventTypes, macro.GetEventTypes()...)
	}

	return eventTypes
}

// GetAst returns the representation of the SECL `Expression`
func (r *Rule) GetAst() *ast.Rule {
	return r.ast
}

// Parse transforms the SECL `Expression` into its AST representation
func (r *Rule) Parse() error {
	astRule, err := ast.ParseRule(r.Expression)
	if err != nil {
		return err
	}
	r.ast = astRule
	return nil
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

	events, err := eventFromFields(model, state)
	if err != nil {
		return nil, err
	}

	// case where the rule is just a value and not a expression
	if evalBool.EvalFnc == nil {
		return &RuleEvaluator{
			Eval: func(ctx *Context) bool {
				return evalBool.Value
			},
			EventTypes:  events,
			Tags:        state.Tags(),
			FieldValues: state.fieldValues,
		}, nil
	}

	return &RuleEvaluator{
		Eval:        evalBool.EvalFnc,
		EventTypes:  events,
		Tags:        state.Tags(),
		FieldValues: state.fieldValues,
	}, nil
}

func (r *Rule) GenEvaluator(model Model, opts *Opts) error {
	r.Model = model
	r.Opts = opts

	evaluator, err := ruleToEvaluator(r.ast, model, opts)
	if err != nil {
		if err, ok := err.(*AstToEvalError); ok {
			return errors.Wrap(&RuleParseError{pos: err.Pos, expr: r.Expression}, "rule syntax error")
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
				if err, ok := err.(*AstToEvalError); ok {
					return nil, errors.Wrap(&RuleParseError{pos: err.Pos, expr: macro.Expression}, "macro syntax error")
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

// GenPartials - to be removed, shouldn't be used
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

		r.evaluator.setPartial(field, pEvalBool.EvalFnc)
	}

	return nil
}
