package rules

import (
	"fmt"

	"github.com/cihub/seelog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type RuleSetListener interface {
	RuleMatch(rule *eval.Rule, event eval.Event)
	EventDiscarderFound(event eval.Event, field eval.Field)
}

type RuleSet struct {
	opts             *eval.Opts
	eventRuleBuckets map[eval.EventType]*RuleBucket
	rules            map[policy.RuleID]*eval.Rule
	model            eval.Model
	eventCtor        func() eval.Event
	listeners        []RuleSetListener
	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields []string
}

// ListRuleIDs - Returns the list of RuleIDs from the ruleset
func (rs *RuleSet) ListRuleIDs() []string {
	var ids []string
	for ruleID := range rs.rules {
		ids = append(ids, ruleID)
	}
	return ids
}

// AddMacros - Parses the macros AST and add them to the list of macros of the ruleset
func (rs *RuleSet) AddMacros(macros []*policy.MacroDefinition) error {
	var result *multierror.Error

	// Build the list of macros for the ruleset
	for _, macroDef := range macros {
		if _, err := rs.AddMacro(macroDef); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

func (rs *RuleSet) AddMacro(macroDef *policy.MacroDefinition) (*eval.Macro, error) {
	if _, exists := rs.opts.Macros[macroDef.ID]; exists {
		return nil, fmt.Errorf("found multiple definition of the macro '%s'", macroDef.ID)
	}

	macro := &eval.Macro{
		ID:         macroDef.ID,
		Expression: macroDef.Expression,
	}

	if err := macro.Parse(); err != nil {
		return nil, errors.Wrapf(err, "couldn't generate an AST of the macro %s", macroDef.ID)
	}

	if err := macro.GenEvaluator(rs.model, rs.opts); err != nil {
		return nil, errors.Wrapf(err, "couldn't generate an evaluation of the macro %s", macroDef.ID)
	}

	rs.opts.Macros[macro.ID] = macro

	return macro, nil
}

// AddRules - Adds rules to the ruleset and generate their partials
func (rs *RuleSet) AddRules(rules []*policy.RuleDefinition) error {
	var result *multierror.Error

	for _, ruleDef := range rules {
		if _, err := rs.AddRule(ruleDef); err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "couldn't add rule %s to the ruleset", ruleDef.ID))
		}
	}

	if err := rs.generatePartials(); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "couldn't generate partials"))
	}

	return result
}

// AddRule - Creates the rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(ruleDef *policy.RuleDefinition) (*eval.Rule, error) {
	if _, exists := rs.rules[ruleDef.ID]; exists {
		return nil, fmt.Errorf("found multiple definition of the rule '%s'", ruleDef.ID)
	}

	rule := &eval.Rule{
		ID:         ruleDef.ID,
		Expression: ruleDef.Expression,
		Tags:       ruleDef.GetTags(),
	}

	if err := rule.Parse(); err != nil {
		return nil, err
	}

	if err := rule.GenEvaluator(rs.model, rs.opts); err != nil {
		return nil, err
	}

	for _, event := range rule.GetEvaluator().EventTypes {
		bucket, exists := rs.eventRuleBuckets[event]
		if !exists {
			bucket = &RuleBucket{}
			rs.eventRuleBuckets[event] = bucket
		}

		if err := bucket.AddRule(rule); err != nil {
			return nil, err
		}
	}

	if len(rule.GetEventTypes()) == 0 {
		log.Errorf("rule without event specified: %s", ruleDef.Expression)
		return nil, RuleWithoutEventErr
	}

	// TODO: this contraints could be removed, but currenlty approver resolution can't handle multiple event type approver
	if len(rule.GetEventTypes()) > 1 {
		log.Errorf("mulitple event types specified on the same rule: %s", ruleDef.Expression)
		return nil, RuleWithMultipleEventsErr
	}

	// Merge the fields of the new rule with the existing list of fields of the ruleset
	rs.AddFields(rule.GetEvaluator().GetFields())

	rs.rules[ruleDef.ID] = rule

	return rule, nil
}

func (rs *RuleSet) NotifyRuleMatch(rule *eval.Rule, event eval.Event) {
	for _, listener := range rs.listeners {
		listener.RuleMatch(rule, event)
	}
}

func (rs *RuleSet) NotifyDiscarderFound(event eval.Event, field eval.Field) {
	for _, listener := range rs.listeners {
		listener.EventDiscarderFound(event, field)
	}
}

func (rs *RuleSet) AddListener(listener RuleSetListener) {
	rs.listeners = append(rs.listeners, listener)
}

// HasRulesForEventType returns if there is at least one rule for the given event type
func (rs *RuleSet) HasRulesForEventType(eventType eval.EventType) bool {
	bucket, found := rs.eventRuleBuckets[eventType]
	if !found {
		return false
	}
	return len(bucket.rules) > 0
}

// GetApprovers returns Approvers for the given event type and the fields
func (rs *RuleSet) GetApprovers(eventType eval.EventType, fieldCaps FieldCapabilities) (Approvers, error) {
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return nil, NoEventTypeBucket{EventType: eventType}
	}

	return bucket.GetApprovers(rs.model, rs.eventCtor(), fieldCaps)
}

func (rs *RuleSet) Evaluate(event eval.Event) bool {
	result := false
	rs.model.SetEvent(event)
	ctx := &eval.Context{}
	eventType := event.GetType()
	eventID := event.GetID()

	bucket, found := rs.eventRuleBuckets[eventType]
	if !found {
		return result
	}
	log.Debugf("Evaluating event `%s` of type `%s` against set of %d rules", eventID, eventType, len(bucket.rules))

	for _, rule := range bucket.rules {
		if rule.GetEvaluator().Eval(ctx) {
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
				evaluator, _ := rs.model.GetEvaluator(field)
				value = evaluator.(eval.Evaluator).StringValue(ctx)
			}

			found = true
			for _, rule := range bucket.rules {
				isTrue, err := rule.GetEvaluator().PartialEval(ctx, field)

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

func (rs *RuleSet) GetEventTypes() []eval.EventType {
	eventTypes := make([]string, 0, len(rs.eventRuleBuckets))
	for eventType := range rs.eventRuleBuckets {
		eventTypes = append(eventTypes, eventType)
	}
	return eventTypes
}

// AddFields - Merges the provided set of fields with the existing set of fields of the ruleset
func (rs *RuleSet) AddFields(fields []eval.EventType) {
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

// generatePartials - Generate the partials of the ruleset. A partial is a boolean evalution function that only depends
// on one field. The goal of partial is to determine if a rule depends on a specific field, so that we can decide if
// we should create an in-kernel filter for that field.
func (rs *RuleSet) generatePartials() error {
	// Compute the partials of each rule
	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.GetRules() {
			if err := rule.GenPartials(); err != nil {
				return err
			}
		}
	}
	return nil
}

func NewRuleSet(model eval.Model, eventCtor func() eval.Event, opts *eval.Opts) *RuleSet {
	return &RuleSet{
		model:            model,
		eventCtor:        eventCtor,
		opts:             opts,
		eventRuleBuckets: make(map[eval.EventType]*RuleBucket),
	}
}
