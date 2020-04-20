package scenario

import (
	"io"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type Scenario struct {
	Name  string      `yaml:"name"`
	Event model.Event `yaml:"event"`
	Count int
}

func (s *Scenario) Evaluate(r *ast.Rule) (bool, error) {
	evaluator, err := eval.RuleToEvaluator(r, false)
	if err != nil {
		return false, err
	}

	context := &eval.Context{
		Event: &s.Event,
	}

	return evaluator(context), nil
}

func NewScenarioFromFile(filename string) (*Scenario, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	return NewScenario(f)
}

func NewScenario(r io.Reader) (*Scenario, error) {
	scenario := &Scenario{}
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(scenario); err != nil {
		return nil, err
	}

	return scenario, nil
}
