// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// MacroID represents the ID of a macro
type MacroID = string

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID         MacroID `yaml:"id"`
	Expression string  `yaml:"expression"`
}

// Macro describes a macro of a ruleset
type Macro struct {
	*eval.Macro
	Definition *MacroDefinition
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID          RuleID            `yaml:"id"`
	Version     string            `yaml:"version"`
	Expression  string            `yaml:"expression"`
	Description string            `yaml:"description"`
	Tags        map[string]string `yaml:"tags"`
	Policy      *Policy
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

// Rule describes a rule of a ruleset
type Rule struct {
	*eval.Rule
	Definition *RuleDefinition
}

// RuleSetListener describes the methods implemented by an object used to be
// notified of events on a rule set.
type RuleSetListener interface {
	RuleMatch(rule *Rule, event eval.Event)
	EventDiscarderFound(rs *RuleSet, event eval.Event, field eval.Field, eventType eval.EventType)
}

// RuleSet holds a list of rules, grouped in bucket. An event can be evaluated
// against it. If the rule matches, the listeners for this rule set are notified
type RuleSet struct {
	opts             *Opts
	loadedPolicies   map[string]string
	eventRuleBuckets map[eval.EventType]*RuleBucket
	rules            map[eval.RuleID]*Rule
	macros           map[eval.RuleID]*Macro
	model            eval.Model
	eventCtor        func() eval.Event
	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields []string
	logger Logger
}

// ListRuleIDs returns the list of RuleIDs from the ruleset
func (rs *RuleSet) ListRuleIDs() []RuleID {
	var ids []string
	for ruleID := range rs.rules {
		ids = append(ids, ruleID)
	}
	return ids
}

// GetRules returns the active rules
func (rs *RuleSet) GetRules() map[eval.RuleID]*Rule {
	return rs.rules
}

// ListMacroIDs returns the list of MacroIDs from the ruleset
func (rs *RuleSet) ListMacroIDs() []MacroID {
	var ids []string
	for macroID := range rs.opts.Macros {
		ids = append(ids, macroID)
	}
	return ids
}

// AddMacros parses the macros AST and adds them to the list of macros of the ruleset
func (rs *RuleSet) AddMacros(macros []*MacroDefinition) *multierror.Error {
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
		return nil, &ErrMacroLoad{Definition: macroDef, Err: errors.New("multiple definition with the same ID")}
	}

	macro := &Macro{
		Macro: &eval.Macro{
			ID:         macroDef.ID,
			Expression: macroDef.Expression,
		},
		Definition: macroDef,
	}

	if err := macro.Parse(); err != nil {
		return nil, &ErrMacroLoad{Definition: macroDef, Err: errors.Wrap(err, "syntax error")}
	}

	if err := macro.GenEvaluator(rs.model, &rs.opts.Opts); err != nil {
		return nil, &ErrMacroLoad{Definition: macroDef, Err: errors.Wrap(err, "compilation error")}
	}

	rs.opts.AddMacro(macro.Macro)

	return macro.Macro, nil
}

