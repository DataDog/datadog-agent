package eval

type Rule struct {
	Name       string
	Expression string
	evaluator  *RuleEvaluator
}
