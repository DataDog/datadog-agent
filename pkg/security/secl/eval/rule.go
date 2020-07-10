package eval

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/pkg/errors"
)

// Rule - rule object identified by an `ID` containing a SECL `Expression`
type Rule struct {
	ID         RuleID
	Expression string
	Tags       []string

	evaluator *RuleEvaluator
	ast       *ast.Rule
}

// GetEvaluator returns the RuleEvaluator of the Rule corresponding to the SECL `Expression`
func (r *Rule) GetEvaluator() *RuleEvaluator {
	return r.evaluator
}

// GetEventTypes returns a list of all the event that the `Expression` handles
func (r *Rule) GetEventTypes() []EventType {
	return r.evaluator.EventTypes
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

	fmt.Printf("SSSSSSSSSSSSSSSSSSS: %+v\n", state)

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
	}

	events, err := eventFromFields(model, state)
	if err != nil {
		return nil, err
	}

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

// GenPartials - to be removed, shouldn't be used
func (r *Rule) GenPartials(model Model, opts *Opts) error {
	// Only generate partials if they have not been generated yet
	if r.evaluator != nil && r.evaluator.partialEvals != nil {
		return nil
	}
	{
	}
	// map field with partial macro evaluators
	macroEvaluators := make(map[Field]map[MacroID]*MacroEvaluator)
	for id, macro := range opts.Macros {
		for field, eval := range macro.partials {
			if _, exists := macroEvaluators[field]; !exists {
				macroEvaluators[field] = make(map[string]*MacroEvaluator)
			}
			macroEvaluators[field][id] = eval
		}
	}

	fmt.Printf("EEEE: %+v\n", macroEvaluators)

	// Only generate partials for the fields of the rule
	for _, field := range r.evaluator.GetFields() {
		state := newState(model, field, macroEvaluators[field])
		pEval, _, _, err := nodeToEvaluator(r.ast.BooleanExpression, opts, state)
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
