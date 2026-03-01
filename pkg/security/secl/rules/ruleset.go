// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cast"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/utils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
)

var (
	arrayIndexRE = regexp.MustCompile(`^(.+)\[(\d+)\]$`)
)

// parseArrayFieldAccess parses a field like "open.file.hashes[0]" and returns the base field and index
func parseArrayFieldAccess(field string) (baseField string, index int, isArray bool) {
	matches := arrayIndexRE.FindStringSubmatch(field)
	if len(matches) == 3 {
		baseField = matches[1]
		index, _ = strconv.Atoi(matches[2])
		return baseField, index, true
	}
	return field, 0, false
}

// Rule presents a rule in a ruleset
type Rule struct {
	*PolicyRule
	*eval.Rule
}

// DiscarderInvalidReport is a report of an invalid discarder
type DiscarderInvalidReport struct {
	RuleID eval.RuleID `json:"rule_id"`
	Field  eval.Field  `json:"field"`
}

// DiscardersReport is a report of the discarders in the ruleset
type DiscardersReport struct {
	Supported []eval.Field             `json:"supported"`
	Invalid   []DiscarderInvalidReport `json:"invalid"`
}

// RuleSetListener describes the methods implemented by an object used to be
// notified of events on a rule set.
type RuleSetListener interface {
	RuleMatch(ctx *eval.Context, rule *Rule, event eval.Event) bool
	EventDiscarderFound(rs *RuleSet, event eval.Event, field eval.Field, eventType eval.EventType)
}

// RuleSet holds a list of rules, grouped in bucket. An event can be evaluated
// against it. If the rule matches, the listeners for this rule set are notified
type RuleSet struct {
	opts             *Opts
	evalOpts         *eval.Opts
	eventRuleBuckets map[eval.EventType]*RuleBucket
	policies         atomic.Value
	fieldEvaluators  map[string]eval.Evaluator
	model            eval.Model
	eventCtor        func() eval.Event
	fakeEventCtor    func() eval.Event
	listenersLock    sync.RWMutex
	listeners        []RuleSetListener
	globalVariables  *eval.Variables
	scopedVariables  map[Scope]VariableProvider
	parsingContext   *ast.ParsingContext

	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields []eval.Field
	logger log.Logger
	pool   *eval.ContextPool

	// event collector, used for tests
	eventCollector EventCollector
}

// ListRuleIDs returns the list of RuleIDs from the ruleset
func (rs *RuleSet) ListRuleIDs() []RuleID {
	var ids []string
	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.rules {
			ids = append(ids, rule.Def.ID)
		}
	}
	return ids
}

// GetRules returns the active rules
func (rs *RuleSet) GetRules() []*Rule {
	var rules []*Rule

	eventTypes := make([]eval.EventType, 0, len(rs.eventRuleBuckets))

	for eventType := range rs.eventRuleBuckets {
		eventTypes = append(eventTypes, eventType)
	}

	slices.SortStableFunc(eventTypes, func(a, b eval.EventType) int {
		return strings.Compare(a, b)
	})

	for _, eventType := range eventTypes {
		bucket := rs.eventRuleBuckets[eventType]
		rules = append(rules, bucket.rules...)
	}
	return rules
}

// GetRuleByID returns the rule with the given ID
func (rs *RuleSet) GetRuleByID(id eval.RuleID) *Rule {
	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.rules {
			if rule.Def.ID == id {
				return rule
			}
		}
	}
	return nil
}

// GetRuleMap returns the active rules as a map by ID
func (rs *RuleSet) GetRuleMap() map[eval.RuleID]*Rule {
	ruleMap := make(map[eval.RuleID]*Rule)
	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.rules {
			ruleMap[rule.Def.ID] = rule
		}
	}
	return ruleMap
}

// GetRuleBucket returns the rule bucket for the given event type
func (rs *RuleSet) GetRuleBucket(eventType eval.EventType) *RuleBucket {
	return rs.eventRuleBuckets[eventType]
}

// GetPolicies returns the policies loaded in the rule set
func (rs *RuleSet) GetPolicies() []*Policy {
	value := rs.policies.Load()
	if value == nil {
		return nil
	}
	return value.([]*Policy)
}

