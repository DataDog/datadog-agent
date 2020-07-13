package eval

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/cihub/seelog"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/secl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type RuleParseError struct {
	pos  lexer.Position
	expr string
}

func (e *RuleParseError) Error() string {
	column := e.pos.Column
	if column > 0 {
		column--
	}

	str := fmt.Sprintf("%s\n", e.expr)
	str += strings.Repeat(" ", column)
	str += "^"
	return str
}

type RuleSetListener interface {
	RuleMatch(rule *Rule, event Event)
	EventDiscarderFound(event Event, field string)
}

type RuleSet struct {
	opts             Opts
	eventRuleBuckets map[EventType]*RuleBucket
	macros           map[policy.MacroID]*Macro
	rules            map[policy.RuleID]*Rule
	model            Model
	eventCtor        func() Event
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

func (rs *RuleSet) AddMacro(macroDef *policy.MacroDefinition) (*Macro, error) {
	if _, exists := rs.macros[macroDef.ID]; exists {
		return nil, fmt.Errorf("found multiple definition of the macro '%s'", macroDef.ID)
	}

	macro := &Macro{
		ID:         macroDef.ID,
		Expression: macroDef.Expression,
	}

	// Generate Macro AST
	if err := macro.Parse(); err != nil {
		return nil, errors.Wrapf(err, "couldn't generate a macro AST of the macro %s", macroDef.ID)
	}

	evaluator, err := MacroToEvaluator(macro.ast, rs.model, &rs.opts, "")
	if err != nil {
		if err, ok := err.(*AstToEvalError); ok {
			log.Errorf("macro syntax error: %s\n%s", err, secl.SprintExprAt(macro.Expression, err.Pos))
		} else {
			log.Errorf("macro parsing error: %s\n%s", err, macro.Expression)
		}
		return nil, err
	}

	// macro.evaluator = evaluator
	rs.macros[macro.ID] = macro
	rs.opts.Macros[macro.ID] = evaluator

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
func (rs *RuleSet) AddRule(ruleDef *policy.RuleDefinition) (*Rule, error) {
	if _, exists := rs.rules[ruleDef.ID]; exists {
		return nil, fmt.Errorf("found multiple definition of the rule '%s'", ruleDef.ID)
	}

	rule := &Rule{
		ID:         ruleDef.ID,
		Expression: ruleDef.Expression,
		Tags:       ruleDef.GetTags(),
	}

	// Generate ast
	if err := rule.Parse(); err != nil {
		return nil, err
	}

	// Generate rule evaluator
	evaluator, err := RuleToEvaluator(rule.ast, rs.model, &rs.opts)
	if err != nil {
		if err, ok := err.(*AstToEvalError); ok {
			return nil, errors.Wrap(&RuleParseError{pos: err.Pos, expr: rule.Expression}, "rule syntax error")
		}
		return nil, errors.Wrap(err, "rule compilation error")
	}
	rule.evaluator = evaluator

	for _, event := range evaluator.EventTypes {
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

// HasRulesForEventType returns if there is at least one rule for the given event type
func (rs *RuleSet) HasRulesForEventType(eventType string) bool {
	bucket, found := rs.eventRuleBuckets[eventType]
	if !found {
		return false
	}
	return len(bucket.rules) > 0
}

// GetApprovers returns Approvers for the given event type and the fields
func (rs *RuleSet) GetApprovers(eventType string, fieldCaps FieldCapabilities) (Approvers, error) {
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return nil, NoEventTypeBucket{EventType: eventType}
	}

	return bucket.GetApprovers(rs.model, rs.eventCtor(), fieldCaps)
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

// generatePartials - Generate the partials of the ruleset. A partial is a boolean evalution function that only depends
// on one field. The goal of partial is to determine if a rule depends on a specific field, so that we can decide if
// we should create an in-kernel filter for that field.
func (rs *RuleSet) generatePartials() error {
	// Compute the macros partials for each fields of the ruleset first.
	macroEvaluators := make(map[Field]map[policy.MacroID]*MacroEvaluator)

	for _, field := range rs.fields {
		for name, macro := range rs.macros {
			eval, err := MacroToEvaluator(macro.ast, rs.model, &rs.opts, field)
			if err != nil {
				if err, ok := err.(*AstToEvalError); ok {
					log.Errorf("macro syntax error: %s\n%s", err, secl.SprintExprAt(macro.Expression, err.Pos))
				} else {
					log.Errorf("macro parsing error: %s\n%s", err, macro.Expression)
				}
				return err
			}
			if _, exists := macroEvaluators[field]; !exists {
				macroEvaluators[field] = make(map[policy.MacroID]*MacroEvaluator)
			}
			macroEvaluators[field][name] = eval
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
				state := newState(rs.model, field, macroEvaluators[field])
				pEval, _, _, err := nodeToEvaluator(rule.ast.BooleanExpression, &rs.opts, state)
				if err != nil {
					return errors.Wrapf(err, "couldn't generate partial for field %s and rule %s", field, rule.ID)
				}

				pEvalBool, ok := pEval.(*BoolEvaluator)
				if !ok {
					return NewTypeError(rule.ast.Pos, reflect.Bool)
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
		macros:           make(map[string]*Macro),
		fields:           []string{},
	}
}
