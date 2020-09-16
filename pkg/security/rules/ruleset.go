// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package rules

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MacroID represents the ID of a macro
type MacroID = string

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID         MacroID `yaml:"id"`
	Expression string  `yaml:"expression"`
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID         RuleID            `yaml:"id"`
	Expression string            `yaml:"expression"`
	Tags       map[string]string `yaml:"tags"`
}

// GetTags returns the tags associated to a rule
func (rd *RuleDefinition) GetTags() []string {
	tags := []string{}
	for k, v := range rd.Tags {
		tags = append(
			tags,
			fmt.Sprintf("%s:%s", k, v))
	}
	return tags
}

// RuleSetListener describes the methods implemented by an object used to be
// notified of events on a rule set.
type RuleSetListener interface {
	RuleMatch(rule *eval.Rule, event eval.Event)
	EventDiscarderFound(rs *RuleSet, event eval.Event, field eval.Field)
}

// Opts defines rules set options
type Opts struct {
	eval.Opts
	InvalidDiscarders map[eval.Field][]interface{}
}

func (o *Opts) getInvalidDiscarders() map[eval.Field]map[interface{}]bool {
	invalidDiscarders := make(map[eval.Field]map[interface{}]bool)

	if o.InvalidDiscarders != nil {
		for field, values := range o.InvalidDiscarders {
			ivalues := invalidDiscarders[field]
			if ivalues == nil {
				ivalues = make(map[interface{}]bool)
				invalidDiscarders[field] = ivalues
			}
			for _, value := range values {
				ivalues[value] = true
			}
		}
	}

	return invalidDiscarders
}

// NewOptsWithParams initializes a new Opts instance with Debug and Constants parameters
func NewOptsWithParams(debug bool, constants map[string]interface{}, invalidDiscarders map[eval.Field][]interface{}) *Opts {
	return &Opts{
		Opts: eval.Opts{
			Debug:     debug,
			Constants: constants,
			Macros:    make(map[eval.MacroID]*eval.Macro),
		},
		InvalidDiscarders: invalidDiscarders,
	}
}

// RuleSet holds a list of rules, grouped in bucket. An event can be evaluated
// against it. If the rule matches, the listeners for this rule set are notified
type RuleSet struct {
	opts             *Opts
	eventRuleBuckets map[eval.EventType]*RuleBucket
	rules            map[eval.RuleID]*eval.Rule
	model            eval.Model
	eventCtor        func() eval.Event
	listeners        []RuleSetListener
	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields            []string
	invalidDiscarders map[eval.Field]map[interface{}]bool
}

// ListRuleIDs returns the list of RuleIDs from the ruleset
func (rs *RuleSet) ListRuleIDs() []string {
	var ids []string
	for ruleID := range rs.rules {
		ids = append(ids, ruleID)
	}
	return ids
}

// AddMacros parses the macros AST and adds them to the list of macros of the ruleset
func (rs *RuleSet) AddMacros(macros []*MacroDefinition) error {
	var result *multierror.Error

	// Build the list of macros for the ruleset
	for _, macroDef := range macros {
		if _, err := rs.AddMacro(macroDef); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

// AddMacro parses the macro AST and adds it to the list of macros of the ruleset
func (rs *RuleSet) AddMacro(macroDef *MacroDefinition) (*eval.Macro, error) {
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

	if err := macro.GenEvaluator(rs.model, &rs.opts.Opts); err != nil {
		return nil, errors.Wrapf(err, "couldn't generate an evaluation of the macro %s", macroDef.ID)
	}

	rs.opts.Macros[macro.ID] = macro

	return macro, nil
}

// AddRules adds rules to the ruleset and generate their partials
func (rs *RuleSet) AddRules(rules []*RuleDefinition) error {
	var result *multierror.Error

	for _, ruleDef := range rules {
		if _, err := rs.AddRule(ruleDef); err != nil {
			result = multierror.Append(result, errors.Wrapf(err, "couldn't add rule %s to the ruleset", ruleDef.ID))
		}
	}

	if err := rs.generatePartials(); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "couldn't generate partials"))
	}

	return result.ErrorOrNil()
}

// AddRule creates the rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(ruleDef *RuleDefinition) (*eval.Rule, error) {
	if _, exists := rs.rules[ruleDef.ID]; exists {
		return nil, fmt.Errorf("found multiple definition of the rule '%s'", ruleDef.ID)
	}

	rule := &eval.Rule{
		ID:         ruleDef.ID,
		Expression: ruleDef.Expression,
	}

	if err := rule.Parse(); err != nil {
		return nil, err
	}

	if err := rule.GenEvaluator(rs.model, &rs.opts.Opts); err != nil {
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
		return nil, ErrRuleWithoutEvent
	}

	// TODO: this contraints could be removed, but currently approver resolution can't handle multiple event type approver
	if len(rule.GetEventTypes()) > 1 {
		log.Errorf("multiple event types specified on the same rule: %s", ruleDef.Expression)
		return nil, ErrRuleWithMultipleEvents
	}

	// Merge the fields of the new rule with the existing list of fields of the ruleset
	rs.AddFields(rule.GetEvaluator().GetFields())

	rs.rules[ruleDef.ID] = rule

	return rule, nil
}

