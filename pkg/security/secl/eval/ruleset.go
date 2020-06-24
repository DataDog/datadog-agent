package eval

import (
	"math/rand"
	"reflect"
	"sort"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/secl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type FilterValue struct {
	Field string
	Value interface{}
	Type  FieldValueType
	Not   bool

	ignore bool
}

type RuleSetListener interface {
	RuleMatch(rule *Rule, event Event)
	EventDiscarderFound(event Event, field string)
}

type RuleBucket struct {
	rules  []*Rule
	fields []string
}

func (rl *RuleBucket) AddRule(rule *Rule) error {
	for _, r := range rl.rules {
		if r.ID == rule.ID {
			return DuplicateRuleID{ID: r.ID}
		}
	}

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

	return nil
}

func (rl *RuleBucket) GetRules() []*Rule {
	return rl.rules
}

type RuleSet struct {
	opts             Opts
	eventRuleBuckets map[string]*RuleBucket
	macros           map[string]*Macro
	model            Model
	eventCtor        func() Event
	listeners        []RuleSetListener
	// fields holds the list of event field queries (like "process.uid") used by the entire set of rules
	fields []string
}

// generateMacroEvaluators - Generates the macros evaluators for the list of macros of the ruleset. If a field is provided,
// the function will compute the macro partials.
func (rs *RuleSet) generateMacroEvaluators(field string) error {
	macroEvaluators := make(map[string]*MacroEvaluator)
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
		macroEvaluators[name] = eval
	}
	rs.opts.SetMacroEvaluators(field, macroEvaluators)
	return nil
}

// AddMacros - Parse the macros AST and add them to the list of macros of the ruleset
func (rs *RuleSet) AddMacros(macros map[string]*policy.MacroDefinition) error {
	// Build the list of macros for the ruleset
	for id, m := range macros {
		macro := &Macro{
			ID:         m.ID,
			Expression: m.Expression,
		}
		// Generate Macro AST
		if err := macro.LoadAST(); err != nil {
			return errors.Wrapf(err, "couldn't generate a macro AST of the macro %s", id)
		}
		rs.macros[id] = macro
	}

	// Generate macro evaluators. The input "field" is therefore empty, we are not generating partials yet.
	if err := rs.generateMacroEvaluators(""); err != nil {
		return errors.Wrap(err, "couldn't generate macros evaluators")
	}
	return nil
}

// AddRules - Adds rules to the ruleset and generate their partials
func (rs *RuleSet) AddRules(rules map[string]*policy.RuleDefinition) error {
	for _, ruleDef := range rules {
		if _, err := rs.AddRule(ruleDef); err != nil {
			return errors.Wrapf(err, "couldn't add rule %s to the ruleset", ruleDef.ID)
		}
	}

	if err := rs.generatePartials(); err != nil {
		return errors.Wrap(err, "couldn't generate partials")
	}
	return nil
}

// AddRule - Creates the rule evaluator and adds it to the bucket of its events
func (rs *RuleSet) AddRule(ruleDef *policy.RuleDefinition) (*Rule, error) {
	rule := &Rule{
		ID:         ruleDef.ID,
		Expression: ruleDef.Expression,
		Tags:       ruleDef.GetTags(),
	}
	// Generate ast
	if err := rule.LoadAST(); err != nil {
		return nil, err
	}
	// Generate rule evaluator
	evaluator, err := RuleToEvaluator(rule.ast, rs.model, &rs.opts)
	if err != nil {
		if err, ok := err.(*AstToEvalError); ok {
			log.Errorf("rule syntax error: %s\n%s", err, secl.SprintExprAt(ruleDef.Expression, err.Pos))
		} else {
			log.Errorf("rule compilation error: %s\n%s", err, ruleDef.Expression)
		}
		return nil, err
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

// FilterValues - list of FilterValue
type FilterValues []FilterValue

// Approvers associates field names with their Filter values
type Approvers map[string]FilterValues

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
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
		return randStringRunes(256), nil
	case bool:
		return !v, nil
	}

	return nil, errors.New("value type unknown")
}

// FieldCombinations - array all the combinations of field
type FieldCombinations [][]string

func (a FieldCombinations) Len() int           { return len(a) }
func (a FieldCombinations) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a FieldCombinations) Less(i, j int) bool { return len(a[i]) < len(a[j]) }

func fieldCombinations(fields []string) FieldCombinations {
	var result FieldCombinations

	for i := 1; i < (1 << len(fields)); i++ {
		var subResult []string
		for j, field := range fields {
			if (i & (1 << j)) > 0 {
				subResult = append(subResult, field)
			}
		}
		result = append(result, subResult)
	}

	// order the list with the single field first
	sort.Sort(result)

	return result
}

// Merge merges to FilterValues ensuring there is no duplicate value
func (fv FilterValues) Merge(n FilterValues) FilterValues {
LOOP:
	for _, v1 := range n {
		for _, v2 := range fv {
			if v1.Value == v2.Value {
				continue LOOP
			}
		}
		fv = append(fv, v1)
	}

	return fv
}

type FieldCapabilities []FieldCapability

type FieldCapability struct {
	Field string
	Types FieldValueType
}

