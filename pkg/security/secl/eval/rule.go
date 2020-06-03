package eval

type Rule struct {
	ID         string
	Expression string
	Tags       []string

	evaluator *RuleEvaluator
}

func (r *Rule) GetEventTypes() []string {
	return r.evaluator.EventTypes
}

func (r *Rule) SetPartial(field string, partialEval func(ctx *Context) bool) {
	r.evaluator.SetPartial(field, partialEval)
}