// GetOnDemandHookPoints gets the on-demand hook points
func (rs *RuleSet) GetOnDemandHookPoints() ([]OnDemandHookPoint, error) {
	onDemandBucket := rs.GetBucket(model.OnDemandEventType.String())
	if onDemandBucket == nil {
		return nil, nil
	}

	var hookPoints []OnDemandHookPoint

	for _, rule := range onDemandBucket.rules {
		hooks := rule.GetFieldValues("ondemand.name")
		if len(hooks) != 1 {
			return nil, fmt.Errorf("invalid number of hooks for rule %s: %d", rule.ID, len(hooks))
		}
		hook := hooks[0]
		if hook.Type != eval.ScalarValueType {
			return nil, fmt.Errorf("invalid hook type for rule %s: %s, expected scalar", rule.ID, hook.Type)
		}
		hookName, ok := hook.Value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid hook value for rule %s: %s, expected string", rule.ID, hook.Value)
		}
		isSyscall := false

		probeType, probeName, found := strings.Cut(hookName, ":")
		if found {
			if probeType == "syscall" {
				isSyscall = true
				hookName = probeName
			} else {
				return nil, fmt.Errorf("invalid hook type for rule %s: %s, expected syscall or nothing", rule.ID, probeType)
			}
		}

		var args []HookPointArg
		for _, field := range rule.GetFields() {
			if !strings.HasPrefix(field, "ondemand.arg") {
				continue
			}

			_, argPart, found := strings.Cut(field, ".")
			if !found {
				return nil, fmt.Errorf("invalid hook argument field %s", field)
			}

			argN, kind, found := strings.Cut(argPart, ".")
			if !found {
				return nil, fmt.Errorf("invalid hook argument field %s", field)
			}

			switch kind {
			case "str":
				kind = "null-terminated-string"
			}

			n, err := strconv.Atoi(strings.TrimPrefix(argN, "arg"))
			if err != nil {
				return nil, fmt.Errorf("invalid hook argument field %s: %w", field, err)
			}

			args = append(args, HookPointArg{
				N:    n,
				Kind: kind,
			})
		}

		hookPoints = append(hookPoints, OnDemandHookPoint{
			Name:      hookName,
			IsSyscall: isSyscall,
			Args:      args,
		})
	}

	return sanitizeHookPoints(hookPoints)
}

func sanitizeHookPoints(hookPoints []OnDemandHookPoint) ([]OnDemandHookPoint, error) {
	type pair struct {
		name    string
		syscall bool
	}

	mapping := make(map[pair]map[int]string)
	for _, hook := range hookPoints {
		key := pair{name: hook.Name, syscall: hook.IsSyscall}
		if _, ok := mapping[key]; !ok {
			mapping[key] = make(map[int]string)
		}

		for _, arg := range hook.Args {
			if old, ok := mapping[key][arg.N]; ok && old != arg.Kind {
				return nil, fmt.Errorf("conflicting argument %d for hook %s: %s != %s", arg.N, hook.Name, old, arg.Kind)
			}
			mapping[key][arg.N] = arg.Kind
		}
	}

	var result []OnDemandHookPoint

	for key, args := range mapping {
		hp := OnDemandHookPoint{
			Name:      key.name,
			IsSyscall: key.syscall,
			Args:      make([]HookPointArg, 0, len(args)),
		}
		for n, kind := range args {
			hp.Args = append(hp.Args, HookPointArg{
				N:    n,
				Kind: kind,
			})
		}
		result = append(result, hp)
	}

	return result, nil
}

// ListMacroIDs returns the list of MacroIDs from the ruleset
func (rs *RuleSet) ListMacroIDs() []MacroID {
	var ids []string
	for _, macro := range rs.evalOpts.MacroStore.List() {
		ids = append(ids, macro.ID)
	}
	return ids
}

// GetVariables returns the variables store
func (rs *RuleSet) GetVariables() map[string]eval.SECLVariable {
	if rs.evalOpts == nil || rs.evalOpts.VariableStore == nil {
		return nil
	}
	return rs.evalOpts.VariableStore.Variables
}

// GetVariableStore returns the variable store
func (rs *RuleSet) GetVariableStore() *eval.VariableStore {
	if rs.evalOpts == nil {
		return nil
	}
	return rs.evalOpts.VariableStore
}