func (fcs FieldCapabilities) GetFields() []string {
	var fields []string
	for _, fc := range fcs {
		fields = append(fields, fc.Field)
	}
	return fields
}

func (fcs FieldCapabilities) Validate(approvers map[string]FilterValues) bool {
	for _, fc := range fcs {
		values, exists := approvers[fc.Field]
		if !exists {
			continue
		}

		for _, value := range values {
			if value.Type&fc.Types == 0 {
				return false
			}
		}
	}

	return true
}

// GetApprovers returns Approvers for the given event type and the fields
func (rs *RuleSet) GetApprovers(eventType string, fieldCaps FieldCapabilities) (Approvers, error) {
	bucket, exists := rs.eventRuleBuckets[eventType]
	if !exists {
		return nil, NoEventTypeBucket{EventType: eventType}
	}

	fcs := fieldCombinations(fieldCaps.GetFields())

	approvers := make(Approvers)
	for _, rule := range bucket.rules {
		truthTable, err := rs.genTruthTable(rule)
		if err != nil {
			return nil, err
		}

		var ruleApprovers map[string]FilterValues
		for _, fields := range fcs {
			ruleApprovers = truthTable.getApprovers(fields...)
			if ruleApprovers != nil && len(ruleApprovers) > 0 && fieldCaps.Validate(ruleApprovers) {
				break
			}
		}

		if ruleApprovers == nil || len(ruleApprovers) == 0 || !fieldCaps.Validate(ruleApprovers) {
			return nil, &NoApprover{Fields: fieldCaps.GetFields()}
		}
		for field, values := range ruleApprovers {
			approvers[field] = approvers[field].Merge(values)
		}
	}

	return approvers, nil
}

type truthEntry struct {
	Values FilterValues
	Result bool
}

type truthTable struct {
	Entries []truthEntry
}

func (tt *truthTable) getApprovers(fields ...string) map[string]FilterValues {
	filterValues := make(map[string]FilterValues)

	for _, entry := range tt.Entries {
		if !entry.Result {
			continue
		}

		// in order to have approvers we need to ensure that for a "true" result
		// we always have all the given field set to true. If we find a "true" result
		// with a field set to false we can exclude the given fields as approvers.
		allFalse := true
		for _, field := range fields {
			for _, value := range entry.Values {
				if value.Field == field && !value.Not {
					allFalse = false
					break
				}
			}
		}

		if allFalse {
			return nil
		}

		for _, field := range fields {
		LOOP:
			for _, value := range entry.Values {
				if !value.ignore && field == value.Field {
					fvs := filterValues[value.Field]
					for _, fv := range fvs {
						// do not append twice the same value
						if fv.Value == value.Value {
							continue LOOP
						}
					}
					fvs = append(fvs, value)
					filterValues[value.Field] = fvs
				}
			}
		}
	}

	return filterValues
}

func (rs *RuleSet) genTruthTable(rule *Rule) (*truthTable, error) {
	var ctx Context

	event := rs.eventCtor()
	rs.model.SetEvent(event)

	var filterValues []*FilterValue
	for field, fValues := range rule.evaluator.FieldValues {
		// case where there is no static value, ex: process.gid == process.uid
		// so generate fake value in order to be able to get the truth table
		if len(fValues) == 0 {
			var value interface{}

			kind, err := event.GetFieldType(field)
			if err != nil {
				return nil, err
			}
			switch kind {
			case reflect.String:
				value = ""
			case reflect.Int:
				value = 0
			case reflect.Bool:
				value = false
			default:
				return nil, &FieldTypeUnknown{Field: field}
			}

			filterValues = append(filterValues, &FilterValue{
				Field:  field,
				Value:  value,
				Type:   ScalarValueType,
				ignore: true,
			})

			continue
		}

		for _, fValue := range fValues {
			filterValues = append(filterValues, &FilterValue{
				Field: field,
				Value: fValue.Value,
				Type:  fValue.Type,
			})
		}
	}

	if len(filterValues) == 0 {
		return nil, nil
	}

	if len(filterValues) >= 64 {
		return nil, errors.New("limit of field values reached")
	}

	var truthTable truthTable
	for i := 0; i < (1 << len(filterValues)); i++ {
		var entry truthEntry

		for j, value := range filterValues {
			value.Not = (i & (1 << j)) > 0
			if value.Not {
				notValue, err := notOfValue(value.Value)
				if err != nil {
					return nil, &ValueTypeUnknown{Field: value.Field}
				}
				event.SetFieldValue(value.Field, notValue)
			} else {
				event.SetFieldValue(value.Field, value.Value)
			}

			entry.Values = append(entry.Values, FilterValue{
				Field: value.Field,
				Value: value.Value,
				Type:  value.Type,
				Not:   value.Not,
			})
		}
		entry.Result = rule.evaluator.Eval(&ctx)

		truthTable.Entries = append(truthTable.Entries, entry)
	}

	return &truthTable, nil
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
	for _, field := range rs.fields {
		if err := rs.generateMacroEvaluators(field); err != nil {
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

				state := newState(rs.model, field, rs.opts.GetMacroEvaluators(field))
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
