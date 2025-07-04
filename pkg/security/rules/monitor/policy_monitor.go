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

// ruleStatus defines status of rules
type ruleStatus = map[eval.RuleID]string

// PolicyMonitor defines a policy monitor
type PolicyMonitor struct {
	sync.RWMutex

	statsdClient         statsd.ClientInterface
	policies             []*PolicyState
	rules                ruleStatus
	perRuleMetricEnabled bool
}

// SetPolicies sets the policies to monitor
func (pm *PolicyMonitor) SetPolicies(policies []*PolicyState) {
	pm.Lock()
	defer pm.Unlock()

	if pm.perRuleMetricEnabled {
		pm.rules = make(ruleStatus)
	}

	for _, p := range policies {
		if pm.perRuleMetricEnabled {
			for _, rule := range p.Rules {
				pm.rules[eval.RuleID(rule.ID)] = rule.Status
			}
		}
		p.Rules = nil // Clear rules to avoid sending them in heartbeat events
	}

	pm.policies = policies
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
						"policy_name:" + p.Name,
						"policy_source:" + p.Source,
						"policy_version:" + p.Version,
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
func ReportRuleSetLoaded(bundle RulesetLoadedEventBundle, sender events.EventSender, statsdClient statsd.ClientInterface) {
	if err := statsdClient.Count(metrics.MetricRuleSetLoaded, 1, []string{}, 1.0); err != nil {
		log.Error(fmt.Errorf("failed to send ruleset_loaded metric: %w", err))
	}

	sender.SendEvent(bundle.Rule, bundle.Event, nil, "")
}

// PolicyMetadata contains the basic information about a policy
type PolicyMetadata struct {
	// Name is the name of the policy
	Name string `json:"name"`
	// Version is the version of the policy
	Version string `json:"version,omitempty"`
	// Source is the source of the policy
	Source string `json:"source"`
}

// RuleState defines a loaded rule
// easyjson:json
type RuleState struct {
	ID                     string            `json:"id"`
	Version                string            `json:"version,omitempty"`
	Expression             string            `json:"expression"`
	Status                 string            `json:"status"`
	Message                string            `json:"message,omitempty"`
	FilterType             string            `json:"filter_type,omitempty"`
	AgentVersionConstraint string            `json:"agent_version,omitempty"`
	Filters                []string          `json:"filters,omitempty"`
	Tags                   map[string]string `json:"tags,omitempty"`
	ProductTags            []string          `json:"product_tags,omitempty"`
	Actions                []RuleAction      `json:"actions,omitempty"`
	ModifiedBy             []*PolicyMetadata `json:"modified_by,omitempty"`
}

// PolicyStatus defines the status of a policy
type PolicyStatus string

const (
	// PolicyStatusLoaded indicates that the policy was loaded successfully
	PolicyStatusLoaded PolicyStatus = "loaded"
	// PolicyStatusPartiallyFiltered indicates that some rules in the policy were filtered out
	PolicyStatusPartiallyFiltered PolicyStatus = "partially_filtered"
	// PolicyStatusPartiallyLoaded indicates that some rules in the policy couldn't be loaded
	PolicyStatusPartiallyLoaded PolicyStatus = "partially_loaded"
	// PolicyStatusFullyRejected indicates that all rules in the policy couldn't be loaded
	PolicyStatusFullyRejected PolicyStatus = "fully_rejected"
	// PolicyStatusFullyFiltered indicates that all rules in the policy were filtered out
	PolicyStatusFullyFiltered PolicyStatus = "fully_filtered"
	// PolicyStatusError indicates that the policy was not loaded due to an error
	PolicyStatusError PolicyStatus = "error"
)

// PolicyState is used to report policy was loaded
// easyjson:json
type PolicyState struct {
	PolicyMetadata
	Status  PolicyStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Rules   []*RuleState `json:"rules,omitempty"`
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

// NewPolicyMetadata returns a new policy metadata object
func NewPolicyMetadata(name, source, version string) *PolicyMetadata {
	return &PolicyMetadata{
		Name:    name,
		Version: version,
		Source:  source,
	}
}

// NewPolicyState returns a policy state based on the policy info
func NewPolicyState(name, source, version string, status PolicyStatus, message string) *PolicyState {
	return &PolicyState{
		PolicyMetadata: *NewPolicyMetadata(name, source, version),
		Status:         status,
		Message:        message,
	}
}

// RuleStateFromRule returns a rule state based on the given rule
func RuleStateFromRule(rule *rules.PolicyRule, policy *rules.PolicyInfo, status string, message string) *RuleState {
	ruleState := &RuleState{
		ID:                     rule.Def.ID,
		Version:                rule.Policy.Version,
		Expression:             rule.Def.Expression,
		Status:                 status,
		Message:                message,
		Tags:                   rule.Def.Tags,
		ProductTags:            rule.Def.ProductTags,
		AgentVersionConstraint: rule.Def.AgentVersionConstraint,
		Filters:                rule.Def.Filters,
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
		ruleState.ModifiedBy = append(ruleState.ModifiedBy, NewPolicyMetadata(pInfo.Name, pInfo.Source, pInfo.Version))
	}

	if !rule.Accepted {
		ruleState.FilterType = string(rule.FilterType)
		switch rule.FilterType {
		case rules.FilterTypeRuleID:
			ruleState.Message = "rule ID was filtered out"
		case rules.FilterTypeAgentVersion:
			ruleState.Message = "this agent version doesn't support this rule"
		case rules.FilterTypeRuleFilter:
			ruleState.Message = "none of the rule filters matched the host or configuration of this agent"
		}
	}

	return ruleState
}

// NewPoliciesState returns the states of policies and rules
func NewPoliciesState(rs *rules.RuleSet, filteredRules []*rules.PolicyRule, err *multierror.Error, includeInternalPolicies bool) []*PolicyState {
	mp := make(map[string]*PolicyState)

	var policyState *PolicyState
	var exists bool

	for _, rule := range rs.GetRules() {
		for pInfo := range rule.Policies(includeInternalPolicies) {
			if policyState, exists = mp[pInfo.Name]; !exists {
				policyState = NewPolicyState(pInfo.Name, pInfo.Source, pInfo.Version, PolicyStatusLoaded, "")
				mp[pInfo.Name] = policyState
			}
			policyState.Rules = append(policyState.Rules, RuleStateFromRule(rule.PolicyRule, pInfo, "loaded", ""))
		}
	}

	// rules ignored due to errors
	if err != nil && err.Errors != nil {
		for _, err := range err.Errors {
			if rerr, ok := err.(*rules.ErrRuleLoad); ok {
				for pInfo := range rerr.Rule.Policies(includeInternalPolicies) {
					policyName := pInfo.Name
					if policyState, exists = mp[policyName]; !exists {
						// if the policy is not in the map, this means that no rule from this policy was loaded successfully
						policyState = NewPolicyState(pInfo.Name, pInfo.Source, pInfo.Version, PolicyStatusFullyRejected, "")
						mp[policyName] = policyState
					} else if policyState.Status == PolicyStatusLoaded {
						policyState.Status = PolicyStatusPartiallyLoaded
					}
					policyState.Rules = append(policyState.Rules, RuleStateFromRule(rerr.Rule, pInfo, string(rerr.Type()), rerr.Err.Error()))
				}
			} else if pErr, ok := err.(*rules.ErrPolicyLoad); ok {
				policyName := pErr.Name
				if policyState, exists = mp[policyName]; !exists {
					mp[policyName] = NewPolicyState(pErr.Name, pErr.Source, pErr.Version, PolicyStatusError, pErr.Err.Error())
				} else { // this case shouldn't happen, but just in case it does let's update the policy status
					policyState.Status = PolicyStatusError
					if policyState.Message == "" {
						policyState.Message = pErr.Err.Error()
					}
				}
			}
		}
	}

	for _, rule := range filteredRules {
		for pInfo := range rule.Policies(includeInternalPolicies) {
			policyName := pInfo.Name
			if policyState, exists = mp[policyName]; !exists {
				// if the policy is not in the map, this means that no rule from this policy was loaded successfully
				policyState = NewPolicyState(pInfo.Name, pInfo.Source, pInfo.Version, PolicyStatusFullyFiltered, "")
				mp[policyName] = policyState
			} else if policyState.Status == PolicyStatusLoaded {
				policyState.Status = PolicyStatusPartiallyFiltered
			}
			policyState.Rules = append(policyState.Rules, RuleStateFromRule(rule, pInfo, "filtered", ""))
		}
	}

	var policies []*PolicyState
	for _, policy := range mp {
		policies = append(policies, policy)
	}

	return policies
}

// RulesetLoadedEventBundle is used to report a ruleset loaded event
type RulesetLoadedEventBundle struct {
	Rule  *rules.Rule
	Event *events.CustomEvent
}

// NewRuleSetLoadedEvent returns the rule (e.g. ruleset_loaded) and a populated custom event for a new_rules_loaded event
func NewRuleSetLoadedEvent(acc *events.AgentContainerContext, rs *rules.RuleSet, policies []*PolicyState, filterReport *kfilters.FilterReport) RulesetLoadedEventBundle {
	evt := RulesetLoadedEvent{
		Policies:       policies,
		Filters:        filterReport,
		MonitoredFiles: extractMonitoredFilesAndFolders(rs),
	}
	evt.FillCustomEventCommonFields(acc)

	return RulesetLoadedEventBundle{
		Rule:  events.NewCustomRule(events.RulesetLoadedRuleID, events.RulesetLoadedRuleDesc),
		Event: events.NewCustomEvent(model.CustomEventType, evt),
	}
}

// newHeartbeatEvents returns the rule (e.g. heartbeat) and a populated custom event for a heartbeat event
func newHeartbeatEvents(acc *events.AgentContainerContext, policies []*PolicyState) (*rules.Rule, []*events.CustomEvent) {
	var evts []*events.CustomEvent

	for _, policy := range policies {
		evt := HeartbeatEvent{
			Policy: policy,
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
