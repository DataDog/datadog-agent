package eval

import "github.com/DataDog/datadog-agent/pkg/security/secl/ast"

type Rule struct {
	ID         string
	Expression string
	Tags       []string

	evaluator *RuleEvaluator
	ast       *ast.Rule
}

func (r *Rule) GetEventTypes() []string {
	return r.evaluator.EventTypes
}

func (r *Rule) SetPartial(field string, partialEval func(ctx *Context) bool) {
	r.evaluator.SetPartial(field, partialEval)
}

func (r *Rule) Parse() error {
	astRule, err := ast.ParseRule(r.Expression)
	if err != nil {
		return err
	}
	r.ast = astRule
	return nil
}
