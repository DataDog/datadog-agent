//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers $GOFILE

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

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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
func (pm *PolicyMonitor) ReportHeartbeatEvent(acc *events.AgentContainerContext, sender events.EventSender) {
	pm.RLock()
	rule, events := newHeartbeatEvents(acc, pm.policies)
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
							constants.CardinalityTagPrefix + types.LowCardinalityString,
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
func ReportRuleSetLoaded(acc *events.AgentContainerContext, sender events.EventSender, statsdClient statsd.ClientInterface, rs *rules.RuleSet, policies []*PolicyState, filterReport *kfilters.FilterReport) {
	rule, event := newRuleSetLoadedEvent(acc, rs, policies, filterReport)

	if err := statsdClient.Count(metrics.MetricRuleSetLoaded, 1, []string{}, 1.0); err != nil {
		log.Error(fmt.Errorf("failed to send ruleset_loaded metric: %w", err))
	}

	sender.SendEvent(rule, event, nil, "")
}

// RuleState defines a loaded rule
// easyjson:json
type RuleState struct {
	ID          string            `json:"id"`
	Version     string            `json:"version,omitempty"`
	Expression  string            `json:"expression"`
	Status      string            `json:"status"`
	Message     string            `json:"message,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	ProductTags []string          `json:"product_tags,omitempty"`
	Actions     []RuleAction      `json:"actions,omitempty"`
	ModifiedBy  []*PolicyState    `json:"modified_by,omitempty"`
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
	Filter   *string         `json:"filter,omitempty"`
	Set      *RuleSetAction  `json:"set,omitempty"`
	Kill     *RuleKillAction `json:"kill,omitempty"`
	Hash     *HashAction     `json:"hash,omitempty"`
	CoreDump *CoreDumpAction `json:"coredump,omitempty"`
	Log      *LogAction      `json:"log,omitempty"`
}

// HashAction is used to report 'hash' action
// easyjson:json
type HashAction struct {
	Enabled bool `json:"enabled,omitempty"`
}

// RuleSetAction is used to report 'set' action
// easyjson:json
type RuleSetAction struct {
	Name         string      `json:"name,omitempty"`
	Value        interface{} `json:"value,omitempty"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Field        string      `json:"field,omitempty"`
	Expression   string      `json:"expression,omitempty"`
	Append       bool        `json:"append,omitempty"`
	Scope        string      `json:"scope,omitempty"`
	ScopeField   string      `json:"scope_field,omitempty"`
	Size         int         `json:"size,omitempty"`
	TTL          string      `json:"ttl,omitempty"`
	Inherited    bool        `json:"inherited,omitempty"`
}

// RuleKillAction is used to report the 'kill' action
// easyjson:json
type RuleKillAction struct {
	Signal string `json:"signal,omitempty"`
	Scope  string `json:"scope,omitempty"`
}

// CoreDumpAction is used to report the 'coredump' action
// easyjson:json
type CoreDumpAction struct {
	Process       bool `json:"process,omitempty"`
	Mount         bool `json:"mount,omitempty"`
	Dentry        bool `json:"dentry,omitempty"`
	NoCompression bool `json:"no_compression,omitempty"`
}

// LogAction is used to report the 'log' action
// easyjson:json
type LogAction struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
}

