package eval

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/secl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Filter struct {
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

func (rl *RuleBucket) GetRules() []*Rule {
	return rl.rules
}

type RuleSet struct {
	opts             Opts
	eventRuleBuckets map[string]*RuleBucket
	model            Model
	eventCtor        func() Event
	listeners        []RuleSetListener
	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields []string
}

// GenerateMacroEvaluators - Generates the macros evaluators for the provided list of macro ASTs. If a field is provided,
// the function will return the macro partials.
func (rs *RuleSet) GenerateMacroEvaluators(field string, macros map[string]*policy.MacroDefinition) error {
	macroEvaluators := make(map[string]*MacroEvaluator)
	for name, macro := range macros {
		eval, err := MacroToEvaluator(macro.Ast, rs.model, &rs.opts, field)
		if err != nil {
			if err, ok := err.(*AstToEvalError); ok {
				log.Errorf("macro syntax error: %s\n%s", err, secl.SprintExprAt(macro.Expression, err.Pos))
			} else {
				log.Errorf("macro parsing error: %s\n%s", err, macro.Expression)
			}
			return err
		}
		macroEvaluators[name] = eval
	}
	rs.opts.SetMacroEvaluators(field, macroEvaluators)
	return nil
}

// AddRule - Creates rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(ruleDef *policy.RuleDefinition) (*Rule, error) {
	// Generate rule evaluator
	evaluator, err := RuleToEvaluator(ruleDef.Ast, rs.model, &rs.opts)
	if err != nil {
		if err, ok := err.(*AstToEvalError); ok {
			log.Errorf("rule syntax error: %s\n%s", err, secl.SprintExprAt(ruleDef.Expression, err.Pos))
		} else {
			log.Errorf("rule compilation error: %s\n%s", err, ruleDef.Expression)
		}
		return nil, err
	}

	rule := &Rule{
		ID:         ruleDef.ID,
		Expression: ruleDef.Ast.Expr,
		evaluator:  evaluator,
		Tags:       ruleDef.GetTags(),
	}

	for _, event := range evaluator.EventTypes {
		bucket, exists := rs.eventRuleBuckets[event]
		if !exists {
			bucket = &RuleBucket{}
			rs.eventRuleBuckets[event] = bucket
		}
		bucket.AddRule(rule)
	}

	if len(rule.GetEventTypes()) == 0 {
		log.Errorf("rule without event specified: %s", ruleDef.Expression)
		return nil, RuleWithoutEventErr
	}

	// Merge the fields of the new rule with the existing list of fields of the ruleset
	rs.AddFields(evaluator.GetFields())
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

type EventFilters map[string][]Filter

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

// check is there is no opposite rule invalidating the current value
func isFilterValid(ctx *Context, bucket *RuleBucket, field string) bool {
	for _, rule := range bucket.rules {
		// do not evaluate rule not having the same field
		if _, ok := rule.evaluator.FieldValues[field]; !ok {
			continue
		}

		if result, _ := rule.evaluator.PartialEval(ctx, field); !result {
			return false
		}
	}

	return true
}

func (rs *RuleSet) getFilters(evaluator *RuleEvaluator, bucket *RuleBucket, isValidEventTypeFnc func(eventType string) bool) ([]Filter, error) {
	var ctx Context

	event := rs.model.GetEvent()

	var filters []Filter
	for field, fValues := range evaluator.FieldValues {
		if eventType, _ := event.GetFieldEventType(field); !isValidEventTypeFnc(eventType) {
			continue
		}

		if len(fValues) == 0 {
			return nil, &NoValue{Field: field}
		}

		for _, fValue := range fValues {
			event.SetFieldValue(field, fValue.Value)
			if result, _ := evaluator.PartialEval(&ctx, field); result {
				if !isFilterValid(&ctx, bucket, field) {
					return nil, &OppositeRule{Field: field}
				}
				filters = append(filters, Filter{
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

			event.SetFieldValue(field, value)
			if result, _ := evaluator.PartialEval(&ctx, field); result {
				if !isFilterValid(&ctx, bucket, field) {
					return nil, &OppositeRule{Field: field}
				}
				filters = append(filters, Filter{
					Field: field,
					Value: fValue.Value,
					Type:  fValue.Type,
					Not:   true,
				})
			}
		}
	}

	return filters, nil
}

func (rs *RuleSet) GetEventFilters(eventType string, capabilities ...FilteringCapability) (EventFilters, error) {
	rs.model.SetEvent(rs.eventCtor())

	capsMap := make(map[string]FilteringCapability)
	for _, cap := range capabilities {
		capsMap[cap.Field] = cap
	}

	eventFilters := make(EventFilters)

	updateEventFilters := func(filter Filter) error {
		approvers := eventFilters[filter.Field]

		cap, ok := capsMap[filter.Field]
		if !ok {
			return &CapabilityNotFound{Field: filter.Field}
		}

		if filter.Type&cap.Types == 0 {
			return &CapabilityMismatch{Field: filter.Field}
		}

		found := false
		for _, a := range approvers {
			if a == filter {
				found = true
			}
		}

		if !found {
			approvers = append(approvers, filter)
		}
		eventFilters[filter.Field] = approvers

		return nil
	}

	if bucket, ok := rs.eventRuleBuckets[eventType]; ok {
		for _, rule := range bucket.rules {
			wildcardFilters, err := rs.getFilters(rule.evaluator, bucket, func(kind string) bool { return kind == "*" })
			if err != nil {
				return nil, err
			}

			for _, approver := range wildcardFilters {
				if err := updateEventFilters(approver); err != nil {
					return nil, err
				}
			}

			approvers, err := rs.getFilters(rule.evaluator, bucket, func(kind string) bool { return kind == eventType })
			if err != nil {
				return nil, err
			}

			for _, approver := range approvers {
				if err := updateEventFilters(approver); err != nil {
					return nil, err
				}
			}
		}
	}

	return eventFilters, nil
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

// AddFields - Merges the provided set of fields with the existing set of fields of the ruleset
func (rs *RuleSet) AddFields(fields []string) {
NewFields:
	for _, newField := range fields {
		for _, oldField := range rs.fields {
			if oldField == newField {
				continue NewFields
			}
		}
		rs.fields = append(rs.fields, newField)
	}
}

// GeneratePartials - Generate the partials of the ruleset. A partial is a boolean evalution function that only depends
// on one field. The goal of partial is to determine if a rule depends on a specific field, so that we can decide if
// we should create an in-kernel filter for that field.
func (rs *RuleSet) GeneratePartials(macros map[string]*policy.MacroDefinition, rules map[string]*policy.RuleDefinition) error {
	// Compute the macros partials for each fields of the ruleset first.
	for _, field := range rs.fields {
		if err := rs.GenerateMacroEvaluators(field, macros); err != nil {
			return err
		}
	}

	// Compute the partials of each rule
	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.GetRules() {
			// Only generate partials if they have not been generated yet
			if rule.evaluator != nil && rule.evaluator.partialEvals != nil {
				continue
			}
			// Only generate partials for the fields of the rule
			for _, field := range rule.evaluator.GetFields() {
				state := newStateWithMacros(rs.model, field, rs.opts.GetMacroEvaluators(field))
				// Get the ast of the rule
				ruleDef, ok := rules[rule.ID]
				if !ok {
					return fmt.Errorf("couldn't find rule %s in the provided rules", rule.ID)
				}
				pEval, _, _, err := nodeToEvaluator(ruleDef.Ast.BooleanExpression, &rs.opts, state)
				if err != nil {
					return errors.Wrapf(err, "couldn't generate partial for field %s and rule %s", field, rule.ID)
				}
				pEvalBool, ok := pEval.(*BoolEvaluator)
				if !ok {
					return NewTypeError(ruleDef.Ast.Pos, reflect.Bool)
				}
				if pEvalBool.EvalFnc == nil {
					pEvalBool.EvalFnc = func(ctx *Context) bool {
						return pEvalBool.Value
					}
				}
				// Insert partial evaluators in the rule
				rule.SetPartial(field, pEvalBool.EvalFnc)
			}
		}
	}
	return nil
}

func NewRuleSet(model Model, eventCtor func() Event, opts Opts) *RuleSet {
	return &RuleSet{
		model:            model,
		eventCtor:        eventCtor,
		opts:             opts,
		eventRuleBuckets: make(map[string]*RuleBucket),
		fields:           []string{},
	}
}
