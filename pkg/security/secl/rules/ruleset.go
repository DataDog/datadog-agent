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
	"slices"
	"sync"

	"github.com/spf13/cast"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/utils"
)

// Rule presents a rule in a ruleset
type Rule struct {
	*PolicyRule
	*eval.Rule
	NoDiscarder bool
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
	fakeEventCtor    func() eval.Event
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

	OnDemandHookPoints []OnDemandHookPoint
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

// GetOnDemandHookPoints gets the on-demand hook points
func (rs *RuleSet) GetOnDemandHookPoints() []OnDemandHookPoint {
	return rs.OnDemandHookPoints
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
func (rs *RuleSet) AddMacros(parsingContext *ast.ParsingContext, macros []*PolicyMacro) *multierror.Error {
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
func (rs *RuleSet) AddMacro(parsingContext *ast.ParsingContext, pMacro *PolicyMacro) (*eval.Macro, error) {
	var err error

	if rs.evalOpts.MacroStore.Contains(pMacro.Def.ID) {
		return nil, &ErrMacroLoad{Macro: pMacro, Err: ErrDefinitionIDConflict}
	}

	var macro *eval.Macro

	switch {
	case pMacro.Def.Expression != "" && len(pMacro.Def.Values) > 0:
		return nil, &ErrMacroLoad{Macro: pMacro, Err: errors.New("only one of 'expression' and 'values' can be defined")}
	case pMacro.Def.Expression != "":
		if macro, err = eval.NewMacro(pMacro.Def.ID, pMacro.Def.Expression, rs.model, parsingContext, rs.evalOpts); err != nil {
			return nil, &ErrMacroLoad{Macro: pMacro, Err: err}
		}
	default:
		if macro, err = eval.NewStringValuesMacro(pMacro.Def.ID, pMacro.Def.Values, rs.evalOpts); err != nil {
			return nil, &ErrMacroLoad{Macro: pMacro, Err: err}
		}
	}

	rs.evalOpts.MacroStore.Add(macro)

	return macro, nil
}

// AddRules adds rules to the ruleset and generate their partials
func (rs *RuleSet) AddRules(parsingContext *ast.ParsingContext, pRules []*PolicyRule) *multierror.Error {
	var result *multierror.Error

	for _, pRule := range pRules {
		if _, err := rs.AddRule(parsingContext, pRule); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

// PopulateFieldsWithRuleActionsData populates the fields with the data from the rule actions
func (rs *RuleSet) PopulateFieldsWithRuleActionsData(policyRules []*PolicyRule, opts PolicyLoaderOpts) *multierror.Error {
	var errs *multierror.Error

	for _, rule := range policyRules {
		for _, actionDef := range rule.Def.Actions {
			if err := actionDef.Check(opts); err != nil {
				errs = multierror.Append(errs, fmt.Errorf("skipping invalid action in rule %s: %w", rule.Def.ID, err))
				continue
			}

			switch {
			case actionDef.Set != nil:
				varName := actionDef.Set.Name
				if actionDef.Set.Scope != "" {
					varName = string(actionDef.Set.Scope) + "." + varName
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

				if actionDef.Set.Value != nil {
					switch value := actionDef.Set.Value.(type) {
					case int:
						actionDef.Set.Value = []int{value}
					case string:
						actionDef.Set.Value = []string{value}
					case []interface{}:
						if len(value) == 0 {
							errs = multierror.Append(errs, fmt.Errorf("unable to infer item type for '%s'", actionDef.Set.Name))
							continue
						}

						switch arrayType := value[0].(type) {
						case int:
							actionDef.Set.Value = cast.ToIntSlice(value)
						case string:
							actionDef.Set.Value = cast.ToStringSlice(value)
						default:
							errs = multierror.Append(errs, fmt.Errorf("unsupported item type '%s' for array '%s'", reflect.TypeOf(arrayType), actionDef.Set.Name))
							continue
						}
					}

					variableValue = actionDef.Set.Value
				} else if actionDef.Set.Field != "" {
					kind, err := rs.eventCtor().GetFieldType(actionDef.Set.Field)
					if err != nil {
						errs = multierror.Append(errs, fmt.Errorf("failed to get field '%s': %w", actionDef.Set.Field, err))
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
						errs = multierror.Append(errs, fmt.Errorf("unsupported field type '%s' for variable '%s'", kind, actionDef.Set.Name))
						continue
					}
				}

				var variable eval.VariableValue
				var variableProvider VariableProvider

				if actionDef.Set.Scope != "" {
					stateScopeBuilder := rs.opts.StateScopes[actionDef.Set.Scope]
					if stateScopeBuilder == nil {
						errs = multierror.Append(errs, fmt.Errorf("invalid scope '%s'", actionDef.Set.Scope))
						continue
					}

					if _, found := rs.scopedVariables[actionDef.Set.Scope]; !found {
						rs.scopedVariables[actionDef.Set.Scope] = stateScopeBuilder()
					}

					variableProvider = rs.scopedVariables[actionDef.Set.Scope]
				} else {
					variableProvider = &rs.globalVariables
				}

				opts := eval.VariableOpts{TTL: actionDef.Set.TTL, Size: actionDef.Set.Size}

				variable, err := variableProvider.GetVariable(actionDef.Set.Name, variableValue, opts)
				if err != nil {
					errs = multierror.Append(errs, fmt.Errorf("invalid type '%s' for variable '%s': %w", reflect.TypeOf(actionDef.Set.Value), actionDef.Set.Name, err))
					continue
				}

				if existingVariable := rs.evalOpts.VariableStore.Get(varName); existingVariable != nil && reflect.TypeOf(variable) != reflect.TypeOf(existingVariable) {
					errs = multierror.Append(errs, fmt.Errorf("conflicting types for variable '%s'", varName))
					continue
				}

				rs.evalOpts.VariableStore.Add(varName, variable)
			}

			rule.Actions = append(rule.Actions, &Action{
				Def: actionDef,
			})
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
	eventType, err := rule.GetEventType()
	if err != nil {
		return "", err
	}

	if eventType == "" {
		return "", ErrRuleWithoutEvent
	}

	return eventType, nil
}

func (rs *RuleSet) isActionAvailable(eventType eval.EventType, action *Action) bool {
	if action.Def.Name() == HashAction && eventType != model.FileOpenEventType.String() && eventType != model.ExecEventType.String() {
		return false
	}
	return true
}

// AddRule creates the rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(parsingContext *ast.ParsingContext, pRule *PolicyRule) (*eval.Rule, error) {
	if pRule.Def.Disabled {
		return nil, nil
	}

	for _, id := range rs.opts.ReservedRuleIDs {
		if id == pRule.Def.ID {
			return nil, &ErrRuleLoad{Rule: pRule, Err: ErrInternalIDConflict}
		}
	}

	if _, exists := rs.rules[pRule.Def.ID]; exists {
		return nil, &ErrRuleLoad{Rule: pRule, Err: ErrDefinitionIDConflict}
	}

	var tags []string
	for k, v := range pRule.Def.Tags {
		tags = append(tags, k+":"+v)
	}

	rule := &Rule{
		PolicyRule: pRule,
		Rule:       eval.NewRule(pRule.Def.ID, pRule.Def.Expression, rs.evalOpts, tags...),
	}

	if err := rule.Parse(parsingContext); err != nil {
		return nil, &ErrRuleLoad{Rule: pRule, Err: &ErrRuleSyntax{Err: err}}
	}

	if err := rule.GenEvaluator(rs.model, parsingContext); err != nil {
		return nil, &ErrRuleLoad{Rule: pRule, Err: err}
	}

	eventType, err := GetRuleEventType(rule.Rule)
	if err != nil {
		return nil, &ErrRuleLoad{Rule: pRule, Err: err}
	}

	// validate event context against event type
	for _, field := range rule.GetFields() {
		restrictions := rs.model.GetFieldRestrictions(field)
		if len(restrictions) > 0 && !slices.Contains(restrictions, eventType) {
			return nil, &ErrRuleLoad{Rule: pRule, Err: &ErrFieldNotAvailable{Field: field, EventType: eventType, RestrictedTo: restrictions}}
		}
	}

	// ignore event types not supported
	if _, exists := rs.opts.EventTypeEnabled["*"]; !exists {
		if enabled, exists := rs.opts.EventTypeEnabled[eventType]; !exists || !enabled {
			return nil, &ErrRuleLoad{Rule: pRule, Err: ErrEventTypeNotEnabled}
		}
	}

	for _, action := range rule.PolicyRule.Actions {
		if !rs.isActionAvailable(eventType, action) {
			return nil, &ErrRuleLoad{Rule: pRule, Err: &ErrActionNotAvailable{ActionName: action.Def.Name(), EventType: eventType}}
		}

		// compile action filter
		if action.Def.Filter != nil {
			if err := action.CompileFilter(parsingContext, rs.model, rs.evalOpts); err != nil {
				return nil, &ErrRuleLoad{Rule: pRule, Err: err}
			}
		}

		if action.Def.Set != nil && action.Def.Set.Field != "" {
			if _, found := rs.fieldEvaluators[action.Def.Set.Field]; !found {
				evaluator, err := rs.model.GetEvaluator(action.Def.Set.Field, "")
				if err != nil {
					return nil, err
				}
				rs.fieldEvaluators[action.Def.Set.Field] = evaluator
			}
		}
	}

	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		bucket = &RuleBucket{}
		rs.eventRuleBuckets[eventType] = bucket
	}

	if err := bucket.AddRule(rule); err != nil {
		return nil, err
	}

	// Merge the fields of the new rule with the existing list of fields of the ruleset
	rs.AddFields(rule.GetEvaluator().GetFields())

	rs.rules[pRule.Def.ID] = rule

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

		eventTypeApprovers, err := rs.GetEventTypeApprovers(eventType, caps)
		if err != nil || len(eventTypeApprovers) == 0 {
			continue
		}
		approvers[eventType] = eventTypeApprovers
	}

	return approvers, nil
}

// GetEventTypeApprovers returns approvers for the given event type and the fields
func (rs *RuleSet) GetEventTypeApprovers(eventType eval.EventType, fieldCaps FieldCapabilities) (Approvers, error) {
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return nil, ErrNoEventTypeBucket{EventType: eventType}
	}

	return getApprovers(bucket.rules, rs.newFakeEvent(), fieldCaps)
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
	var isDiscarder bool

	for _, rule := range rules {
		// ignore rule that can't generate discarders
		if rule.NoDiscarder {
			continue
		}

		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || isTrue {
			return false, err
		}

		isDiscarder = true
	}
	return isDiscarder, nil
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

func (rs *RuleSet) runSetActions(_ eval.Event, ctx *eval.Context, rule *Rule) error {
	for _, action := range rule.PolicyRule.Actions {
		if !action.IsAccepted(ctx) {
			continue
		}

		switch {
		// other actions are handled by ruleset listeners
		case action.Def.Set != nil:
			name := string(action.Def.Set.Scope)
			if name != "" {
				name += "."
			}
			name += action.Def.Set.Name

			variable := rs.evalOpts.VariableStore.Get(name)
			if variable == nil {
				return fmt.Errorf("unknown variable: %s", name)
			}

			if mutable, ok := variable.(eval.MutableVariable); ok {
				value := action.Def.Set.Value
				if field := action.Def.Set.Field; field != "" {
					if evaluator := rs.fieldEvaluators[field]; evaluator != nil {
						value = evaluator.Eval(ctx)
					}
				}

				if action.Def.Set.Append {
					if err := mutable.Append(ctx, value); err != nil {
						return fmt.Errorf("append is not supported for %s", reflect.TypeOf(value))
					}
				} else {
					if err := mutable.Set(ctx, value); err != nil {
						return err
					}
				}
			}

			if rs.opts.ruleActionPerformedCb != nil {
				rs.opts.ruleActionPerformedCb(rule, action.Def)
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
		utils.PprofDoWithoutContext(rule.GetPprofLabels(), func() {
			if rule.GetEvaluator().Eval(ctx) {

				if rs.logger.IsTracing() {
					rs.logger.Tracef("Rule `%s` matches with event `%s`\n", rule.ID, event)
				}

				if err := rs.runSetActions(event, ctx, rule); err != nil {
					rs.logger.Errorf("Error while executing Set actions: %s", err)
				}

				rs.NotifyRuleMatch(rule, event)
				result = true
			}
		})
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

	var mdiscsToCheck []*multiDiscarderCheck

	for _, field := range bucket.fields {
		if check := rs.getValidMultiDiscarder(field); check != nil {
			value, err := event.GetFieldValue(field)
			if err != nil {
				rs.logger.Debugf("Failed to get field value for %s: %s", field, err)
				continue
			}

			// currently only support string values
			if valueStr, ok := value.(string); ok {
				check.value = valueStr
				mdiscsToCheck = append(mdiscsToCheck, check)
			}
		}

		if rs.opts.SupportedDiscarders != nil {
			if _, exists := rs.opts.SupportedDiscarders[field]; !exists {
				continue
			}
		}

		if isDiscarder, _ := IsDiscarder(ctx, field, bucket.rules); isDiscarder {
			rs.NotifyDiscarderFound(event, field, eventType)
		}
	}

	for _, check := range mdiscsToCheck {
		isMultiDiscarder := true
		for _, entry := range check.mdisc.Entries {
			bucket := rs.eventRuleBuckets[entry.EventType.String()]
			if bucket == nil || len(bucket.rules) == 0 {
				continue
			}

			dctx, err := buildDiscarderCtx(entry.EventType, entry.Field, check.value)
			if err != nil {
				rs.logger.Errorf("failed to build discarder context: %v", err)
				isMultiDiscarder = false
				break
			}

			if isDiscarder, _ := IsDiscarder(dctx, entry.Field, bucket.rules); !isDiscarder {
				isMultiDiscarder = false
				break
			}
		}

		if isMultiDiscarder {
			rs.NotifyDiscarderFound(event, check.mdisc.FinalField, check.mdisc.FinalEventType.String())
		}
	}
}

func (rs *RuleSet) getValidMultiDiscarder(field string) *multiDiscarderCheck {
	for _, mdisc := range rs.opts.SupportedMultiDiscarders {
		for _, entry := range mdisc.Entries {
			if entry.Field == field {
				return &multiDiscarderCheck{
					mdisc: mdisc,
				}
			}
		}
	}

	return nil
}

type multiDiscarderCheck struct {
	mdisc *MultiDiscarder
	value string
}

func buildDiscarderCtx(eventType model.EventType, field string, value interface{}) (*eval.Context, error) {
	ev := model.NewFakeEvent()
	ev.BaseEvent.Type = uint32(eventType)
	if err := ev.SetFieldValue(field, value); err != nil {
		return nil, err
	}
	return eval.NewContext(ev), nil
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

// LoadPolicies loads policies from the provided policy loader
func (rs *RuleSet) LoadPolicies(loader *PolicyLoader, opts PolicyLoaderOpts) *multierror.Error {
	var (
		errs       *multierror.Error
		allRules   []*PolicyRule
		allMacros  []*PolicyMacro
		macroIndex = make(map[string]*PolicyMacro)
		rulesIndex = make(map[string]*PolicyRule)
	)

	parsingContext := ast.NewParsingContext(false)

	policies, err := loader.LoadPolicies(opts)
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	rs.policies = policies

	for _, policy := range policies {
		for _, macro := range policy.GetAcceptedMacros() {
			if existingMacro := macroIndex[macro.Def.ID]; existingMacro != nil {
				if err := existingMacro.MergeWith(macro); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				macroIndex[macro.Def.ID] = macro
				allMacros = append(allMacros, macro)
			}
		}

		for _, rule := range policy.GetAcceptedRules() {
			if existingRule := rulesIndex[rule.Def.ID]; existingRule != nil {
				if err := existingRule.MergeWith(rule); err != nil {
					errs = multierror.Append(errs, err)
				}
			} else {
				rulesIndex[rule.Def.ID] = rule
				allRules = append(allRules, rule)
			}
		}

		rs.OnDemandHookPoints = append(rs.OnDemandHookPoints, policy.onDemandHookPoints...)
	}

	if err := rs.AddMacros(parsingContext, allMacros); err.ErrorOrNil() != nil {
		errs = multierror.Append(errs, err)
	}

	if err := rs.PopulateFieldsWithRuleActionsData(allRules, opts); err.ErrorOrNil() != nil {
		errs = multierror.Append(errs, err)
	}

	if err := rs.AddRules(parsingContext, allRules); err.ErrorOrNil() != nil {
		errs = multierror.Append(errs, err)
	}

	return errs
}

// NewEvent returns a new event using the embedded constructor
func (rs *RuleSet) NewEvent() eval.Event {
	return rs.eventCtor()
}

// SetFakeEventCtor sets the fake event constructor to the provided callback
func (rs *RuleSet) SetFakeEventCtor(fakeEventCtor func() eval.Event) {
	rs.fakeEventCtor = fakeEventCtor
}

// newFakeEvent returns a new event using the embedded constructor for fake events
func (rs *RuleSet) newFakeEvent() eval.Event {
	if rs.fakeEventCtor != nil {
		return rs.fakeEventCtor()
	}

	return model.NewFakeEvent()
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