// AddMacros parses the macros AST and adds them to the list of macros of the ruleset
func (rs *RuleSet) AddMacros(macros []*PolicyMacro) *multierror.Error {
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
func (rs *RuleSet) AddMacro(pMacro *PolicyMacro) (*eval.Macro, error) {
	var err error

	if rs.evalOpts.MacroStore.Contains(pMacro.Def.ID) {
		return nil, nil
	}

	var macro *eval.Macro

	switch {
	case pMacro.Def.Expression != "" && len(pMacro.Def.Values) > 0:
		return nil, &ErrMacroLoad{Macro: pMacro, Err: errors.New("only one of 'expression' and 'values' can be defined")}
	case pMacro.Def.Expression != "":
		if strings.Contains(pMacro.Def.Expression, "fim.write.file.") {
			return nil, &ErrMacroLoad{Macro: pMacro, Err: errors.New("macro expression cannot contain 'fim.write.file.' event types")}
		}

		if macro, err = eval.NewMacro(pMacro.Def.ID, pMacro.Def.Expression, rs.model, rs.parsingContext, rs.evalOpts); err != nil {
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
func (rs *RuleSet) AddRules(pRules []*PolicyRule) *multierror.Error {
	var result *multierror.Error

	for _, pRule := range pRules {
		if _, err := rs.AddRule(pRule); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

// GetDiscardersReport returns a discarders state report
func (rs *RuleSet) GetDiscardersReport() (*DiscardersReport, error) {
	var report DiscardersReport

	event := rs.NewFakeEvent()
	ctx := eval.NewContext(event)

	errFieldNotFound := &eval.ErrFieldNotFound{}

	for field := range rs.opts.SupportedDiscarders {
		eventType, _, _, _, err := event.GetFieldMetadata(field)
		if err != nil {
			return nil, err
		}

		bucket := rs.GetRuleBucket(eventType)
		if bucket == nil {
			continue
		}

		_, rule, err := rs.IsDiscarder(ctx, field, bucket.GetRules())
		if err != nil {
			if errors.As(err, &errFieldNotFound) {
				report.Invalid = append(report.Invalid, DiscarderInvalidReport{
					RuleID: rule.ID,
					Field:  field,
				})
			}
		} else {
			report.Supported = append(report.Supported, field)
		}
	}

	return &report, nil
}

// PopulateFieldsWithRuleActionsData populates the fields with the data from the rule actions
func (rs *RuleSet) PopulateFieldsWithRuleActionsData(policyRules []*PolicyRule, opts PolicyLoaderOpts) *multierror.Error {
	var errs *multierror.Error

	for _, rule := range policyRules {
		for _, actionDef := range rule.Def.Actions {
			if err := actionDef.PreCheck(opts); err != nil {
				errs = multierror.Append(errs, fmt.Errorf("skipping invalid action in rule %s: %w", rule.Def.ID, err))
				continue
			}

			switch {
			case actionDef.Set != nil:
				varName := actionDef.Set.Name
				if !validators.CheckRuleID(varName) {
					errs = multierror.Append(errs, fmt.Errorf("invalid variable name '%s'", varName))
					continue
				}
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

				var variableValue = actionDef.Set.DefaultValue
				if variableValue == nil {
					variableValue = actionDef.Set.Value
				}

				if variableValue != nil {
					switch value := variableValue.(type) {
					case int:
						if actionDef.Set.Append {
							variableValue = []int{value}
						}
					case string:
						if actionDef.Set.Append {
							variableValue = []string{value}
						}
					case []interface{}:
						if len(value) == 0 {
							errs = multierror.Append(errs, fmt.Errorf("unable to infer item type for '%s'", actionDef.Set.Name))
							continue
						}

						switch arrayType := value[0].(type) {
						case int:
							variableValue = cast.ToIntSlice(value)
						case string:
							variableValue = cast.ToStringSlice(value)
						default:
							errs = multierror.Append(errs, fmt.Errorf("unsupported item type '%s' for array '%s'", reflect.TypeOf(arrayType), actionDef.Set.Name))
							continue
						}

						actionDef.Set.Value = variableValue
					}

					if actionDef.Set.Field != "" {
						// Check if this is an array access like "field[index]"
						baseField, _, isArrayAccess := parseArrayFieldAccess(actionDef.Set.Field)
						fieldToValidate := actionDef.Set.Field
						if isArrayAccess {
							fieldToValidate = baseField
						}

						_, kind, _, fieldIsArray, err := rs.eventCtor().GetFieldMetadata(fieldToValidate)
						if err != nil {
							errs = multierror.Append(errs, fmt.Errorf("failed to get field '%s': %w", fieldToValidate, err))
							continue
						}

						// If accessing array by index, the field is not an array from the variable's perspective
						if isArrayAccess {
							fieldIsArray = false
						}

						var valueIsArray bool
						variableValueKind := reflect.TypeOf(variableValue).Kind()
						if variableValueKind == reflect.Slice {
							variableValueKind = reflect.TypeOf(variableValue).Elem().Kind()
							valueIsArray = true
						}
						if variableValueKind != kind {
							errs = multierror.Append(errs, fmt.Errorf("value and field have different types for variable '%s' (%s != %s)", actionDef.Set.Name, variableValueKind.String(), kind.String()))
							continue
						}

						if fieldIsArray != valueIsArray && !actionDef.Set.Append {
							errs = multierror.Append(errs, fmt.Errorf("value and field cardinality mismatch for variable '%s': field '%s' is an array, but append is not set for variable '%s' with value '%v'", actionDef.Set.Name, actionDef.Set.Field, actionDef.Set.Name, variableValue))
							continue
						}
					}
				} else if actionDef.Set.Field != "" {
					// Check if this is an array access like "field[index]"
					baseField, _, isArrayAccess := parseArrayFieldAccess(actionDef.Set.Field)
					fieldToValidate := actionDef.Set.Field
					if isArrayAccess {
						fieldToValidate = baseField
					}

					_, kind, goType, isArray, err := rs.eventCtor().GetFieldMetadata(fieldToValidate)
					if err != nil {
						errs = multierror.Append(errs, fmt.Errorf("failed to get field '%s': %w", fieldToValidate, err))
						continue
					}

					// If accessing array by index, validate that the base field is actually an array
					if isArrayAccess {
						if !isArray {
							errs = multierror.Append(errs, fmt.Errorf("field '%s' is not an array, cannot use index access", baseField))
							continue
						}
						// When accessing by index, we treat it as a scalar value (no further validation needed)
					} else {
						// Check if the field is an array and append is not set
						if isArray && !actionDef.Set.Append {
							errs = multierror.Append(errs, fmt.Errorf("field '%s' is an array and can only be used with 'append: yes' in set action for variable '%s'", actionDef.Set.Field, actionDef.Set.Name))
							continue
						}
					}

					switch kind {
					case reflect.String:
						if actionDef.Set.Append {
							variableValue = []string{}
						} else {
							variableValue = ""
						}
					case reflect.Int:
						if actionDef.Set.Append {
							variableValue = []int{}
						} else {
							variableValue = 0
						}
					case reflect.Bool:
						variableValue = false
					case reflect.Struct:
						if goType == "net.IPNet" {
							variableValue = []net.IPNet{}
							break
						}
						fallthrough
					default:
						errs = multierror.Append(errs, fmt.Errorf("unsupported field type '%s (%s)' for variable '%s'", kind, goType, actionDef.Set.Name))
						continue
					}

					if defaultValueKind := reflect.TypeOf(actionDef.Set.DefaultValue); actionDef.Set.DefaultValue != nil && defaultValueKind != nil && defaultValueKind.Kind() != kind {
						errs = multierror.Append(errs, fmt.Errorf("value and default_value have different types for variable '%s' (%s != %s)", kind, defaultValueKind, actionDef.Set.Name))
						continue
					}
				}

				var variable eval.SECLVariable
				var variableProvider VariableProvider

				if actionDef.Set.Scope != "" {
					if _, found := rs.scopedVariables[actionDef.Set.Scope]; !found {
						stateScopeBuilder := rs.opts.StateScopes[actionDef.Set.Scope]
						if stateScopeBuilder == nil {
							errs = multierror.Append(errs, fmt.Errorf("invalid scope '%s'", actionDef.Set.Scope))
							continue
						}

						rs.scopedVariables[actionDef.Set.Scope] = stateScopeBuilder()
					}

					variableProvider = rs.scopedVariables[actionDef.Set.Scope]
				} else {
					variableProvider = rs.globalVariables
				}

				opts := eval.VariableOpts{
					TTL:       actionDef.Set.TTL.GetDuration(),
					Size:      actionDef.Set.Size,
					Private:   actionDef.Set.Private,
					Inherited: actionDef.Set.Inherited,
					Telemetry: rs.evalOpts.Telemetry,
				}

				variable, err := variableProvider.NewSECLVariable(actionDef.Set.Name, variableValue, string(actionDef.Set.Scope), opts)
				if err != nil {
					errs = multierror.Append(errs, fmt.Errorf("invalid type '%s' for variable '%s' (%+v): %w", reflect.TypeOf(variableValue), actionDef.Set.Name, actionDef.Set, err))
					continue
				}

				if existingVariable := rs.evalOpts.VariableStore.Get(varName); existingVariable != nil && reflect.TypeOf(variable) != reflect.TypeOf(existingVariable) {
					errs = multierror.Append(errs, fmt.Errorf("conflicting types for variable '%s': %s != %s", varName, reflect.TypeOf(variable), reflect.TypeOf(existingVariable)))
					continue
				}

				if existingVariable := rs.evalOpts.VariableStore.Get(varName); existingVariable != nil && existingVariable.GetVariableOpts().Private != variable.GetVariableOpts().Private {
					errs = multierror.Append(errs, fmt.Errorf("conflicting private flag for variable '%s'", varName))
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

// WithExcludedRuleFromDiscarders set excluded rule from discarders
func (rs *RuleSet) WithExcludedRuleFromDiscarders(excludedRuleFromDiscarders map[eval.RuleID]bool) {
	rs.opts.ExcludedRuleFromDiscarders = excludedRuleFromDiscarders
}

// AddRule creates the rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(pRule *PolicyRule) (model.EventCategory, error) {
	if pRule.Def.Disabled {
		return model.UnknownCategory, nil
	}

	if slices.Contains(rs.opts.ReservedRuleIDs, pRule.Def.ID) {
		return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: ErrInternalIDConflict}
	}

	if slices.Contains(rs.ListRuleIDs(), pRule.Def.ID) {
		return model.UnknownCategory, nil
	}

	var tags []string
	for k, v := range pRule.Def.Tags {
		tags = append(tags, k+":"+v)
	}
	tags = append(tags, pRule.Def.ProductTags...)

	expandedRules := expandFim(pRule.Def.ID, pRule.Def.GroupID, pRule.Def.Expression)

	categories := make([]model.EventCategory, 0)
	for _, er := range expandedRules {
		category, err := rs.innerAddExpandedRule(pRule, er, tags)
		if err != nil {
			return model.UnknownCategory, err
		}
		categories = append(categories, category)
	}
	categories = slices.Compact(categories)
	if len(categories) != 1 {
		return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: ErrMultipleEventCategories}
	}
	return categories[0], nil
}

func (rs *RuleSet) innerAddExpandedRule(pRule *PolicyRule, exRule expandedRule, tags []string) (model.EventCategory, error) {
	evalRule, err := eval.NewRule(exRule.id, exRule.expr, rs.parsingContext, rs.evalOpts, tags...)
	if err != nil {
		return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: &ErrRuleSyntax{Err: err}}
	}

	rule := &Rule{
		PolicyRule: pRule,
		Rule:       evalRule,
	}

	if err := rule.GenEvaluator(rs.model); err != nil {
		return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: err}
	}

	// call an extra layer of validation
	if err := evalRule.Model.ValidateRule(evalRule); err != nil {
		return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: &ErrRuleSyntax{Err: err}}
	}

	eventType, err := GetRuleEventType(rule.Rule)
	if err != nil {
		return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: err}
	}

	// validate event context against event type
	for _, field := range rule.GetFields() {
		restrictions := rs.model.GetFieldRestrictions(field)
		if len(restrictions) > 0 && !slices.Contains(restrictions, eventType) {
			return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: &ErrFieldNotAvailable{Field: field, EventType: eventType, RestrictedTo: restrictions}}
		}
	}

	// ignore event types not supported
	if _, exists := rs.opts.EventTypeEnabled["*"]; !exists {
		if enabled, exists := rs.opts.EventTypeEnabled[eventType]; !exists || !enabled {
			return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: ErrEventTypeNotEnabled}
		}

		// ignore rules requiring an unsupported event type to execute their action
		if err = pRule.AreActionsSupported(rs.opts.EventTypeEnabled); err != nil {
			return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: err}
		}
	}

	for _, action := range rule.PolicyRule.Actions {
		if action.Def.Filter != nil {
			// compile action filter
			if err := action.CompileFilter(rs.parsingContext, rs.model, rs.evalOpts); err != nil {
				return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: err}
			}
		}

		switch {

		case action.Def.Set != nil:
			// compile scope field
			if len(action.Def.Set.ScopeField) > 0 {
				if err := action.CompileScopeField(rs.model); err != nil {
					return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: err}
				}
			}

			if field := action.Def.Set.Field; field != "" {
				ev := model.NewFakeEvent()
				ruleEventType, err := rule.GetEventType()
				if err != nil {
					return model.UnknownCategory, fmt.Errorf("failed to compile action expression: %w", err)
				}

				// Check if this is an array access like "field[index]"
				baseField, _, isArrayAccess := parseArrayFieldAccess(field)

				// Validate using the base field if this is an array access
				fieldToValidate := field
				if isArrayAccess {
					fieldToValidate = baseField
				}

				fieldEventType, _, _, _, err := ev.GetFieldMetadata(fieldToValidate)
				if err != nil {
					return model.UnknownCategory, fmt.Errorf("failed to get event type for field '%s': %w", fieldToValidate, err)
				}
				if fieldEventType != "" && fieldEventType != ruleEventType {
					return model.UnknownCategory, fmt.Errorf("field '%s' with event type `%s` is not compatible with '%s' rules", field, fieldEventType, ruleEventType)
				}

				// Map legacy field before calling GetEvaluator
				mappedField := field
				if rs.evalOpts.LegacyFields != nil {
					if newField, found := rs.evalOpts.LegacyFields[field]; found {
						mappedField = newField
					}
				}

				if _, found := rs.fieldEvaluators[field]; !found {
					// GetEvaluator now handles array index access automatically
					evaluator, err := rs.model.GetEvaluator(mappedField, "", 0)
					if err != nil {
						return model.UnknownCategory, err
					}
					rs.fieldEvaluators[field] = evaluator
				}
			} else if expression := action.Def.Set.Expression; expression != "" {
				astRule, err := rs.parsingContext.ParseExpression(expression)
				if err != nil {
					return model.UnknownCategory, fmt.Errorf("failed to parse action expression: %w", err)
				}

				ruleEventType, err := rule.GetEventType()
				if err != nil {
					return model.UnknownCategory, fmt.Errorf("failed to compile action expression: %w", err)
				}

				state := eval.NewState(rs.model, "", rs.evalOpts.MacroStore)

				evaluator, _, err := eval.NodeToEvaluator(astRule, rs.evalOpts, state)
				if err != nil {
					return model.UnknownCategory, fmt.Errorf("failed to compile action expression: %w", err)
				}

				exprEventType, err := eval.EventTypeFromState(rs.model, state)
				if err != nil {
					return model.UnknownCategory, fmt.Errorf("failed to compile action expression: %w", err)
				}

				if exprEventType != "" && exprEventType != ruleEventType {
					return model.UnknownCategory, fmt.Errorf("expression '%s' with event type `%s` is not compatible with '%s' rules", expression, exprEventType, ruleEventType)
				}

				// validate that the expression type matches the default_value type
				if err := checkTypeCompatibility(action.Def.Set.DefaultValue, evaluator, action.Def.Set.Append); err != nil {
					return model.UnknownCategory, fmt.Errorf("expression '%s' for variable '%s': %w", expression, action.Def.Set.Name, err)
				}

				rs.fieldEvaluators[expression] = evaluator.(eval.Evaluator)
			}

		case action.Def.Hash != nil:
			if err := action.Def.Hash.PostCheck(evalRule); err != nil {
				return model.UnknownCategory, &ErrRuleLoad{Rule: pRule, Err: err}
			}
		}
	}

	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		bucket = &RuleBucket{}
		rs.eventRuleBuckets[eventType] = bucket
	}

	if err := bucket.AddRule(rule); err != nil {
		return model.UnknownCategory, err
	}

	// Merge the fields of the new rule with the existing list of fields of the ruleset
	rs.AddFields(rule.GetEvaluator().GetFields())

	return model.GetEventTypeCategory(eventType), nil
}

