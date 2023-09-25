// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/spf13/cast"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/hashicorp/go-multierror"
)

// MacroID represents the ID of a macro
type MacroID = string

// CombinePolicy represents the policy to use to combine rules and macros
type CombinePolicy = string

// Combine policies
const (
	NoPolicy       CombinePolicy = ""
	MergePolicy    CombinePolicy = "merge"
	OverridePolicy CombinePolicy = "override"
)

// Ruleset loading operations
const (
	RuleSetTagKey          = "ruleset"
	DefaultRuleSetTagValue = "probe_evaluation"
)

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID                     MacroID       `yaml:"id"`
	Expression             string        `yaml:"expression"`
	Description            string        `yaml:"description"`
	AgentVersionConstraint string        `yaml:"agent_version"`
	Filters                []string      `yaml:"filters"`
	Values                 []string      `yaml:"values"`
	Combine                CombinePolicy `yaml:"combine"`
}

// MergeWith merges macro m2 into m
func (m *MacroDefinition) MergeWith(m2 *MacroDefinition) error {
	switch m2.Combine {
	case MergePolicy:
		if m.Expression != "" || m2.Expression != "" {
			return &ErrMacroLoad{Definition: m2, Err: ErrCannotMergeExpression}
		}
		m.Values = append(m.Values, m2.Values...)
	case OverridePolicy:
		m.Values = m2.Values
	default:
		return &ErrMacroLoad{Definition: m2, Err: ErrDefinitionIDConflict}
	}
	return nil
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
	ID                     RuleID             `yaml:"id"`
	Version                string             `yaml:"version"`
	Expression             string             `yaml:"expression"`
	Description            string             `yaml:"description"`
	Tags                   map[string]string  `yaml:"tags"`
	AgentVersionConstraint string             `yaml:"agent_version"`
	Filters                []string           `yaml:"filters"`
	Disabled               bool               `yaml:"disabled"`
	Combine                CombinePolicy      `yaml:"combine"`
	Actions                []ActionDefinition `yaml:"actions"`
	Every                  time.Duration      `yaml:"every"`
	Policy                 *Policy
}

// GetTag returns the tag value associated with a tag key
func (rd *RuleDefinition) GetTag(tagKey string) (string, bool) {
	tagValue, ok := rd.Tags[tagKey]
	if ok {
		return tagValue, true
	}
	return "", false
}

// MergeWith merges rule rd2 into rd
func (rd *RuleDefinition) MergeWith(rd2 *RuleDefinition) error {
	switch rd2.Combine {
	case OverridePolicy:
		rd.Expression = rd2.Expression
	default:
		if !rd2.Disabled {
			return &ErrRuleLoad{Definition: rd2, Err: ErrDefinitionIDConflict}
		}
	}
	rd.Disabled = rd2.Disabled
	return nil
}

// ActionDefinition describes a rule action section
type ActionDefinition struct {
	Set *SetDefinition `yaml:"set"`
}

// Check returns an error if the action in invalid
func (a *ActionDefinition) Check() error {
	if a.Set == nil {
		return errors.New("missing 'set' section in action")
	}

	if a.Set.Name == "" {
		return errors.New("action name is empty")
	}

	if (a.Set.Value == nil && a.Set.Field == "") || (a.Set.Value != nil && a.Set.Field != "") {
		return errors.New("either 'value' or 'field' must be specified")
	}

	return nil
}

// Scope describes the scope variables
type Scope string

// SetDefinition describes the 'set' section of a rule action
type SetDefinition struct {
	Name   string      `yaml:"name"`
	Value  interface{} `yaml:"value"`
	Field  string      `yaml:"field"`
	Append bool        `yaml:"append"`
	Scope  Scope       `yaml:"scope"`
}

// Rule describes a rule of a ruleset
type Rule struct {
	*eval.Rule
	Definition *RuleDefinition
}