// RulesetLoadedEvent is used to report that a new ruleset was loaded
// easyjson:json
type RulesetLoadedEvent struct {
	events.CustomEventCommonFields
	Policies       []*PolicyState         `json:"policies"`
	Filters        *kfilters.FilterReport `json:"filters,omitempty"`
	MonitoredFiles []string               `json:"monitored_files,omitempty"`
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

// PolicyStateFromPolicyInfo returns a policy state based on the policy info
func PolicyStateFromPolicyInfo(policyInfo *rules.PolicyInfo) *PolicyState {
	return &PolicyState{
		Name:    policyInfo.Name,
		Version: policyInfo.Version,
		Source:  policyInfo.Source,
	}
}

// RuleStateFromRule returns a rule state based on the given rule
func RuleStateFromRule(rule *rules.PolicyRule, policy *rules.PolicyInfo, status string, message string) *RuleState {
	ruleState := &RuleState{
		ID:          rule.Def.ID,
		Version:     rule.Policy.Version,
		Expression:  rule.Def.Expression,
		Status:      status,
		Message:     message,
		Tags:        rule.Def.Tags,
		ProductTags: rule.Def.ProductTags,
	}

	for _, action := range rule.Actions {
		ruleAction := RuleAction{Filter: action.Def.Filter}
		switch {
		case action.Def.Kill != nil:
			ruleAction.Kill = &RuleKillAction{
				Scope:  action.Def.Kill.Scope,
				Signal: action.Def.Kill.Signal,
			}
		case action.Def.Set != nil:
			ruleAction.Set = &RuleSetAction{
				Name:         action.Def.Set.Name,
				Value:        action.Def.Set.Value,
				DefaultValue: action.Def.Set.DefaultValue,
				Field:        action.Def.Set.Field,
				Expression:   action.Def.Set.Expression,
				Append:       action.Def.Set.Append,
				Scope:        string(action.Def.Set.Scope),
				Size:         action.Def.Set.Size,
				Inherited:    action.Def.Set.Inherited,
				ScopeField:   action.Def.Set.ScopeField,
			}
			if action.Def.Set.TTL != nil {
				ruleAction.Set.TTL = action.Def.Set.TTL.String()
			}
		case action.Def.Hash != nil:
			ruleAction.Hash = &HashAction{
				Enabled: true,
			}
		case action.Def.CoreDump != nil:
			ruleAction.CoreDump = &CoreDumpAction{
				Process:       action.Def.CoreDump.Process,
				Mount:         action.Def.CoreDump.Mount,
				Dentry:        action.Def.CoreDump.Dentry,
				NoCompression: action.Def.CoreDump.NoCompression,
			}
		case action.Def.Log != nil:
			ruleAction.Log = &LogAction{
				Level:   action.Def.Log.Level,
				Message: action.Def.Log.Message,
			}
		}
		ruleState.Actions = append(ruleState.Actions, ruleAction)
	}

	for _, pInfo := range rule.ModifiedBy {
		// The policy of an override rule is listed in both the UsedBy and ModifiedBy fields of the rule
		// In that case we want to avoid reporting the ModifiedBy field for the rule with the override field
		if policy.Equals(&pInfo) {
			continue
		}
		ruleState.ModifiedBy = append(ruleState.ModifiedBy, PolicyStateFromPolicyInfo(&pInfo))
	}

	return ruleState
}

// NewPoliciesState returns the states of policies and rules
func NewPoliciesState(rs *rules.RuleSet, err *multierror.Error, includeInternalPolicies bool) []*PolicyState {
	mp := make(map[string]*PolicyState)

	var policyState *PolicyState
	var exists bool

	for _, rule := range rs.GetRules() {
		for _, pInfo := range rule.UsedBy {
			if pInfo.IsInternal && !includeInternalPolicies {
				continue
			}

			if policyState, exists = mp[pInfo.Name]; !exists {
				policyState = PolicyStateFromPolicyInfo(&pInfo)
				mp[pInfo.Name] = policyState
			}
			policyState.Rules = append(policyState.Rules, RuleStateFromRule(rule.PolicyRule, &pInfo, "loaded", ""))
		}
	}

	// rules ignored due to errors
	if err != nil && err.Errors != nil {
		for _, err := range err.Errors {
			if rerr, ok := err.(*rules.ErrRuleLoad); ok {
				if rerr.Rule.Policy.IsInternal && !includeInternalPolicies {
					continue
				}
				policyName := rerr.Rule.Policy.Name

				if _, exists := mp[policyName]; !exists {
					policyState = PolicyStateFromPolicyInfo(&rerr.Rule.Policy)
					mp[policyName] = policyState
				} else {
					policyState = mp[policyName]
				}
				policyState.Rules = append(policyState.Rules, RuleStateFromRule(rerr.Rule, &rerr.Rule.Policy, string(rerr.Type()), rerr.Err.Error()))
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
func newRuleSetLoadedEvent(acc *events.AgentContainerContext, rs *rules.RuleSet, policies []*PolicyState, filterReport *kfilters.FilterReport) (*rules.Rule, *events.CustomEvent) {
	evt := RulesetLoadedEvent{
		Policies:       policies,
		Filters:        filterReport,
		MonitoredFiles: extractMonitoredFilesAndFolders(rs),
	}
	evt.FillCustomEventCommonFields(acc)

	return events.NewCustomRule(events.RulesetLoadedRuleID, events.RulesetLoadedRuleDesc),
		events.NewCustomEvent(model.CustomEventType, evt)
}

// newHeartbeatEvents returns the rule (e.g. heartbeat) and a populated custom event for a heartbeat event
func newHeartbeatEvents(acc *events.AgentContainerContext, policies []*policy) (*rules.Rule, []*events.CustomEvent) {
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
		evt.FillCustomEventCommonFields(acc)
		evts = append(evts, events.NewCustomEvent(model.CustomEventType, evt))
	}

	return events.NewCustomRule(events.HeartbeatRuleID, events.HeartbeatRuleDesc),
		evts
}

// extractMonitoredFilesAndFolders extracts file and folder paths from rule expressions
func extractMonitoredFilesAndFolders(rs *rules.RuleSet) []string {
	if rs == nil {
		return nil
	}

	pathsSet := make(map[string]bool)

	// Get FIM events
	fimEvents := model.GetEventTypePerCategory(model.FIMCategory)[model.FIMCategory]

	// Check both file.name and file.path for each FIM event
	for _, event := range fimEvents {
		for _, suffix := range []string{".file.name", ".file.path"} {
			field := event + suffix
			// Get all rules that use this field
			for _, rule := range rs.GetRules() {
				values := rule.GetFieldValues(field)
				for _, value := range values {
					path, ok := value.Value.(string)
					if !ok || path == "" {
						continue
					}

					// Check if this value is used positively in the rule expression (NO: not in or !=)
					if isPositivelyUsed(rule, field, value, rs) {
						pathsSet[path] = true
					}
				}
			}
		}
	}

	if len(pathsSet) == 0 {
		return nil
	}

	monitored := make([]string, 0, len(pathsSet))
	for path := range pathsSet {
		monitored = append(monitored, path)
	}

	return monitored
}

// isPositivelyUsed checks if a field value is used positively
func isPositivelyUsed(rule *rules.Rule, field eval.Field, value eval.FieldValue, rs *rules.RuleSet) bool {
	fakeEvent := rs.NewFakeEvent()
	ctx := eval.NewContext(fakeEvent)

	// Test with the actual value
	err := fakeEvent.SetFieldValue(field, value.Value)
	if err != nil {
		return false
	}

	origResult, err := rule.PartialEval(ctx, field)
	if err != nil {
		return false
	}

	// Test with a different value to see if the rule behavior changes
	notValue, err := eval.NotOfValue(value.Value)
	if err != nil {
		return false
	}

	err = fakeEvent.SetFieldValue(field, notValue)
	if err != nil {
		return false
	}

	notResult, err := rule.PartialEval(ctx, field)
	if err != nil {
		return false
	}

	// If the results are different, this means the field is used positively
	return origResult != notResult
}
