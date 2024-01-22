//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package monitor holds rules related files
package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// policyMetricRate defines how often the policy metric will be sent
	policyMetricRate = 30 * time.Second
)

// policy describes policy related information
type policy struct {
	name    string
	source  string
	version string
}

// ruleStatus defines status of rules
type ruleStatus = map[eval.RuleID]string

// PolicyMonitor defines a policy monitor
type PolicyMonitor struct {
	sync.RWMutex

	statsdClient         statsd.ClientInterface
	policies             []*policy
	rules                ruleStatus
	perRuleMetricEnabled bool
}

// SetPolicies sets the policies to monitor
func (pm *PolicyMonitor) SetPolicies(policies []*PolicyState) {
	pm.Lock()
	defer pm.Unlock()

	pm.policies = make([]*policy, 0, len(policies))
	if pm.perRuleMetricEnabled {
		pm.rules = make(ruleStatus)
	}

	for _, p := range policies {
		pm.policies = append(pm.policies, &policy{
			name:    p.Name,
			source:  p.Source,
			version: p.Version,
		})

		if pm.perRuleMetricEnabled {
			for _, rule := range p.Rules {
				pm.rules[eval.RuleID(rule.ID)] = rule.Status
			}
		}
	}
}

// ReportHeartbeatEvent sends HeartbeatEvents reporting the current set of policies
func (pm *PolicyMonitor) ReportHeartbeatEvent(sender events.EventSender) {
	pm.RLock()
	rule, events := newHeartbeatEvents(pm.policies)
	pm.RUnlock()

	for _, event := range events {
		sender.SendEvent(rule, event, nil, "")
	}
}

// Start the monitor
func (pm *PolicyMonitor) Start(ctx context.Context) {
	go func() {
		timerMetric := time.NewTicker(policyMetricRate)
		defer timerMetric.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-timerMetric.C:
				pm.RLock()
				for _, p := range pm.policies {
					tags := []string{
						"policy_name:" + p.name,
						"policy_source:" + p.source,
						"policy_version:" + p.version,
						"agent_version:" + version.AgentVersion,
					}

					if err := pm.statsdClient.Gauge(metrics.MetricPolicy, 1, tags, 1.0); err != nil {
						log.Error(fmt.Errorf("failed to send policy metric: %w", err))
					}
				}

				if pm.perRuleMetricEnabled {
					for id, status := range pm.rules {
						tags := []string{
							"rule_id:" + id,
							fmt.Sprintf("status:%v", status),
							constants.CardinalityTagPrefix + collectors.LowCardinalityString,
						}

						if err := pm.statsdClient.Gauge(metrics.MetricRulesStatus, 1, tags, 1.0); err != nil {
							log.Error(fmt.Errorf("failed to send policy metric: %w", err))
						}
					}
				}
				pm.RUnlock()
			}
		}
	}()
}

// NewPolicyMonitor returns a new Policy monitor
func NewPolicyMonitor(statsdClient statsd.ClientInterface, perRuleMetricEnabled bool) *PolicyMonitor {
	return &PolicyMonitor{
		statsdClient:         statsdClient,
		perRuleMetricEnabled: perRuleMetricEnabled,
	}
}

// RuleSetLoadedReport represents the rule and the custom event related to a RuleSetLoaded event, ready to be dispatched
type RuleSetLoadedReport struct {
	Rule  *rules.Rule
	Event *events.CustomEvent
}

// ReportRuleSetLoaded reports to Datadog that a new ruleset was loaded
func ReportRuleSetLoaded(sender events.EventSender, statsdClient statsd.ClientInterface, policies []*PolicyState) {
	rule, event := newRuleSetLoadedEvent(policies)

	if err := statsdClient.Count(metrics.MetricRuleSetLoaded, 1, []string{}, 1.0); err != nil {
		log.Error(fmt.Errorf("failed to send ruleset_loaded metric: %w", err))
	}

	sender.SendEvent(rule, event, nil, "")
}

