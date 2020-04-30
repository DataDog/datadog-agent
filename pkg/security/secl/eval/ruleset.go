package eval

import (
	"log"
	"sort"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

type Event interface {
	GetType() string
}

type RuleSetListener interface {
	RuleMatch(rule *Rule, event Event)
	DiscriminatorDiscovered(event Event, field string)
}

type Rule struct {
	name      string
	evaluator *RuleEvaluator
}

type RuleBucket struct {
	rules  []*Rule
	fields []string
}

func (rl *RuleBucket) AddRule(rule *Rule) {
	for _, field := range rule.evaluator.GetFields() {
		index := sort.SearchStrings(rl.fields, field)
		if index < len(rl.fields) && rl.fields[index] == field {
			continue
		}
		rl.fields = append(rl.fields, "")
		copy(rl.fields[index+1:], rl.fields[index:])
		rl.fields[index] = field
	}

	rl.rules = append(rl.rules, rule)
}

type RuleSet struct {
	rulesByTag map[string]*RuleBucket
	model      Model
	listeners  []RuleSetListener
	debug      bool
}

func (rs *RuleSet) AddRule(name, expression string) (*Rule, error) {
	astRule, err := ast.ParseRule(expression)
	if err != nil {
		return nil, errors.Wrap(err, "invalid rule")
	}

	evaluator, err := RuleToEvaluator(astRule, nil, rs.model, rs.debug)
	if err != nil {
		return nil, err
	}

	rule := &Rule{
		evaluator: evaluator,
		name:      name,
	}

	for _, tag := range evaluator.Tags {
		bucket, exists := rs.rulesByTag[tag]
		if !exists {
			bucket = &RuleBucket{}
			rs.rulesByTag[tag] = bucket
		}

		bucket.AddRule(rule)
	}

	return rule, nil
}

func (rs *RuleSet) NotifyRuleMatch(rule *Rule, event Event) {
	log.Printf("Rule %s was triggered (event: %+v)\n", rule.name, spew.Sdump(event))

	for _, listener := range rs.listeners {
		listener.RuleMatch(rule, event)
	}
}

func (rs *RuleSet) NotifyDiscriminatorDiscovered(event Event, field string) {
	log.Printf("No rule match for event %+v\n", event)

	for _, listener := range rs.listeners {
		listener.DiscriminatorDiscovered(event, field)
	}
}

func (rs *RuleSet) AddListener(listener RuleSetListener) {
	rs.listeners = append(rs.listeners, listener)
}

func (rs *RuleSet) Evaluate(event Event) bool {
	result := false
	rs.model.SetData(event)
	context := &Context{}
	eventType := event.GetType()

	bucket, found := rs.rulesByTag[eventType]
	log.Printf("Evaluating event of type %s against set of %d rules", eventType, len(bucket.rules))

	if found {
		for _, rule := range bucket.rules {
			if rule.evaluator.Eval(context) {
				rs.NotifyRuleMatch(rule, event)
				result = true
			}
		}

		log.Printf("Looking for discriminators")
		if !result {
			// Look for discriminators
			for _, field := range bucket.fields {
				for _, rule := range bucket.rules {
					if rule.evaluator.partialEval[field](context) {
						return result
					}
				}
				rs.NotifyDiscriminatorDiscovered(event, field)
			}
		}
	}

	return result
}

func NewRuleSet(model Model, debug bool) *RuleSet {
	return &RuleSet{
		model:      model,
		debug:      debug,
		rulesByTag: make(map[string]*RuleBucket),
	}
}
