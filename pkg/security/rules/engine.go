// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"context"
	json "encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// ProbeEvaluationRuleSetTagValue defines the probe evaluation rule-set tag value
	ProbeEvaluationRuleSetTagValue = "probe_evaluation"
	// ThreatScoreRuleSetTagValue defines the threat-score rule-set tag value
	ThreatScoreRuleSetTagValue = "threat_score"
)

// RuleEngine defines a rule engine
type RuleEngine struct {
	sync.RWMutex
	config                    *config.RuntimeSecurityConfig
	probe                     *probe.Probe
	apiServer                 APIServer
	reloading                 *atomic.Bool
	rateLimiter               *events.RateLimiter
	currentRuleSet            *atomic.Value
	currentThreatScoreRuleSet *atomic.Value
	rulesLoaded               func(es *rules.EvaluationSet, err *multierror.Error)
	policiesVersions          []string
	policyProviders           []rules.PolicyProvider
	policyLoader              *rules.PolicyLoader
	policyOpts                rules.PolicyLoaderOpts
	policyMonitor             *PolicyMonitor
	statsdClient              statsd.ClientInterface
	eventSender               events.EventSender
	rulesetListeners          []rules.RuleSetListener
}

// APIServer defines the API server
type APIServer interface {
	Apply([]string)
}

// NewRuleEngine returns a new rule engine
func NewRuleEngine(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, probe *probe.Probe, rateLimiter *events.RateLimiter, apiServer APIServer, sender events.EventSender, statsdClient statsd.ClientInterface, rulesetListeners ...rules.RuleSetListener) (*RuleEngine, error) {
	engine := &RuleEngine{
		probe:                     probe,
		config:                    config,
		apiServer:                 apiServer,
		eventSender:               sender,
		rateLimiter:               rateLimiter,
		reloading:                 atomic.NewBool(false),
		policyMonitor:             NewPolicyMonitor(evm.StatsdClient, config.PolicyMonitorPerRuleEnabled),
		currentRuleSet:            new(atomic.Value),
		currentThreatScoreRuleSet: new(atomic.Value),
		policyLoader:              rules.NewPolicyLoader(),
		statsdClient:              statsdClient,
		rulesetListeners:          rulesetListeners,
	}

	// register as event handler
	if err := probe.AddFullAccessEventHandler(engine); err != nil {
		return nil, err
	}

	return engine, nil
}

