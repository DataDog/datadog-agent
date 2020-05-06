package eval

import (
	"sort"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Event interface {
	GetID() string
	GetType() string
}

type RuleSetListener interface {
	RuleMatch(rule *Rule, event Event)
	DiscriminatorDiscovered(event Event, field string)
}

type Rule struct {
	Name       string
	Expression string
	evaluator  *RuleEvaluator
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
		Name:       name,
		Expression: expression,
		evaluator:  evaluator,
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
	for _, listener := range rs.listeners {
		listener.RuleMatch(rule, event)
	}
}

func (rs *RuleSet) NotifyDiscriminatorDiscovered(event Event, field string) {
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
	eventID := event.GetID()

	bucket, found := rs.rulesByTag[eventType]
	if !found {
		return result
	}
	log.Debugf("Evaluating event `%s` of type %s against set of %d rules", eventID, eventType, len(bucket.rules))

	for _, rule := range bucket.rules {
		if rule.evaluator.Eval(context) {
			log.Infof("Rule `%s` matches with event %+v\n", rule.Name, spew.Sdump(event))

			rs.NotifyRuleMatch(rule, event)
			result = true
		}
	}

	if !result {
		log.Debugf("Looking for discriminators for event `%s`", eventID)

		for _, field := range bucket.fields {
			eval, _ := rs.model.GetEvaluator(field)

			found = true
			for _, rule := range bucket.rules {
				partial, ok := rule.evaluator.partialEval[field]
				if !ok {
					continue
				}

				isTrue := partial(context)

				log.Debugf("Partial eval of rule %s(`%s`) with field `%s` with value `%s` => %t\n", rule.Name, rule.Expression, field, eval, isTrue)
				if isTrue {
					found = false
				}
			}
			if found {
				log.Debugf("Found discriminator for field `%s` with value `%s`\n", field, eval)
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