// checkTypeCompatibility validates that the evaluator's return type is compatible with the default value type.
func checkTypeCompatibility(defaultValue interface{}, evaluator interface{}, isAppend bool) error {
	if defaultValue == nil {
		return nil
	}

	// Get the evaluator's output type from its Value/Values field
	t := reflect.TypeOf(evaluator).Elem()
	var exprType reflect.Type
	if field, ok := t.FieldByName("Value"); ok {
		exprType = field.Type
	} else if field, ok := t.FieldByName("Values"); ok {
		exprType = field.Type
	} else {
		return fmt.Errorf("unable to determine type for evaluator %s", t.Name())
	}

	defaultType := reflect.TypeOf(defaultValue)
	exprKind, defaultKind := exprType.Kind(), defaultType.Kind()

	// Helper to get the actual element kind from a slice, inferring from values if element type is interface{}
	getSliceElemKind := func(defaultVal interface{}, defaultTyp reflect.Type) reflect.Kind {
		elemKind := defaultTyp.Elem().Kind()
		if elemKind == reflect.Interface {
			// Infer from actual values in the slice
			v := reflect.ValueOf(defaultVal)
			if v.Len() > 0 {
				elemKind = reflect.TypeOf(v.Index(0).Interface()).Kind()
			}
		}
		return elemKind
	}

	// When append is true and default_value is a slice, compare against the element type
	if isAppend && defaultKind == reflect.Slice && exprKind != reflect.Slice {
		elemKind := getSliceElemKind(defaultValue, defaultType)
		if elemKind != exprKind {
			return fmt.Errorf("incompatible types: expression returns %s but default_value element type is %s", exprKind, elemKind)
		}
	} else if defaultKind != exprKind {
		return fmt.Errorf("incompatible types: expression returns %s but default_value is %s", exprKind, defaultKind)
	} else if exprKind == reflect.Slice {
		defaultElemKind := getSliceElemKind(defaultValue, defaultType)
		exprElemKind := exprType.Elem().Kind()
		if defaultElemKind != exprElemKind {
			return fmt.Errorf("incompatible slice element types: expression returns %s but default_value element type is %s", exprElemKind, defaultElemKind)
		}
	}

	return nil
}

