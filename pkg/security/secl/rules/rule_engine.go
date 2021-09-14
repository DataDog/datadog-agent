// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
)

// RuleEngine evaluates events against a set of policy and profiles
type RuleEngine struct {
	logger    Logger
	opts      *Opts
	policy    *RuleSet
	profiles  map[*eval.Rule]*RuleSet
	pool      *eval.ContextPool
	listeners []RuleEngineListener
	model     eval.Model
	eventCtor func() eval.Event
}

// RuleEngineListener describes the methods implemented by an object used to be
// notified of events on a rule engine.
type RuleEngineListener interface {
	RuleMatch(rule *Rule, event eval.Event)
	EventDiscarderFound(re *RuleEngine, event eval.Event, field eval.Field, eventType eval.EventType)
}

// GetProfile returns the profile that applies to an event
func (re *RuleEngine) GetProfile(ctx *eval.Context) *RuleSet {
	for selector, profile := range re.profiles {
		if selector.Eval(ctx) {
			return profile
		}
	}
	return nil
}

// NotifyRuleMatch notifies all the rule engine listeners that an event matched a rule
func (re *RuleEngine) NotifyRuleMatch(rule *Rule, event eval.Event) {
	for _, listener := range re.listeners {
		listener.RuleMatch(rule, event)
	}
}

// NotifyDiscarderFound notifies all the rule engine listeners that a discarder was found for an event
func (re *RuleEngine) NotifyDiscarderFound(event eval.Event, field eval.Field, eventType eval.EventType) {
	for _, listener := range re.listeners {
		listener.EventDiscarderFound(re, event, field, eventType)
	}
}

// Evaluate the specified event against the set of rules
func (re *RuleEngine) Evaluate(event eval.Event) bool {
	ctx := re.pool.Get(event.GetPointer())
	defer re.pool.Put(ctx)

	bucket := RuleBucket{}
	eventType := event.GetType()
	var policyRules []*Rule
	var profileRules []*Rule

	profileMatch := true
	if profile := re.GetProfile(ctx); profile != nil {
		profileMatch = profile.Evaluate(ctx, event, nil)

		if profileMatch {
			profileBucket := profile.GetBucket(eventType)
			bucket.fields = append(bucket.fields, profileBucket.fields...)
			profileRules = append(profileRules, profileBucket.rules...)
		}
	}

	policyMatch := false
	if policyBucket := re.policy.GetBucket(eventType); policyBucket != nil {
		bucket.fields = append(bucket.fields, policyBucket.fields...)
		policyMatch = re.policy.Evaluate(ctx, event, func(rule *Rule, event eval.Event) {
			re.logger.Debugf("Event %s matched profile: %+v", event.GetType(), event)
			re.NotifyRuleMatch(rule, event)
		})
		if policyMatch {
			policyRules = append(profileRules, policyBucket.rules...)
		}
	}

	if profileMatch && !policyMatch {
		re.logger.Debugf("Looking for discarders for event of type `%s`", eventType)

		for _, field := range bucket.fields {
			isDiscarder, _ := re.isDiscarder(ctx, field, policyRules, profileRules)
			if isDiscarder {
				value, _ := event.GetFieldValue(field)
				re.logger.Debugf("Found discarder for %s: %v", field, value)
				re.NotifyDiscarderFound(event, field, eventType)
			}
		}
	}

	return policyMatch || !profileMatch
}

// GetBucket returns rule bucket for the given event type
func (re *RuleEngine) GetBucket(eventType eval.EventType) (*RuleBucket, bool) {
	bucket, exists := &RuleBucket{}, false

	if policyBucket := re.policy.GetBucket(eventType); policyBucket != nil {
		bucket.Merge(policyBucket)
		exists = true
	}

	for _, profile := range re.profiles {
		if profileBucket := profile.GetBucket(eventType); profileBucket != nil {
			bucket.Merge(profileBucket)
			exists = true
		}
	}

	return bucket, exists
}

// AddListener adds a listener on the rule engine
func (re *RuleEngine) AddListener(listener RuleEngineListener) {
	re.listeners = append(re.listeners, listener)
}

// GetApprovers returns all approvers
func (re *RuleEngine) GetApprovers(fieldCaps map[eval.EventType]FieldCapabilities) (map[eval.EventType]Approvers, error) {
	approvers := make(map[eval.EventType]Approvers)
	for _, eventType := range re.GetEventTypes() {
		caps, exists := fieldCaps[eventType]
		if !exists {
			continue
		}

		eventApprovers, err := re.GetEventApprovers(eventType, caps)
		if err != nil {
			continue
		}
		approvers[eventType] = eventApprovers
	}

	return approvers, nil
}

// GetEventTypes returns all the event types handled by the rule engine
func (re *RuleEngine) GetEventTypes() []eval.EventType {
	eventTypesMap := make(map[string]bool)
	for eventType := range re.policy.eventRuleBuckets {
		eventTypesMap[eventType] = true
	}

	for _, profile := range re.profiles {
		for eventType := range profile.eventRuleBuckets {
			eventTypesMap[eventType] = true
		}
	}

	i := 0
	eventTypes := make([]eval.EventType, len(eventTypesMap))
	for eventType := range eventTypesMap {
		eventTypes[i] = eventType
		i++
	}

	return eventTypes
}

// GetEventApprovers returns approvers for the given event type and the fields
func (re *RuleEngine) GetEventApprovers(eventType eval.EventType, fieldCaps FieldCapabilities) (Approvers, error) {
	event := re.eventCtor()

	for _, profile := range re.profiles {
		if profileBucket := profile.GetBucket(eventType); profileBucket != nil {
			return nil, nil
		}
	}

	policyBucket := re.policy.GetBucket(eventType)
	if policyBucket != nil {
		return policyBucket.GetApprovers(event, fieldCaps, true)
	}

	return nil, nil
}

