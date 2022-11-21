//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// policyStatusMetricRate defines how often the policy metric will be sent
	policyStatusMetricRate = time.Minute

	// policyStatusMetricRate defines how often the policy log status will be sent
	policyStatusLogRate = 15 * time.Minute

	// monitor types
	metricMonitorType = "metric"
	logMonitorType    = "log"
)

// Policy describes policy related information
type Policy struct {
	Name    string
	Source  string
	Version string
}

// PolicyMonitor defines a policy monitor
type PolicyMonitor struct {
	sync.RWMutex

	types        []string
	sender       EventSender
	statsdClient statsd.ClientInterface
	policies     map[string]Policy
	rules        map[string]*rules.ErrRuleLoad
}

// AddPolicies add policies to the monitor
func (p *PolicyMonitor) AddPolicies(policies []*rules.Policy, mErrs *multierror.Error) {
	p.Lock()
	defer p.Unlock()

	for _, policy := range policies {
		p.policies[policy.Name] = Policy{Name: policy.Name, Source: policy.Source, Version: policy.Version}

		for _, rule := range policy.Rules {
			p.rules[rule.ID] = nil
		}

		if mErrs != nil && mErrs.Errors != nil {
			for _, err := range mErrs.Errors {
				if rerr, ok := err.(*rules.ErrRuleLoad); ok {
					p.rules[rerr.Definition.ID] = rerr
				}
			}
		}
	}
}

// Start the monitor
func (p *PolicyMonitor) Start(ctx context.Context) {
	go func() {
		timerMetric := time.NewTicker(policyStatusMetricRate)
		defer timerMetric.Stop()

		timerLog := time.NewTicker(policyStatusLogRate)
		defer timerLog.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-timerLog.C:
				if !slices.Contains(p.types, logMonitorType) {
					continue
				}

				var report RuleStatesReport

				p.RLock()
				for id, err := range p.rules {
					if err == nil {
						report.Loaded = append(report.Loaded, id)
					} else {
						switch err.Type() {
						case rules.AgentVersionErrType:
							report.AgentVersionErr = append(report.AgentVersionErr, id)
						case rules.AgentFilterErrType:
							report.AgentFilterErr = append(report.AgentFilterErr, id)
						case rules.EventTypeNotEnabledErrType:
							report.EventTypeNotEnabledErr = append(report.EventTypeNotEnabledErr, id)
						case rules.SyntaxErrType:
							report.SyntaxErr = append(report.SyntaxErr, id)
						default:
							report.UnknownErr = append(report.UnknownErr, id)
						}
					}
				}
				p.RUnlock()

				rule, event := events.NewCustomRule(events.RulesStateRuleID), events.NewCustomEvent(model.CustomRulesStateEventType, report)
				p.sender.SendEvent(rule, event, func() []string { return nil }, "")
			case <-timerMetric.C:
				if !slices.Contains(p.types, metricMonitorType) {
					continue
				}

				p.RLock()
				for _, policy := range p.policies {
					tags := []string{
						"policy_name:" + policy.Name,
						"policy_source:" + policy.Source,
						"policy_version:" + policy.Version,
						"agent_version:" + version.AgentVersion,
					}

					if err := p.statsdClient.Gauge(metrics.MetricPolicy, 1, tags, 1.0); err != nil {
						log.Error(fmt.Errorf("failed to send policy metric: %w", err))
					}
				}

				for id, err := range p.rules {
					var status = "loaded"
					if err != nil {
						status = string(err.Type())
					}

					tags := []string{
						"rule_id:" + id,
						fmt.Sprintf("status:%v", status),
						dogstatsd.CardinalityTagPrefix + collectors.LowCardinalityString,
					}

					if err := p.statsdClient.Gauge(metrics.MetricRulesStatus, 1, tags, 1.0); err != nil {
						log.Error(fmt.Errorf("failed to send policy metric: %w", err))
					}
				}
				p.RUnlock()
			}
		}
	}()
}

