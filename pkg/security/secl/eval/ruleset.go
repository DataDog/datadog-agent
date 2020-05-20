package eval

import (
	"math/rand"
	"sort"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type FieldApprover struct {
	Field string
	Value interface{}
	Type  FieldValueType
	Not   bool
}

type RuleSetListener interface {
	RuleMatch(rule *Rule, event Event)
	EventDiscarderFound(event Event, field string)
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
	opts             Opts
	eventRuleBuckets map[string]*RuleBucket
	model            Model
	eventCtor        func() Event
	listeners        []RuleSetListener
}

func (rs *RuleSet) AddRule(id string, astRule *ast.Rule, tags ...string) (*Rule, error) {
	evaluator, err := RuleToEvaluator(astRule, rs.model, rs.opts)
	if err != nil {
		return nil, err
	}

	rule := &Rule{
		ID:         id,
		Expression: astRule.Expr,
		evaluator:  evaluator,
		Tags:       tags,
	}

	for _, event := range evaluator.EventTypes {
		bucket, exists := rs.eventRuleBuckets[event]
		if !exists {
			bucket = &RuleBucket{}
			rs.eventRuleBuckets[event] = bucket
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

func (rs *RuleSet) NotifyDiscarderFound(event Event, field string) {
	for _, listener := range rs.listeners {
		listener.EventDiscarderFound(event, field)
	}
}

func (rs *RuleSet) AddListener(listener RuleSetListener) {
	rs.listeners = append(rs.listeners, listener)
}

func (rs *RuleSet) HasRulesForEventType(kind string) bool {
	bucket, found := rs.eventRuleBuckets[kind]
	if !found {
		return false
	}
	return len(bucket.rules) > 0
}

type EventApprovers map[string][]FieldApprover

type FilteringCapability struct {
	Field string
	Types FieldValueType
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func notOfValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return ^v, nil
	case string:
		return RandStringRunes(256), nil
	case bool:
		return !v, nil
	}

	return nil, errors.New("value type unknown")
}

func (rs *RuleSet) getFieldApprovers(evaluator *RuleEvaluator, isValidEventTypeFnc func(eventType string) bool) ([]FieldApprover, error) {
	var ctx Context

	var approvers []FieldApprover
	for field, fValues := range evaluator.FieldValues {
		if eventType, _ := rs.model.GetEventType(field); !isValidEventTypeFnc(eventType) {
			continue
		}

		if len(fValues) == 0 {
			return nil, &NoValue{Field: field}
		}

		for _, fValue := range fValues {
			rs.model.SetEventValue(field, fValue.Value)
			if result, _ := evaluator.PartialEval(&ctx, field); result {
				approvers = append(approvers, FieldApprover{
					Field: field,
					Value: fValue.Value,
					Type:  fValue.Type,
					Not:   false,
				})
			}

			value, err := notOfValue(fValue.Value)
			if err != nil {
				return nil, &ValueTypeUnknown{Field: field}
			}

			rs.model.SetEventValue(field, value)
			if result, _ := evaluator.PartialEval(&ctx, field); result {
				approvers = append(approvers, FieldApprover{
					Field: field,
					Value: fValue.Value,
					Type:  fValue.Type,
					Not:   true,
				})
			}
		}
	}

	return approvers, nil
}

func (rs *RuleSet) GetEventApprovers(eventType string, capabitlies ...FilteringCapability) (EventApprovers, error) {
	rs.model.SetEvent(rs.eventCtor())

	capsMap := make(map[string]FilteringCapability)
	for _, cap := range capabitlies {
		capsMap[cap.Field] = cap
	}

	eventApprovers := make(EventApprovers)

	updateEventApprovers := func(approver FieldApprover) error {
		approvers := eventApprovers[approver.Field]

		cap, ok := capsMap[approver.Field]
		if !ok {
			return &CapabilityNotFound{Field: approver.Field}
		}

		if approver.Type&cap.Types == 0 {
			return &CapabilityMismatch{Field: approver.Field}
		}

		found := false
		for _, a := range approvers {
			if a == approver {
				found = true
			}
		}

		if !found {
			approvers = append(approvers, approver)
		}
		eventApprovers[approver.Field] = approvers

		return nil
	}

	if bucket, ok := rs.eventRuleBuckets[eventType]; ok {
		for _, rule := range bucket.rules {
			wildcardApprovers, err := rs.getFieldApprovers(rule.evaluator, func(kind string) bool { return kind == "*" })
			if err != nil {
				return nil, err
			}

			for _, approver := range wildcardApprovers {
				if err := updateEventApprovers(approver); err != nil {
					return nil, err
				}
			}

			approvers, err := rs.getFieldApprovers(rule.evaluator, func(kind string) bool { return kind == eventType })
			if err != nil {
				return nil, err
			}

			for _, approver := range approvers {
				if err := updateEventApprovers(approver); err != nil {
					return nil, err
				}
			}
		}
	}

	return eventApprovers, nil
}

func (rs *RuleSet) Evaluate(event Event) bool {
	result := false
	rs.model.SetEvent(event)
	ctx := &Context{}
	eventType := event.GetType()
	eventID := event.GetID()

	bucket, found := rs.eventRuleBuckets[eventType]
	if !found {
		return result
	}
	log.Debugf("Evaluating event `%s` of type `%s` against set of %d rules", eventID, eventType, len(bucket.rules))

	for _, rule := range bucket.rules {
		if rule.evaluator.Eval(ctx) {
			log.Infof("Rule `%s` matches with event `%s`\n", rule.ID, event)

			rs.NotifyRuleMatch(rule, event)
			result = true
		}
	}

	if !result {
		log.Debugf("Looking for discarders for event `%s`", eventID)

		for _, field := range bucket.fields {
			var value string
			if level, _ := log.GetLogLevel(); level == seelog.DebugLvl {
				eval, _ := rs.model.GetEvaluator(field)
				value = eval.(Evaluator).StringValue(ctx)
			}

			found = true
			for _, rule := range bucket.rules {
				isTrue, err := rule.evaluator.PartialEval(ctx, field)

				log.Debugf("Partial eval of rule %s(`%s`) with field `%s` with value `%s` => %t\n", rule.ID, rule.Expression, field, value, isTrue)

				if err != nil || isTrue {
					found = false
					break
				}
			}
			if found {
				log.Debugf("Found a discarder for field `%s` with value `%s`\n", field, value)
				rs.NotifyDiscarderFound(event, field)
			}
		}
	}

	return result
}

func (rs *RuleSet) GetEventTypes() []string {
	eventTypes := make([]string, 0, len(rs.eventRuleBuckets))
	for eventType := range rs.eventRuleBuckets {
		eventTypes = append(eventTypes, eventType)
	}
	return eventTypes
}

func NewRuleSet(model Model, eventCtor func() Event, opts Opts) *RuleSet {
	return &RuleSet{
		model:            model,
		eventCtor:        eventCtor,
		opts:             opts,
		eventRuleBuckets: make(map[string]*RuleBucket),
	}
}