// HasRulesForEventType returns if there is at least one rule for the given event type
func (re *RuleEngine) HasRulesForEventType(eventType eval.EventType) bool {
	if re.policy.HasRulesForEventType(eventType) {
		return true
	}

	for _, profile := range re.profiles {
		if profile.HasRulesForEventType(eventType) {
			return true
		}
	}

	return false
}

func (re *RuleEngine) isDiscarder(ctx *eval.Context, field eval.Field, policyRules, profileRules []*Rule) (bool, error) {
	if re.opts.SupportedDiscarders != nil {
		if _, exists := re.opts.SupportedDiscarders[field]; !exists {
			return false, nil
		}
	}

	for _, rule := range policyRules {
		// s'il y a une seule chance que cela soit vrai avec la valeur actuelle de 'field'
		// ce n'est pas un discarder
		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || isTrue {
			return false, err
		}
	}

	for _, rule := range profileRules {
		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || !isTrue {
			return false, err
		}
	}

	return true, nil
}

// IsDiscarder partially evaluates an Event against a field
func (re *RuleEngine) IsDiscarder(event eval.Event, field eval.Field) (bool, error) {
	eventType, err := event.GetFieldEventType(field)
	if err != nil {
		return false, err
	}

	bucket, exists := re.GetBucket(eventType)
	if !exists {
		return false, &ErrNoEventTypeBucket{EventType: eventType}
	}

	ctx := re.pool.Get(event.GetPointer())
	defer re.pool.Put(ctx)

	for _, rule := range bucket.rules {
		isTrue, err := rule.PartialEval(ctx, field)
		if err != nil || isTrue {
			return false, err
		}
	}
	return true, nil
}

func (re *RuleEngine) newRuleSet() *RuleSet {
	return NewRuleSet(re.model, re.eventCtor, re.opts)
}

// LoadProfiles loads all the profiles in a folder
func (re *RuleEngine) LoadProfiles(profilesDir string) error {
	var result *multierror.Error

	profileFiles, err := ioutil.ReadDir(profilesDir)
	if err != nil {
		return multierror.Append(result, ErrProfileLoad{Name: profilesDir, Err: err})
	}
	sort.Slice(profileFiles, func(i, j int) bool { return profileFiles[i].Name() < profileFiles[j].Name() })

	// Load and parse profiles
PROFILE:
	for _, profilePath := range profileFiles {
		filename := profilePath.Name()
		ruleSet := re.newRuleSet()

		// profile path extension check
		if filepath.Ext(filename) != ".profile" {
			ruleSet.logger.Debugf("ignoring file `%s` wrong extension `%s`", profilePath.Name(), filepath.Ext(filename))
			continue PROFILE
		}

		// Open profile path
		f, err := os.Open(filepath.Join(profilesDir, filename))
		if err != nil {
			result = multierror.Append(result, &ErrPolicyLoad{Name: filename, Err: err})
			continue PROFILE
		}
		defer f.Close()

		// Parse profile file
		re.opts.Logger.Debugf("Loading profile %s", filename)
		profile, err := LoadProfile(f, filepath.Base(filename))
		if err != nil {
			result = multierror.Append(result, err)
			continue PROFILE
		}

		if profile.Selector == "" {
			result = multierror.Append(fmt.Errorf("profile %s has no selector", filename))
			continue PROFILE
		}

		selectorRule := &eval.Rule{
			ID:         filename,
			Expression: profile.Selector,
		}
		if err := selectorRule.Parse(); err != nil {
			result = multierror.Append(result, err)
			continue PROFILE
		}

		if err := selectorRule.GenEvaluator(ruleSet.model, &ruleSet.opts.Opts); err != nil {
			result = multierror.Append(result, err)
			continue PROFILE
		}

		for _, rule := range profile.Rules {
			rule.Expression = profile.Selector + " && " + rule.Expression
		}

		macros, rules, mErr := profile.GetValidMacroAndRules()
		if mErr.ErrorOrNil() != nil {
			result = multierror.Append(result, mErr)
		}

		if len(macros) > 0 {
			// Add the macros to the ruleset and generate macros evaluators
			if err := ruleSet.AddMacros(macros); err != nil {
				result = multierror.Append(result, err)
			}
		}

		// Add rules to the ruleset and generate rules evaluators
		if err := ruleSet.AddRules(rules); err.ErrorOrNil() != nil {
			result = multierror.Append(result, err)
		}

		re.profiles[selectorRule] = ruleSet
	}

	return result
}

// Load both policies and profiles in a folder into the rule engine
func (re *RuleEngine) Load(directory string) *multierror.Error {
	var result *multierror.Error

	if err := LoadPolicies(directory, re.policy); err != nil {
		result = multierror.Append(result, err)
	}

	if err := re.LoadProfiles(directory); err != nil {
		result = multierror.Append(result, err)
	}

	return result
}

// GetPolicy returns the rule engine policy rules
func (re *RuleEngine) GetPolicy() *RuleSet {
	return re.policy
}

// NewRuleEngine returns a new rule engine for the specified data model
func NewRuleEngine(model eval.Model, eventCtor func() eval.Event, opts *Opts) *RuleEngine {
	var logger Logger

	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = &NullLogger{}
	}

	return &RuleEngine{
		model:     model,
		eventCtor: eventCtor,
		opts:      opts,
		pool:      eval.NewContextPool(),
		logger:    logger,
		policy:    NewRuleSet(model, eventCtor, opts),
		profiles:  make(map[*eval.Rule]*RuleSet),
	}
}