// Start the rule engine
func (e *RuleEngine) Start(ctx context.Context, reloadChan <-chan struct{}, wg *sync.WaitGroup) error {
	// monitor policies
	if e.config.PolicyMonitorEnabled {
		e.policyMonitor.Start(ctx)
	}

	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		seclog.Errorf("failed to parse agent version: %v", err)
	}

	// Set up rule filters
	var macroFilters []rules.MacroFilter
	var ruleFilters []rules.RuleFilter

	agentVersionFilter, err := rules.NewAgentVersionFilter(agentVersion)
	if err != nil {
		seclog.Errorf("failed to create agent version filter: %v", err)
	} else {
		macroFilters = append(macroFilters, agentVersionFilter)
		ruleFilters = append(ruleFilters, agentVersionFilter)
	}

	ruleFilterModel := NewRuleFilterModel()
	seclRuleFilter := rules.NewSECLRuleFilter(ruleFilterModel)
	macroFilters = append(macroFilters, seclRuleFilter)
	ruleFilters = append(ruleFilters, seclRuleFilter)

	e.policyOpts = rules.PolicyLoaderOpts{
		MacroFilters: macroFilters,
		RuleFilters:  ruleFilters,
	}

	policyProviders := e.gatherPolicyProviders()
	if err := e.LoadPolicies(policyProviders, true); err != nil {
		return fmt.Errorf("failed to load policies: %s", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for range reloadChan {
			if err := e.ReloadPolicies(); err != nil {
				seclog.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for range e.policyLoader.NewPolicyReady() {
			if err := e.ReloadPolicies(); err != nil {
				seclog.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	for _, provider := range e.policyProviders {
		provider.Start()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		heartbeatTicker := time.NewTicker(15 * time.Second)
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				tags := []string{fmt.Sprintf("version:%s", version.AgentVersion)}

				e.RLock()
				for _, version := range e.policiesVersions {
					tags = append(tags, fmt.Sprintf("policies_version:%s", version))
				}
				e.RUnlock()

				if e.config.RuntimeEnabled {
					_ = e.statsdClient.Gauge(metrics.MetricSecurityAgentRuntimeRunning, 1, tags, 1)
				} else if e.config.FIMEnabled {
					_ = e.statsdClient.Gauge(metrics.MetricSecurityAgentFIMRunning, 1, tags, 1)
				}
			}
		}
	}()
	return nil
}

// ReloadPolicies reloads the policies
func (e *RuleEngine) ReloadPolicies() error {
	seclog.Infof("reload policies")

	return e.LoadPolicies(e.policyProviders, true)
}

// LoadPolicies loads the policies
func (e *RuleEngine) LoadPolicies(policyProviders []rules.PolicyProvider, sendLoadedReport bool) error {
	seclog.Infof("load policies")

	e.Lock()
	defer e.Unlock()

	e.reloading.Store(true)
	defer e.reloading.Store(false)

	// load policies
	e.policyLoader.SetProviders(policyProviders)

	evaluationSet, err := e.probe.NewEvaluationSet(e.getEventTypeEnabled(), []string{ProbeEvaluationRuleSetTagValue, ThreatScoreRuleSetTagValue})
	if err != nil {
		return err
	}

	loadErrs := evaluationSet.LoadPolicies(e.policyLoader, e.policyOpts)
	if loadErrs.ErrorOrNil() != nil {
		logLoadingErrors("error while loading policies: %+v", loadErrs)
	}

	// update current policies related module attributes
	e.policiesVersions = getPoliciesVersions(evaluationSet)
	e.policyProviders = policyProviders

	// notify listeners
	if e.rulesLoaded != nil {
		e.rulesLoaded(evaluationSet, loadErrs)
	}

	for _, listener := range e.rulesetListeners {
		for _, rs := range evaluationSet.RuleSets {
			rs.AddListener(listener)
		}
	}

	// add module as listener for rule match callback
	for _, rs := range evaluationSet.RuleSets {
		rs.AddListener(e)
	}

	// full list of IDs, user rules + custom
	var ruleIDs []rules.RuleID
	ruleIDs = append(ruleIDs, events.AllCustomRuleIDs()...)

	probeEvaluationRuleSet := evaluationSet.RuleSets[ProbeEvaluationRuleSetTagValue]
	threatScoreRuleSet := evaluationSet.RuleSets[ThreatScoreRuleSetTagValue]

	if threatScoreRuleSet != nil {
		e.currentThreatScoreRuleSet.Store(threatScoreRuleSet)
		ruleIDs = append(ruleIDs, threatScoreRuleSet.ListRuleIDs()...)
	}

	if probeEvaluationRuleSet != nil {
		e.currentRuleSet.Store(probeEvaluationRuleSet)
		ruleIDs = append(ruleIDs, probeEvaluationRuleSet.ListRuleIDs()...)

		// analyze the ruleset, push probe evaluation rule sets to the kernel and generate the policy report
		report, err := e.probe.ApplyRuleSet(probeEvaluationRuleSet)
		if err != nil {
			return err
		}

		content, _ := json.Marshal(report)
		seclog.Debugf("Policy report: %s", content)

		// set the rate limiters on sending events to the backend
		e.rateLimiter.Apply(probeEvaluationRuleSet, events.AllCustomRuleIDs())

	}

	e.apiServer.Apply(ruleIDs)

	if sendLoadedReport {
		ReportRuleSetLoaded(e.eventSender, e.statsdClient, evaluationSet.RuleSets, loadErrs)
		e.policyMonitor.SetPolicies(evaluationSet.GetPolicies(), loadErrs)
	}

	return nil
}

func (e *RuleEngine) gatherPolicyProviders() []rules.PolicyProvider {
	var policyProviders []rules.PolicyProvider

	// add remote config as config provider if enabled.
	if e.config.RemoteConfigurationEnabled {
		rcPolicyProvider, err := rconfig.NewRCPolicyProvider()
		if err != nil {
			seclog.Errorf("will be unable to load remote policies: %s", err)
		} else {
			policyProviders = append(policyProviders, rcPolicyProvider)
		}
	}

	// directory policy provider
	if provider, err := rules.NewPoliciesDirProvider(e.config.PoliciesDir, e.config.WatchPoliciesDir); err != nil {
		seclog.Errorf("failed to load local policies: %s", err)
	} else {
		policyProviders = append(policyProviders, provider)
	}

	return policyProviders
}

// GetPolicyProviders returns the active policy providers
func (e *RuleEngine) GetPolicyProviders() []rules.PolicyProvider {
	return e.policyProviders
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (e *RuleEngine) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if e.reloading.Load() {
		return
	}

	e.probe.OnNewDiscarder(rs, event.(*model.Event), field, eventType)
}

// RuleMatch is called by the ruleset when a rule matches
func (e *RuleEngine) RuleMatch(rule *rules.Rule, event eval.Event) bool {
	ev := event.(*model.Event)

	// do not send broken event
	if ev.Error != nil {
		return false
	}

	// ensure that all the fields are resolved before sending
	ev.FieldHandlers.ResolveContainerID(ev, ev.ContainerContext)
	ev.FieldHandlers.ResolveContainerTags(ev, ev.ContainerContext)
	ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.ContainerContext)

	if ev.ContainerContext.ID != "" && (e.config.ActivityDumpTagRulesEnabled || e.config.AnomalyDetectionTagRulesEnabled) {
		ev.Rules = append(ev.Rules, model.NewMatchedRule(rule.Definition.ID, rule.Definition.Version, rule.Definition.Tags, rule.Definition.Policy.Name, rule.Definition.Policy.Version))
	}
	if val, ok := rule.Definition.GetTag("ruleset"); ok && val == "threat_score" {
		return false // if the triggered rule is only meant to tag secdumps, dont send it
	}

	// needs to be resolved here, outside of the callback as using process tree
	// which can be modified during queuing
	service := e.probe.GetService(ev)
	containerID := ev.ContainerContext.ID
	extTagsCb := func() []string {
		return e.probe.GetEventTags(containerID)
	}

	e.eventSender.SendEvent(rule, ev, extTagsCb, service)

	return true
}

// Stop stops the rule engine
func (e *RuleEngine) Stop() {
	for _, provider := range e.policyProviders {
		_ = provider.Close()
	}

	// close the policy loader and all the related providers
	if e.policyLoader != nil {
		e.policyLoader.Close()
	}
}

func (e *RuleEngine) getEventTypeEnabled() map[eval.EventType]bool {
	enabled := make(map[eval.EventType]bool)

	categories := model.GetEventTypePerCategory()

	if e.config.FIMEnabled {
		if eventTypes, exists := categories[model.FIMCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	if e.probe.IsNetworkEnabled() {
		if eventTypes, exists := categories[model.NetworkCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	if e.config.RuntimeEnabled {
		// everything but FIM
		for _, category := range model.GetAllCategories() {
			if category == model.FIMCategory || category == model.NetworkCategory {
				continue
			}

			if eventTypes, exists := categories[category]; exists {
				for _, eventType := range eventTypes {
					enabled[eventType] = true
				}
			}
		}
	}

	return enabled
}

// GetRuleSet returns the set of loaded rules
func (e *RuleEngine) GetRuleSet() (rs *rules.RuleSet) {
	if ruleSet := e.currentRuleSet.Load(); ruleSet != nil {
		return ruleSet.(*rules.RuleSet)
	}
	return nil
}

// GetThreatScoreRuleSet returns the set of loaded rules
func (e *RuleEngine) GetThreatScoreRuleSet() (rs *rules.RuleSet) {
	if threatScoreRuleSet := e.currentThreatScoreRuleSet.Load(); threatScoreRuleSet != nil {
		return threatScoreRuleSet.(*rules.RuleSet)
	}
	return nil
}

// SetRulesetLoadedCallback allows setting a callback called when a rule set is loaded
func (e *RuleEngine) SetRulesetLoadedCallback(cb func(es *rules.EvaluationSet, err *multierror.Error)) {
	e.rulesLoaded = cb
}

// HandleEvent is called by the probe when an event arrives from the kernel
func (e *RuleEngine) HandleEvent(event *model.Event) {
	// event already marked with an error, skip it
	if event.Error != nil {
		return
	}

	if threatScoreRuleSet := e.GetThreatScoreRuleSet(); threatScoreRuleSet != nil {
		threatScoreRuleSet.Evaluate(event)
	}

	// if the event should have been discarded in kernel space, we don't need to evaluate it
	if event.IsSavedByActivityDumps() {
		return
	}

	if ruleSet := e.GetRuleSet(); ruleSet != nil {
		if (event.SecurityProfileContext.Status.IsEnabled(model.AutoSuppression) && event.IsInProfile()) || !ruleSet.Evaluate(event) {
			ruleSet.EvaluateDiscarders(event)
		}
	}
}

// StopEventCollector stops the event collector
func (e *RuleEngine) StopEventCollector() []rules.CollectedEvent {
	return e.GetRuleSet().StopEventCollector()
}

func logLoadingErrors(msg string, m *multierror.Error) {
	var errorLevel bool
	for _, err := range m.Errors {
		if rErr, ok := err.(*rules.ErrRuleLoad); ok {
			if !errors.Is(rErr.Err, rules.ErrEventTypeNotEnabled) {
				errorLevel = true
			}
		}
	}

	if errorLevel {
		seclog.Errorf(msg, m.Error())
	} else {
		seclog.Warnf(msg, m.Error())
	}
}

func getPoliciesVersions(es *rules.EvaluationSet) []string {
	var versions []string

	cache := make(map[string]bool)
	for _, rs := range es.RuleSets {
		for _, rule := range rs.GetRules() {
			version := rule.Definition.Policy.Version

			if _, exists := cache[version]; !exists {
				cache[version] = true

				versions = append(versions, version)
			}
		}
	}

	return versions
}