// RuleState defines a loaded rule
// easyjson:json
type RuleState struct {
	ID         string            `json:"id"`
	Version    string            `json:"version,omitempty"`
	Expression string            `json:"expression"`
	Status     string            `json:"status"`
	Message    string            `json:"message,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Actions    []RuleAction      `json:"actions,omitempty"`
}

// PolicyState is used to report policy was loaded
// easyjson:json
type PolicyState struct {
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Source  string       `json:"source"`
	Rules   []*RuleState `json:"rules"`
}

// RuleAction is used to report policy was loaded
// easyjson:json
type RuleAction struct {
	Filter *string         `json:"filter,omitempty"`
	Set    *RuleSetAction  `json:"set,omitempty"`
	Kill   *RuleKillAction `json:"kill,omitempty"`
}

// RuleSetAction is used to report 'set' action
// easyjson:json
type RuleSetAction struct {
	Name   string      `json:"name,omitempty"`
	Value  interface{} `json:"value,omitempty"`
	Field  string      `json:"field,omitempty"`
	Append bool        `json:"append,omitempty"`
	Scope  string      `json:"scope,omitempty"`
}

// RuleKillAction is used to report the 'kill' action
// easyjson:json
type RuleKillAction struct {
	Signal string `json:"signal,omitempty"`
	Scope  string `json:"scope,omitempty"`
}

// RulesetLoadedEvent is used to report that a new ruleset was loaded
// easyjson:json
type RulesetLoadedEvent struct {
	events.CustomEventCommonFields
	Policies []*PolicyState `json:"policies"`
}

// ToJSON marshal using json format
func (e RulesetLoadedEvent) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// HeartbeatEvent is used to report the policies that has been loaded
// easyjson:json
type HeartbeatEvent struct {
	events.CustomEventCommonFields
	Policy *PolicyState `json:"policy"`
}

// ToJSON marshal using json format
func (e HeartbeatEvent) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// PolicyStateFromRuleDefinition returns a policy state based on the rule definition
func PolicyStateFromRuleDefinition(def *rules.RuleDefinition) *PolicyState {
	return &PolicyState{
		Name:    def.Policy.Name,
		Version: def.Policy.Version,
		Source:  def.Policy.Source,
	}
}

// RuleStateFromDefinition returns a rule state based on the rule definition
func RuleStateFromDefinition(def *rules.RuleDefinition, status string, message string) *RuleState {
	ruleState := &RuleState{
		ID:         def.ID,
		Version:    def.Version,
		Expression: def.Expression,
		Status:     status,
		Message:    message,
		Tags:       def.Tags,
	}

	for _, action := range def.Actions {
		ruleAction := RuleAction{Filter: action.Filter}
		switch {
		case action.Kill != nil:
			ruleAction.Kill = &RuleKillAction{
				Scope:  action.Kill.Scope,
				Signal: action.Kill.Signal,
			}
		case action.Set != nil:
			ruleAction.Set = &RuleSetAction{
				Name:   action.Set.Name,
				Value:  action.Set.Value,
				Field:  action.Set.Field,
				Append: action.Set.Append,
				Scope:  string(action.Set.Scope),
			}
		}
		ruleState.Actions = append(ruleState.Actions, ruleAction)
	}

	return ruleState
}

// NewPoliciesState returns the states of policies and rules
func NewPoliciesState(ruleSets map[string]*rules.RuleSet, err *multierror.Error, includeInternalPolicies bool) []*PolicyState {
	mp := make(map[string]*PolicyState)

	var policyState *PolicyState
	var exists bool

	for _, rs := range ruleSets {
		for _, rule := range rs.GetRules() {
			if rule.Definition.Policy.IsInternal && !includeInternalPolicies {
				continue
			}

			ruleDef := rule.Definition
			policyName := ruleDef.Policy.Name

			if policyState, exists = mp[policyName]; !exists {
				policyState = PolicyStateFromRuleDefinition(ruleDef)
				mp[policyName] = policyState
			}
			policyState.Rules = append(policyState.Rules, RuleStateFromDefinition(ruleDef, "loaded", ""))
		}
	}

	// rules ignored due to errors
	if err != nil && err.Errors != nil {
		for _, err := range err.Errors {
			if rerr, ok := err.(*rules.ErrRuleLoad); ok {
				if rerr.Definition.Policy.IsInternal && !includeInternalPolicies {
					continue
				}
				policyName := rerr.Definition.Policy.Name

				if _, exists := mp[policyName]; !exists {
					policyState = PolicyStateFromRuleDefinition(rerr.Definition)
					mp[policyName] = policyState
				} else {
					policyState = mp[policyName]
				}
				policyState.Rules = append(policyState.Rules, RuleStateFromDefinition(rerr.Definition, string(rerr.Type()), rerr.Err.Error()))
			}
		}
	}

	var policies []*PolicyState
	for _, policy := range mp {
		policies = append(policies, policy)
	}

	return policies
}

// newRuleSetLoadedEvent returns the rule (e.g. ruleset_loaded) and a populated custom event for a new_rules_loaded event
func newRuleSetLoadedEvent(policies []*PolicyState) (*rules.Rule, *events.CustomEvent) {
	evt := RulesetLoadedEvent{
		Policies: policies,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.RulesetLoadedRuleID, events.RulesetLoadedRuleDesc),
		events.NewCustomEvent(model.CustomRulesetLoadedEventType, evt)
}

// newHeartbeatEvents returns the rule (e.g. heartbeat) and a populated custom event for a heartbeat event
func newHeartbeatEvents(policies []*policy) (*rules.Rule, []*events.CustomEvent) {
	var evts []*events.CustomEvent

	for _, policy := range policies {
		var policyState = PolicyState{
			Name:    policy.name,
			Version: policy.version,
			Source:  policy.source,
			Rules:   nil, // The rules that have been loaded at startup are not reported in the heartbeat event
		}

		evt := HeartbeatEvent{
			Policy: &policyState,
		}
		evt.FillCustomEventCommonFields()
		evts = append(evts, events.NewCustomEvent(model.CustomHeartbeatEventType, evt))
	}

	return events.NewCustomRule(events.HeartbeatRuleID, events.HeartbeatRuleDesc),
		evts
}
