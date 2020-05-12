package eval

type Rule struct {
	ID         string
	Expression string
	Tags       []string
	evaluator  *RuleEvaluator
}