// NotifyRuleMatch notifies all the ruleset listeners that an event matched a rule
func (rs *RuleSet) NotifyRuleMatch(rule *eval.Rule, event eval.Event) {
	for _, listener := range rs.listeners {
		listener.RuleMatch(rule, event)
	}
}

// NotifyDiscarderFound notifies all the ruleset listeners that a discarder was found for an event
func (rs *RuleSet) NotifyDiscarderFound(event eval.Event, field eval.Field) {
	for _, listener := range rs.listeners {
		listener.EventDiscarderFound(rs, event, field)
	}
}

// AddListener adds a listener on the ruleset
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
		return nil, ErrNoEventTypeBucket{EventType: eventType}
	}

	return bucket.GetApprovers(rs.eventCtor(), fieldCaps)
}

// GetFieldValues returns all the values of the given field
func (rs *RuleSet) GetFieldValues(field eval.Field) []eval.FieldValue {
	var values []eval.FieldValue

	for _, rule := range rs.rules {
		rv := rule.GetFieldValues(field)
		if len(rv) > 0 {
			values = append(values, rv...)
		}
	}

	return values
}

// IsDiscarder partially evaluates an Event against a field
func (rs *RuleSet) IsDiscarder(field eval.Field, value interface{}) (bool, error) {
	event := rs.eventCtor()
	if err := event.SetFieldValue(field, value); err != nil {
		return false, err
	}

	ctx := &eval.Context{}
	ctx.SetObject(event.GetPointer())

	eventType, err := event.GetFieldEventType(field)
	if err != nil {
		return false, err
	}

	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return false, &ErrNoEventTypeBucket{EventType: eventType}
	}

	for _, rule := range bucket.rules {
		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || isTrue {
			return false, err
		}
	}
	return true, nil
}

func (rs *RuleSet) isInvalidDiscarder(field eval.Field, value interface{}) bool {
	values, exists := rs.invalidDiscarders[field]
	if !exists {
		return false
	}

	return values[value]
}

// Evaluate the specified event against the set of rules
func (rs *RuleSet) Evaluate(event eval.Event) bool {
	ctx := &eval.Context{}
	ctx.SetObject(event.GetPointer())

	eventType := event.GetType()

	result := false
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return result
	}
	log.Tracef("Evaluating event of type `%s` against set of %d rules", eventType, len(bucket.rules))

	for _, rule := range bucket.rules {
		if rule.GetEvaluator().Eval(ctx) {
			log.Infof("Rule `%s` matches with event `%s`\n", rule.ID, event)

			rs.NotifyRuleMatch(rule, event)
			result = true
		}
	}

	if !result {
		log.Tracef("Looking for discarders for event of type `%s`", eventType)

		for _, field := range bucket.fields {
			evaluator, err := rs.model.GetEvaluator(field)
			if err != nil {
				continue
			}
			value := evaluator.Eval(ctx)

			if rs.isInvalidDiscarder(field, value) {
				continue
			}

			isDiscarder := true
			for _, rule := range bucket.rules {
				isTrue, err := rule.PartialEval(ctx, field)

				log.Tracef("Partial eval of rule %s(`%s`) with field `%s` with value `%v` => %t\n", rule.ID, rule.Expression, field, value, isTrue)

				if err != nil || isTrue {
					isDiscarder = false
					break
				}
			}
			if isDiscarder {
				log.Tracef("Found a discarder for field `%s` with value `%s`\n", field, value)
				rs.NotifyDiscarderFound(event, field)
			}
		}
	}

	return result
}

// GetEventTypes returns all the event types handled by the ruleset
func (rs *RuleSet) GetEventTypes() []eval.EventType {
	eventTypes := make([]string, 0, len(rs.eventRuleBuckets))
	for eventType := range rs.eventRuleBuckets {
		eventTypes = append(eventTypes, eventType)
	}
	return eventTypes
}

// AddFields merges the provided set of fields with the existing set of fields of the ruleset
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

// generatePartials generates the partials of the ruleset. A partial is a boolean evalution function that only depends
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

// NewRuleSet returns a new ruleset for the specified data model
func NewRuleSet(model eval.Model, eventCtor func() eval.Event, opts *Opts) *RuleSet {
	return &RuleSet{
		model:             model,
		eventCtor:         eventCtor,
		opts:              opts,
		eventRuleBuckets:  make(map[eval.EventType]*RuleBucket),
		rules:             make(map[eval.RuleID]*eval.Rule),
		invalidDiscarders: opts.getInvalidDiscarders(),
	}
}