// NewPolicyMonitor returns a new Policy monitor
func NewPolicyMonitor(sender EventSender, statsdClient statsd.ClientInterface, types []string) *PolicyMonitor {
	return &PolicyMonitor{
		sender:       sender,
		statsdClient: statsdClient,
		types:        types,
		policies:     make(map[string]Policy),
		rules:        make(map[string]*rules.ErrRuleLoad),
	}
}

// RuleStatesReport describes the rules states report
// easyjson:json
type RuleStatesReport struct {
	Loaded                 []eval.RuleID
	AgentVersionErr        []eval.RuleID
	AgentFilterErr         []eval.RuleID
	EventTypeNotEnabledErr []eval.RuleID
	SyntaxErr              []eval.RuleID
	UnknownErr             []eval.RuleID
}

// RuleSetLoadedReport represents the rule and the custom event related to a RuleSetLoaded event, ready to be dispatched
type RuleSetLoadedReport struct {
	Rule  *rules.Rule
	Event *events.CustomEvent
}

// ReportRuleSetLoaded reports to Datadog that new ruleset was loaded
func ReportRuleSetLoaded(sender EventSender, statsdClient statsd.ClientInterface, ruleSet *rules.RuleSet, err *multierror.Error) {
	rule, event := NewRuleSetLoadedEvent(ruleSet, err)

	if err := statsdClient.Count(metrics.MetricRuleSetLoaded, 1, []string{}, 1.0); err != nil {
		log.Error(fmt.Errorf("failed to send ruleset_loaded metric: %w", err))
	}

	sender.SendEvent(rule, event, func() []string { return nil }, "")
}

// RuleLoaded defines a loaded rule
// easyjson:json
type RuleState struct {
	ID         string `json:"id"`
	Version    string `json:"version,omitempty"`
	Expression string `json:"expression"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

// PolicyState is used to report policy was loaded
// easyjson:json
type PolicyState struct {
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Source  string       `json:"source"`
	Rules   []*RuleState `json:"rules"`
}

// RulesetLoadedEvent is used to report that a new ruleset was loaded
// easyjson:json
type RulesetLoadedEvent struct {
	Timestamp time.Time      `json:"date"`
	Policies  []*PolicyState `json:"policies"`
}

func PolicyStateFromRuleDefinition(def *rules.RuleDefinition) *PolicyState {
	return &PolicyState{
		Name:    def.Policy.Name,
		Version: def.Policy.Version,
		Source:  def.Policy.Source,
	}
}

func RuleStateFromDefinition(def *rules.RuleDefinition, status string, message string) *RuleState {
	return &RuleState{
		ID:         def.ID,
		Version:    def.Version,
		Expression: def.Expression,
		Status:     status,
		Message:    message,
	}
}

// NewRuleSetLoadedEvent returns the rule and a populated custom event for a new_rules_loaded event
func NewRuleSetLoadedEvent(rs *rules.RuleSet, err *multierror.Error) (*rules.Rule, *events.CustomEvent) {
	mp := make(map[string]*PolicyState)

	var policyState *PolicyState
	var exists bool

	for _, policy := range rs.GetPolicies() {
		// rule successfully loaded
		for _, ruleDef := range policy.Rules {
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
				policyName := rerr.Definition.Policy.Name

				if _, exists := mp[policyName]; !exists {
					policyState = PolicyStateFromRuleDefinition(rerr.Definition)
					mp[policyName] = policyState
				}
				policyState.Rules = append(policyState.Rules, RuleStateFromDefinition(rerr.Definition, string(rerr.Type()), rerr.Err.Error()))
			}
		}
	}

	var policies []*PolicyState
	for _, policy := range mp {
		policies = append(policies, policy)
	}

	return events.NewCustomRule(events.RulesetLoadedRuleID), events.NewCustomEvent(model.CustomRulesetLoadedEventType, RulesetLoadedEvent{
		Timestamp: time.Now(),
		Policies:  policies,
	})
}
