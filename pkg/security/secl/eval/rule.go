package eval

type Rule struct {
	ID         string
	Expression string
	Events     []string
	Tags       []string
	evaluator  *RuleEvaluator
}