// NotifyRuleMatch notifies all the ruleset listeners that an event matched a rule
func (rs *RuleSet) NotifyRuleMatch(ctx *eval.Context, rule *Rule, event eval.Event) {
	rs.listenersLock.RLock()
	defer rs.listenersLock.RUnlock()

	for _, listener := range rs.listeners {
		if !listener.RuleMatch(ctx, rule, event) {
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
func (rs *RuleSet) GetApprovers(fieldCaps map[eval.EventType]FieldCapabilities) (map[eval.EventType]Approvers, *ApproverStats, []*Rule, error) {
	var (
		approvers        = make(map[eval.EventType]Approvers)
		noDiscarderRules []*Rule
		stats            = NewApproverStats()
	)

	for _, eventType := range rs.GetEventTypes() {
		caps, exists := fieldCaps[eventType]
		if !exists {
			continue
		}

		evtApprovers, evtStats, evtNoDiscarderRules, err := rs.GetEventTypeApprovers(eventType, caps)
		stats.Merge(evtStats)

		if err != nil || len(evtApprovers) == 0 {
			continue
		}

		approvers[eventType] = evtApprovers
		noDiscarderRules = append(noDiscarderRules, evtNoDiscarderRules...)
	}

	return approvers, stats, noDiscarderRules, nil
}

// GetEventTypeApprovers returns approvers for the given event type and the fields
func (rs *RuleSet) GetEventTypeApprovers(eventType eval.EventType, fieldCaps FieldCapabilities) (Approvers, *ApproverStats, []*Rule, error) {
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return nil, nil, nil, ErrNoEventTypeBucket{EventType: eventType}
	}

	// all the rules needs to be of the same type
	return getApprovers(bucket.rules, rs.NewFakeEvent(), fieldCaps)
}

// GetFieldValues returns all the values of the given field
func (rs *RuleSet) GetFieldValues(field eval.Field) []eval.FieldValue {
	var values []eval.FieldValue

	for _, bucket := range rs.eventRuleBuckets {
		for _, rule := range bucket.rules {
			rv := rule.GetFieldValues(field)
			if len(rv) > 0 {
				values = append(values, rv...)
			}
		}
	}

	return values
}

// IsDiscarder partially evaluates an Event against a field
func (rs *RuleSet) IsDiscarder(ctx *eval.Context, field eval.Field, rules []*Rule) (bool, *Rule, error) {
	var isDiscarder bool

	for _, rule := range rules {
		// ignore rule that can't generate discarders
		if _, exists := rs.opts.ExcludedRuleFromDiscarders[rule.ID]; exists {
			continue
		}

		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || isTrue {
			return false, rule, err
		}

		isDiscarder = true
	}
	return isDiscarder, nil, nil
}

func (rs *RuleSet) runSetActions(_ eval.Event, ctx *eval.Context, rule *Rule) error {
	for _, action := range rule.PolicyRule.Actions {
		// set context scope field evaluator
		ctx.SetScopeFieldEvaluator(action.ScopeFieldEvaluator)

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
				return fmt.Errorf("unknown variable `%s` in rule `%s`", name, rule.ID)
			}

			if mutable, ok := variable.(eval.MutableVariable); ok {
				value := action.Def.Set.Value
				if field := action.Def.Set.Field; field != "" {
					if evaluator := rs.fieldEvaluators[field]; evaluator != nil {
						value = evaluator.Eval(ctx)
					}
				} else if expression := action.Def.Set.Expression; expression != "" {
					if evaluator := rs.fieldEvaluators[expression]; evaluator != nil {
						value = evaluator.Eval(ctx)
					}
				}
				if action.Def.Set.Append {
					if err := mutable.Append(ctx, value); err != nil {
						return fmt.Errorf("append is not supported for type `%s` with variable `%s` in rule `%s`: %w", reflect.TypeOf(value), name, rule.ID, err)
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

		ctx.PerActionReset()
	}

	return nil
}

func (rs *RuleSet) runLogActions(_ eval.Event, ctx *eval.Context, rule *Rule) error {
	for _, action := range rule.PolicyRule.Actions {
		if !action.IsAccepted(ctx) {
			continue
		}

		switch {
		// other actions are handled by ruleset listeners
		case action.Def.Log != nil:
			message := action.Def.Log.Message
			if message == "" {
				message = fmt.Sprintf("Rule %s triggered", rule.ID)
			}

			switch strings.ToLower(action.Def.Log.Level) {
			case "debug":
				rs.logger.Debugf("%s", message)
			case "info":
				rs.logger.Infof("%s", message)
			case "warning":
				rs.logger.Warnf("%s", message)
			case "error":
				rs.logger.Errorf("%s", message)
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

	resetEventEvalCtx := func() {
		event.(*model.Event).RuleTags = nil
	}
	defer resetEventEvalCtx()

	for _, rule := range bucket.rules {
		utils.PprofDoWithoutContext(rule.GetPprofLabels(), func() {
			event := event.(*model.Event)
			event.RuleTags = rule.PolicyRule.Def.ProductTags

			if rule.GetEvaluator().Eval(ctx) {
				if rs.logger.IsTracing() {
					rs.logger.Tracef("Rule `%s` matches with event `%s`\n", rule.ID, event)
				}

				if err := rs.runSetActions(event, ctx, rule); err != nil {
					rs.logger.Errorf("Error while executing 'set' actions: %s", err)
				}

				if err := rs.runLogActions(event, ctx, rule); err != nil {
					rs.logger.Errorf("Error while executing 'log' actions: %s", err)
				}

				rs.NotifyRuleMatch(ctx, rule, event)
				result = true
			}
			ctx.PerEvalReset()
		})
	}

	// no-op in the general case, only used to collect events in functional tests
	// for debugging purposes
	rs.eventCollector.CollectEvent(rs, ctx, event, result)

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

		if _, exists := rs.opts.SupportedDiscarders[field]; !exists {
			continue
		}

		if isDiscarder, _, _ := rs.IsDiscarder(ctx, field, bucket.rules); isDiscarder {
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

			if isDiscarder, _, _ := rs.IsDiscarder(dctx, entry.Field, bucket.rules); !isDiscarder {
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
func (rs *RuleSet) LoadPolicies(loader *PolicyLoader, opts PolicyLoaderOpts) ([]*PolicyRule, *multierror.Error) {
	var (
		errs          *multierror.Error
		allRules      []*PolicyRule
		filteredRules []*PolicyRule
		allMacros     []*PolicyMacro
		macroIndex    = make(map[string]*PolicyMacro)
		rulesIndex    = make(map[string]*PolicyRule)
	)

	policies, err := loader.LoadPolicies(opts)
	if err != nil {
		errs = multierror.Append(errs, err)
	}
	rs.policies.Store(policies)

	for _, policy := range policies {
		if len(policy.Macros) == 0 && len(policy.Rules) == 0 && (policy.Info.Name != DefaultPolicyName && !policy.Info.IsInternal) {
			errs = multierror.Append(errs, &ErrPolicyLoad{
				Name:    policy.Info.Name,
				Type:    policy.Info.Type,
				Version: policy.Info.Version,
				Source:  policy.Info.Source,
				Err:     ErrPolicyIsEmpty,
			})
			continue
		}

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
				existingRule.UsedBy = append(existingRule.UsedBy, rule.Policy)
				existingRule.MergeWith(rule)
			} else {
				rulesIndex[rule.Def.ID] = rule
				allRules = append(allRules, rule)
			}
		}

		for _, rule := range policy.GetFilteredRules() {
			if existingRule := rulesIndex[rule.Def.ID]; existingRule != nil {
				// if the rule is already in the rules index, this means that a rule with the same ID was already accepted
				// in this case let's only report the version of the rule that was accepted
				continue
			}
			filteredRules = append(filteredRules, rule)
		}
	}

	if err := rs.AddMacros(allMacros); err.ErrorOrNil() != nil {
		errs = multierror.Append(errs, err)
	}

	if err := rs.PopulateFieldsWithRuleActionsData(allRules, opts); err.ErrorOrNil() != nil {
		errs = multierror.Append(errs, err)
	}

	if err := rs.AddRules(allRules); err.ErrorOrNil() != nil {
		errs = multierror.Append(errs, err)
	}

	return filteredRules, errs
}

// NewEvent returns a new event using the embedded constructor
func (rs *RuleSet) NewEvent() eval.Event {
	return rs.eventCtor()
}

// SetFakeEventCtor sets the fake event constructor to the provided callback
func (rs *RuleSet) SetFakeEventCtor(fakeEventCtor func() eval.Event) {
	rs.fakeEventCtor = fakeEventCtor
}

// NewFakeEvent returns a new event using the embedded constructor for fake events
func (rs *RuleSet) NewFakeEvent() eval.Event {
	if rs.fakeEventCtor != nil {
		return rs.fakeEventCtor()
	}

	return model.NewFakeEvent()
}

// CleanupExpiredVariables cleans up all epxired variables in the ruleset
func (rs *RuleSet) CleanupExpiredVariables() {
	if rs.globalVariables != nil {
		rs.globalVariables.CleanupExpiredVariables()
	}

	for _, variableProvider := range rs.scopedVariables {
		variableProvider.CleanupExpiredVariables()
	}
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

	// Set legacy fields mapping on the model if it has the method
	if setter, ok := model.(interface {
		SetLegacyFields(map[eval.Field]eval.Field)
	}); ok {
		setter.SetLegacyFields(evalOpts.LegacyFields)
	}

	return &RuleSet{
		model:            model,
		eventCtor:        eventCtor,
		opts:             opts,
		evalOpts:         evalOpts,
		eventRuleBuckets: make(map[eval.EventType]*RuleBucket),
		logger:           logger,
		pool:             eval.NewContextPool(),
		fieldEvaluators:  make(map[string]eval.Evaluator),
		scopedVariables:  make(map[Scope]VariableProvider),
		globalVariables:  eval.NewVariables(),
		parsingContext:   ast.NewParsingContext(opts.RuleCacheEnabled),
	}
}

// GetScopedVariables returns all scoped variables that match the given name and scope
func (rs *RuleSet) GetScopedVariables(scope Scope, name string) map[eval.ScopeHashKey]eval.Variable {
	variableProvider, found := rs.scopedVariables[scope]
	if !found {
		return nil
	}
	return variableProvider.GetScopedVariables(name)
}
