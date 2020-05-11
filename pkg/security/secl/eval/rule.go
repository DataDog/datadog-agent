package eval

type Rule struct {
	ID         string
	Expression string
	evaluator  *RuleEvaluator
}