// RuleSetListener describes the methods implemented by an object used to be
// notified of events on a rule set.
type RuleSetListener interface {
	RuleMatch(rule *Rule, event eval.Event) bool
	EventDiscarderFound(rs *RuleSet, event eval.Event, field eval.Field, eventType eval.EventType)
}

// RuleSet holds a list of rules, grouped in bucket. An event can be evaluated
// against it. If the rule matches, the listeners for this rule set are notified
type RuleSet struct {
	opts             *Opts
	evalOpts         *eval.Opts
	eventRuleBuckets map[eval.EventType]*RuleBucket
	rules            map[eval.RuleID]*Rule
	policies         []*Policy
	fieldEvaluators  map[string]eval.Evaluator
	model            eval.Model
	eventCtor        func() eval.Event
	listenersLock    sync.RWMutex
	listeners        []RuleSetListener
	globalVariables  eval.GlobalVariables
	scopedVariables  map[Scope]VariableProvider
	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields []string
	logger log.Logger
	pool   *eval.ContextPool

	// event collector, used for tests
	eventCollector EventCollector
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

// GetRuleSetTag gets the value of the "ruleset" tag, which is the tag of the rules that belong in this rule set
func (rs *RuleSet) GetRuleSetTag() eval.RuleSetTagValue {
	return rs.opts.RuleSetTag[RuleSetTagKey]
}

// ListMacroIDs returns the list of MacroIDs from the ruleset
func (rs *RuleSet) ListMacroIDs() []MacroID {
	var ids []string
	for _, macro := range rs.evalOpts.MacroStore.List() {
		ids = append(ids, macro.ID)
	}
	return ids
}

// AddMacros parses the macros AST and adds them to the list of macros of the ruleset
func (rs *RuleSet) AddMacros(parsingContext *ast.ParsingContext, macros []*MacroDefinition) *multierror.Error {
	var result *multierror.Error

	// Build the list of macros for the ruleset
	for _, macroDef := range macros {
		if _, err := rs.AddMacro(parsingContext, macroDef); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

// AddMacro parses the macro AST and adds it to the list of macros of the ruleset
func (rs *RuleSet) AddMacro(parsingContext *ast.ParsingContext, macroDef *MacroDefinition) (*eval.Macro, error) {
	var err error

	if macro := rs.evalOpts.MacroStore.Get(macroDef.ID); macro != nil {
		return nil, &ErrMacroLoad{Definition: macroDef, Err: ErrDefinitionIDConflict}
	}

	macro := &Macro{Definition: macroDef}

	switch {
	case macroDef.Expression != "" && len(macroDef.Values) > 0:
		return nil, &ErrMacroLoad{Definition: macroDef, Err: errors.New("only one of 'expression' and 'values' can be defined")}
	case macroDef.Expression != "":
		if macro.Macro, err = eval.NewMacro(macroDef.ID, macroDef.Expression, rs.model, parsingContext, rs.evalOpts); err != nil {
			return nil, &ErrMacroLoad{Definition: macroDef, Err: err}
		}
	default:
		if macro.Macro, err = eval.NewStringValuesMacro(macroDef.ID, macroDef.Values, rs.evalOpts); err != nil {
			return nil, &ErrMacroLoad{Definition: macroDef, Err: err}
		}
	}

	rs.evalOpts.MacroStore.Add(macro.Macro)

	return macro.Macro, nil
}

// AddRules adds rules to the ruleset and generate their partials
func (rs *RuleSet) AddRules(parsingContext *ast.ParsingContext, rules []*RuleDefinition) *multierror.Error {
	var result *multierror.Error

	for _, ruleDef := range rules {
		if _, err := rs.AddRule(parsingContext, ruleDef); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

func (rs *RuleSet) populateFieldsWithRuleActionsData(policyRules []*RuleDefinition) *multierror.Error {
	var errs *multierror.Error

	for _, rule := range policyRules {
		for _, action := range rule.Actions {
			if err := action.Check(); err != nil {
				errs = multierror.Append(errs, fmt.Errorf("invalid action: %w", err))
				continue
			}

			if action.Set != nil {
				varName := action.Set.Name
				if action.Set.Scope != "" {
					varName = string(action.Set.Scope) + "." + varName
				}

				if _, err := rs.eventCtor().GetFieldValue(varName); err == nil {
					errs = multierror.Append(errs, fmt.Errorf("variable '%s' conflicts with field", varName))
					continue
				}

				if _, found := rs.evalOpts.Constants[varName]; found {
					errs = multierror.Append(errs, fmt.Errorf("variable '%s' conflicts with constant", varName))
					continue
				}

				var variableValue interface{}

				if action.Set.Value != nil {
					switch value := action.Set.Value.(type) {
					case int:
						action.Set.Value = []int{value}
					case string:
						action.Set.Value = []string{value}
					case []interface{}:
						if len(value) == 0 {
							errs = multierror.Append(errs, fmt.Errorf("unable to infer item type for '%s'", action.Set.Name))
							continue
						}

						switch arrayType := value[0].(type) {
						case int:
							action.Set.Value = cast.ToIntSlice(value)
						case string:
							action.Set.Value = cast.ToStringSlice(value)
						default:
							errs = multierror.Append(errs, fmt.Errorf("unsupported item type '%s' for array '%s'", reflect.TypeOf(arrayType), action.Set.Name))
							continue
						}
					}

					variableValue = action.Set.Value
				} else if action.Set.Field != "" {
					kind, err := rs.eventCtor().GetFieldType(action.Set.Field)
					if err != nil {
						errs = multierror.Append(errs, fmt.Errorf("failed to get field '%s': %w", action.Set.Field, err))
						continue
					}

					switch kind {
					case reflect.String:
						variableValue = []string{}
					case reflect.Int:
						variableValue = []int{}
					case reflect.Bool:
						variableValue = false
					default:
						errs = multierror.Append(errs, fmt.Errorf("unsupported field type '%s' for variable '%s'", kind, action.Set.Name))
						continue
					}
				}

				var variable eval.VariableValue
				var variableProvider VariableProvider

				if action.Set.Scope != "" {
					stateScopeBuilder := rs.opts.StateScopes[action.Set.Scope]
					if stateScopeBuilder == nil {
						errs = multierror.Append(errs, fmt.Errorf("invalid scope '%s'", action.Set.Scope))
						continue
					}

					if _, found := rs.scopedVariables[action.Set.Scope]; !found {
						rs.scopedVariables[action.Set.Scope] = stateScopeBuilder()
					}

					variableProvider = rs.scopedVariables[action.Set.Scope]
				} else {
					variableProvider = &rs.globalVariables
				}

				variable, err := variableProvider.GetVariable(action.Set.Name, variableValue)
				if err != nil {
					errs = multierror.Append(errs, fmt.Errorf("invalid type '%s' for variable '%s': %w", reflect.TypeOf(action.Set.Value), action.Set.Name, err))
					continue
				}

				if existingVariable := rs.evalOpts.VariableStore.Get(varName); existingVariable != nil && reflect.TypeOf(variable) != reflect.TypeOf(existingVariable) {
					errs = multierror.Append(errs, fmt.Errorf("conflicting types for variable '%s'", varName))
					continue
				}

				rs.evalOpts.VariableStore.Add(varName, variable)
			}
		}
	}
	return errs
}

// ListFields returns all the fields accessed by all rules of this rule set
func (rs *RuleSet) ListFields() []string {
	return rs.fields
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
func (rs *RuleSet) AddRule(parsingContext *ast.ParsingContext, ruleDef *RuleDefinition) (*eval.Rule, error) {
	if ruleDef.Disabled {
		return nil, nil
	}

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
		Rule:       eval.NewRule(ruleDef.ID, ruleDef.Expression, rs.evalOpts, tags...),
		Definition: ruleDef,
	}

	if err := rule.Parse(parsingContext); err != nil {
		return nil, &ErrRuleLoad{Definition: ruleDef, Err: &ErrRuleSyntax{Err: err}}
	}

	if err := rule.GenEvaluator(rs.model, parsingContext); err != nil {
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

	// Generate evaluator for fields that are used in variables
	for _, action := range rule.Definition.Actions {
		if action.Set != nil && action.Set.Field != "" {
			if _, found := rs.fieldEvaluators[action.Set.Field]; !found {
				evaluator, err := rs.model.GetEvaluator(action.Set.Field, "")
				if err != nil {
					return nil, err
				}
				rs.fieldEvaluators[action.Set.Field] = evaluator
			}
		}
	}

	return rule.Rule, nil
}

// NotifyRuleMatch notifies all the ruleset listeners that an event matched a rule
func (rs *RuleSet) NotifyRuleMatch(rule *Rule, event eval.Event) {
	rs.listenersLock.RLock()
	defer rs.listenersLock.RUnlock()

	for _, listener := range rs.listeners {
		if !listener.RuleMatch(rule, event) {
			break
		}
	}
}

// NotifyDiscarderFound notifies all the ruleset listeners that a discarder was found for an event
func (rs *RuleSet) NotifyDiscarderFound(event eval.Event, field eval.Field, eventType eval.EventType) {
	rs.listenersLock.RLock()
	defer rs.listenersLock.RUnlock()

	for _, listener := range rs.listeners {
		listener.EventDiscarderFound(rs, event, field, eventType)
	}
}

// AddListener adds a listener on the ruleset
func (rs *RuleSet) AddListener(listener RuleSetListener) {
	rs.listenersLock.Lock()
	defer rs.listenersLock.Unlock()

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

// GetBucket returns rule bucket for the given event type
func (rs *RuleSet) GetBucket(eventType eval.EventType) *RuleBucket {
	if bucket, exists := rs.eventRuleBuckets[eventType]; exists {
		return bucket
	}
	return nil
}

// GetApprovers returns all approvers
func (rs *RuleSet) GetApprovers(fieldCaps map[eval.EventType]FieldCapabilities) (map[eval.EventType]Approvers, error) {
	approvers := make(map[eval.EventType]Approvers)
	for _, eventType := range rs.GetEventTypes() {
		caps, exists := fieldCaps[eventType]
		if !exists {
			continue
		}

		eventApprovers, err := rs.GetEventApprovers(eventType, caps)
		if err != nil || len(eventApprovers) == 0 {
			continue
		}
		approvers[eventType] = eventApprovers
	}

	return approvers, nil
}

// GetEventApprovers returns approvers for the given event type and the fields
func (rs *RuleSet) GetEventApprovers(eventType eval.EventType, fieldCaps FieldCapabilities) (Approvers, error) {
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return nil, ErrNoEventTypeBucket{EventType: eventType}
	}

	return GetApprovers(bucket.rules, model.NewDefaultEvent(), fieldCaps)
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
func IsDiscarder(ctx *eval.Context, field eval.Field, rules []*Rule) (bool, error) {
	for _, rule := range rules {
		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || isTrue {
			return false, err
		}
	}
	return true, nil
}

// IsDiscarder partially evaluates an Event against a field
func (rs *RuleSet) IsDiscarder(event eval.Event, field eval.Field) (bool, error) {
	eventType, err := event.GetFieldEventType(field)
	if err != nil {
		return false, err
	}

	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return false, &ErrNoEventTypeBucket{EventType: eventType}
	}

	ctx := rs.pool.Get(event)
	defer rs.pool.Put(ctx)

	return IsDiscarder(ctx, field, bucket.rules)
}

func (rs *RuleSet) runRuleActions(ctx *eval.Context, rule *Rule) error {
	for _, action := range rule.Definition.Actions {
		switch {
		case action.Set != nil:
			name := string(action.Set.Scope)
			if name != "" {
				name += "."
			}
			name += action.Set.Name

			variable := rs.evalOpts.VariableStore.Get(name)
			if variable == nil {
				return fmt.Errorf("unknown variable: %s", name)
			}

			if mutable, ok := variable.(eval.MutableVariable); ok {
				value := action.Set.Value
				if field := action.Set.Field; field != "" {
					if evaluator := rs.fieldEvaluators[field]; evaluator != nil {
						value = evaluator.Eval(ctx)
					}
				}

				if action.Set.Append {
					if err := mutable.Append(ctx, value); err != nil {
						return fmt.Errorf("append is not supported for %s", reflect.TypeOf(value))
					}
				} else {
					if err := mutable.Set(ctx, value); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// Evaluate the specified event against the set of rules
func (rs *RuleSet) Evaluate(event eval.Event) bool {
	ctx := rs.pool.Get(event)
	defer rs.pool.Put(ctx)

	eventType := event.GetType()

	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return false
	}

	// Since logger is an interface this call cannot be inlined, requiring to pass the trace call arguments
	// through the heap. To improve this situation we first check if we actually need to call the function.
	if rs.logger.IsTracing() {
		rs.logger.Tracef("Evaluating event of type `%s` against set of %d rules", eventType, len(bucket.rules))
	}

	result := false
	for _, rule := range bucket.rules {
		if rule.GetEvaluator().Eval(ctx) {

			if rs.logger.IsTracing() {
				rs.logger.Tracef("Rule `%s` matches with event `%s`\n", rule.ID, event)
			}

			rs.NotifyRuleMatch(rule, event)
			result = true

			if err := rs.runRuleActions(ctx, rule); err != nil {
				rs.logger.Errorf("Error while executing rule actions: %s", err)
			}
		}
	}

	// no-op in the general case, only used to collect events in functional tests
	// for debugging purposes
	rs.eventCollector.CollectEvent(rs, event, result)

	return result
}

// EvaluateDiscarders evaluates the discarders for the given event if any
func (rs *RuleSet) EvaluateDiscarders(event eval.Event) {
	ctx := rs.pool.Get(event)
	defer rs.pool.Put(ctx)

	eventType := event.GetType()
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return
	}

	if rs.logger.IsTracing() {
		rs.logger.Tracef("Looking for discarders for event of type `%s`", eventType)
	}

	for _, field := range bucket.fields {
		if rs.opts.SupportedDiscarders != nil {
			if _, exists := rs.opts.SupportedDiscarders[field]; !exists {
				continue
			}
		}

		if isDiscarder, _ := IsDiscarder(ctx, field, bucket.rules); isDiscarder {
			rs.NotifyDiscarderFound(event, field, eventType)
		}
	}
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

// StopEventCollector stops the event collector
func (rs *RuleSet) StopEventCollector() []CollectedEvent {
	return rs.eventCollector.Stop()
}

// NewEvent returns a new event using the embedded constructor
func (rs *RuleSet) NewEvent() eval.Event {
	return rs.eventCtor()
}

// NewRuleSet returns a new ruleset for the specified data model
func NewRuleSet(model eval.Model, eventCtor func() eval.Event, opts *Opts, evalOpts *eval.Opts) *RuleSet {
	logger := log.OrNullLogger(opts.Logger)

	if evalOpts.MacroStore == nil {
		evalOpts.WithMacroStore(&eval.MacroStore{})
	}

	if evalOpts.VariableStore == nil {
		evalOpts.WithVariableStore(&eval.VariableStore{})
	}

	return &RuleSet{
		model:            model,
		eventCtor:        eventCtor,
		opts:             opts,
		evalOpts:         evalOpts,
		eventRuleBuckets: make(map[eval.EventType]*RuleBucket),
		rules:            make(map[eval.RuleID]*Rule),
		logger:           logger,
		pool:             eval.NewContextPool(),
		fieldEvaluators:  make(map[string]eval.Evaluator),
		scopedVariables:  make(map[Scope]VariableProvider),
	}
}