// AddRules adds rules to the ruleset and generate their partials
func (rs *RuleSet) AddRules(rules []*RuleDefinition) *multierror.Error {
	var result *multierror.Error

	for _, ruleDef := range rules {
		if _, err := rs.AddRule(ruleDef); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if err := rs.generatePartials(); err != nil {
		result = multierror.Append(result, errors.Wrapf(err, "couldn't generate partials for rule"))
	}

	return result
}

// GetRuleEventType return the rule EventType. Currently rules support only one eventType
func GetRuleEventType(rule *eval.Rule) (eval.EventType, error) {
	eventTypes, err := rule.GetEventTypes()
	if err != nil {
		return "", err
	}

	if len(eventTypes) == 0 {
		return "", ErrRuleWithoutEvent
	}

	// TODO: this contraints could be removed, but currently approver resolution can't handle multiple event type approver
	if len(eventTypes) > 1 {
		return "", ErrRuleWithMultipleEvents
	}

	return eventTypes[0], nil
}

// AddRule creates the rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(ruleDef *RuleDefinition) (*eval.Rule, error) {
	for _, id := range rs.opts.ReservedRuleIDs {
		if id == ruleDef.ID {
			return nil, &ErrRuleLoad{Definition: ruleDef, Err: ErrInternalIDConflict}
		}
	}

	if _, exists := rs.rules[ruleDef.ID]; exists {
		return nil, &ErrRuleLoad{Definition: ruleDef, Err: ErrDefinitionIDConflict}
	}

	var tags []string
	for k, v := range ruleDef.Tags {
		tags = append(tags, k+":"+v)
	}

	rule := &Rule{
		Rule: &eval.Rule{
			ID:         ruleDef.ID,
			Expression: ruleDef.Expression,
			Tags:       tags,
		},
		Definition: ruleDef,
	}

	if err := rule.Parse(); err != nil {
		return nil, &ErrRuleLoad{Definition: ruleDef, Err: errors.Wrap(err, "syntax error")}
	}

	if err := rule.GenEvaluator(rs.model, &rs.opts.Opts); err != nil {
		return nil, &ErrRuleLoad{Definition: ruleDef, Err: err}
	}

	eventType, err := GetRuleEventType(rule.Rule)
	if err != nil {
		return nil, &ErrRuleLoad{Definition: ruleDef, Err: err}
	}

	// ignore event types not supported
	if _, exists := rs.opts.EventTypeEnabled["*"]; !exists {
		if _, exists := rs.opts.EventTypeEnabled[eventType]; !exists {
			return nil, &ErrRuleLoad{Definition: ruleDef, Err: ErrEventTypeNotEnabled}
		}
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

	// Merge the fields of the new rule with the existing list of fields of the ruleset
	rs.AddFields(rule.GetEvaluator().GetFields())

	rs.rules[ruleDef.ID] = rule

	return rule.Rule, nil
}

// HasRulesForEventType returns if there is at least one rule for the given event type
func (rs *RuleSet) HasRulesForEventType(eventType eval.EventType) bool {
	bucket, found := rs.eventRuleBuckets[eventType]
	if !found {
		return false
	}
	return len(bucket.rules) > 0
}

// GetBucket returns rule bucket for the given event type
func (rs *RuleSet) GetBucket(eventType eval.EventType) *RuleBucket {
	if bucket, exists := rs.eventRuleBuckets[eventType]; exists {
		return bucket
	}
	return nil
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

// Evaluate the specified event against the set of rules
func (rs *RuleSet) Evaluate(ctx *eval.Context, event eval.Event, cb func(*Rule, eval.Event)) bool {
	eventType := event.GetType()

	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return false
	}
	rs.logger.Tracef("Evaluating event of type `%s` against set of %d rules", eventType, len(bucket.rules))

	for _, rule := range bucket.rules {
		if rule.GetEvaluator().Eval(ctx) {
			rs.logger.Tracef("Rule `%s` matches with event `%s`\n", rule.ID, event)

			if cb != nil {
				cb(rule, event)
			}
			return true
		}
	}

	return false
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

// AddPolicyVersion adds the provided policy filename and version to the map of loaded policies
func (rs *RuleSet) AddPolicyVersion(filename string, version string) {
	rs.loadedPolicies[strings.ReplaceAll(filename, ".", "_")] = version
}

// NewRuleSet returns a new ruleset for the specified data model
func NewRuleSet(model eval.Model, eventCtor func() eval.Event, opts *Opts) *RuleSet {
	var logger Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = &NullLogger{}
	}

	return &RuleSet{
		model:            model,
		eventCtor:        eventCtor,
		opts:             opts,
		eventRuleBuckets: make(map[eval.EventType]*RuleBucket),
		rules:            make(map[eval.RuleID]*Rule),
		macros:           make(map[eval.RuleID]*Macro),
		loadedPolicies:   make(map[string]string),
		logger:           logger,
	}
}
