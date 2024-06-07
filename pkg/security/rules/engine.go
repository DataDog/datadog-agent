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
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/rules/autosuppression"
	"github.com/DataDog/datadog-agent/pkg/security/rules/monitor"
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
	// TagMaxResolutionDelay maximum tag resolution delay
	TagMaxResolutionDelay = 5 * time.Second
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
	policyMonitor             *monitor.PolicyMonitor
	statsdClient              statsd.ClientInterface
	eventSender               events.EventSender
	rulesetListeners          []rules.RuleSetListener
	AutoSuppression           autosuppression.AutoSuppression
}

// APIServer defines the API server
type APIServer interface {
	ApplyRuleIDs([]rules.RuleID)
	ApplyPolicyStates([]*monitor.PolicyState)
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
		policyMonitor:             monitor.NewPolicyMonitor(evm.StatsdClient, config.PolicyMonitorPerRuleEnabled),
		currentRuleSet:            new(atomic.Value),
		currentThreatScoreRuleSet: new(atomic.Value),
		policyLoader:              rules.NewPolicyLoader(),
		statsdClient:              statsdClient,
		rulesetListeners:          rulesetListeners,
	}

	engine.AutoSuppression.Init(autosuppression.Opts{
		SecurityProfileAutoSuppressionEnabled: config.SecurityProfileAutoSuppressionEnabled,
		ActivityDumpAutoSuppressionEnabled:    config.ActivityDumpAutoSuppressionEnabled,
		EventTypes:                            config.SecurityProfileAutoSuppressionEventTypes,
	})

	// register as event handler
	if err := probe.AddEventHandler(engine); err != nil {
		return nil, err
	}

	engine.policyProviders = engine.gatherDefaultPolicyProviders()

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

	ruleFilterModel, err := NewRuleFilterModel(e.probe.Origin())
	if err != nil {
		return fmt.Errorf("failed to create rule filter: %w", err)
	}
	seclRuleFilter := rules.NewSECLRuleFilter(ruleFilterModel)
	macroFilters = append(macroFilters, seclRuleFilter)
	ruleFilters = append(ruleFilters, seclRuleFilter)

	e.policyOpts = rules.PolicyLoaderOpts{
		MacroFilters:       macroFilters,
		RuleFilters:        ruleFilters,
		DisableEnforcement: !e.config.EnforcementEnabled,
	}

	if err := e.LoadPolicies(e.policyProviders, true); err != nil {
		return fmt.Errorf("failed to load policies: %w", err)
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
				tags := []string{
					fmt.Sprintf("version:%s", version.AgentVersion),
					fmt.Sprintf("os:%s", runtime.GOOS),
					constants.CardinalityTagPrefix + "none",
				}

				if os.Getenv("ECS_FARGATE") == "true" || os.Getenv("DD_ECS_FARGATE") == "true" {
					tags = append(tags, "mode:fargate_ecs")
				} else if os.Getenv("DD_EKS_FARGATE") == "true" {
					tags = append(tags, "mode:fargate_eks")
				} else {
					tags = append(tags, "mode:default")
				}

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

	// Sending an heartbeat event every minute
	wg.Add(1)
	go func() {
		defer wg.Done()

		// 5 heartbeats with a period of 1 min, after that we move the period to 10 min
		// if the policies change we go back to 5 beats every 1 min

		heartbeatTicker := time.NewTicker(1 * time.Minute)
		defer heartbeatTicker.Stop()

		heartBeatCounter := 5

		for {
			select {
			case <-ctx.Done():
				return
			case <-e.policyLoader.NewPolicyReady():
				heartBeatCounter = 5
				heartbeatTicker.Reset(1 * time.Minute)
				// we report a heartbeat anyway
				e.policyMonitor.ReportHeartbeatEvent(e.eventSender)
			case <-heartbeatTicker.C:
				e.policyMonitor.ReportHeartbeatEvent(e.eventSender)
				if heartBeatCounter > 0 {
					heartBeatCounter--
					if heartBeatCounter == 0 {
						heartbeatTicker.Reset(10 * time.Minute)
					}
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

// AddPolicyProvider add a provider
func (e *RuleEngine) AddPolicyProvider(provider rules.PolicyProvider) {
	e.Lock()
	defer e.Unlock()
	e.policyProviders = append(e.policyProviders, provider)
}

// LoadPolicies loads the policies
func (e *RuleEngine) LoadPolicies(providers []rules.PolicyProvider, sendLoadedReport bool) error {
	seclog.Infof("load policies")

	e.Lock()
	defer e.Unlock()

	e.reloading.Store(true)
	defer e.reloading.Store(false)

	// load policies
	e.policyLoader.SetProviders(providers)

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
		// analyze the ruleset, push probe evaluation rule sets to the kernel and generate the policy report
		report, err := e.probe.ApplyRuleSet(probeEvaluationRuleSet)
		if err != nil {
			return err
		}

		e.currentRuleSet.Store(probeEvaluationRuleSet)
		ruleIDs = append(ruleIDs, probeEvaluationRuleSet.ListRuleIDs()...)

		if err := e.probe.FlushDiscarders(); err != nil {
			return fmt.Errorf("failed to flush discarders: %w", err)
		}

		content, _ := json.Marshal(report)
		seclog.Debugf("Policy report: %s", content)

		// set the rate limiters on sending events to the backend
		e.rateLimiter.Apply(probeEvaluationRuleSet, events.AllCustomRuleIDs())

		// update the stats of auto-suppression rules
		e.AutoSuppression.Apply(probeEvaluationRuleSet)
	}

	policies := monitor.NewPoliciesState(evaluationSet.RuleSets, loadErrs, e.config.PolicyMonitorReportInternalPolicies)
	e.notifyAPIServer(ruleIDs, policies)

	if sendLoadedReport {
		monitor.ReportRuleSetLoaded(e.eventSender, e.statsdClient, policies)
		e.policyMonitor.SetPolicies(policies)
	}

	return nil
}

func (e *RuleEngine) notifyAPIServer(ruleIDs []rules.RuleID, policies []*monitor.PolicyState) {
	e.apiServer.ApplyRuleIDs(ruleIDs)
	e.apiServer.ApplyPolicyStates(policies)
}

func (e *RuleEngine) gatherDefaultPolicyProviders() []rules.PolicyProvider {
	var policyProviders []rules.PolicyProvider

	policyProviders = append(policyProviders, NewBundledPolicyProvider(e.config))

	// add remote config as config provider if enabled.
	if e.config.RemoteConfigurationEnabled {
		rcPolicyProvider, err := rconfig.NewRCPolicyProvider(e.config.RemoteConfigurationDumpPolicies)
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

	// add matched rules before any auto suppression check to ensure that this information is available in activity dumps
	if ev.ContainerContext.ID != "" && (e.config.ActivityDumpTagRulesEnabled || e.config.AnomalyDetectionTagRulesEnabled) {
		ev.Rules = append(ev.Rules, model.NewMatchedRule(rule.Definition.ID, rule.Definition.Version, rule.Definition.Tags, rule.Definition.Policy.Name, rule.Definition.Policy.Version))
	}

	if e.AutoSuppression.Suppresses(rule, ev) {
		return false
	}

	e.probe.HandleActions(rule, event)

	if rule.Definition.Silent {
		return false
	}

	// ensure that all the fields are resolved before sending
	ev.FieldHandlers.ResolveContainerID(ev, ev.ContainerContext)
	ev.FieldHandlers.ResolveContainerTags(ev, ev.ContainerContext)
	ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.ContainerContext)

	// do not send event if a anomaly detection event will be sent
	if e.config.AnomalyDetectionSilentRuleEventsEnabled && ev.IsAnomalyDetectionEvent() {
		return false
	}

	if val, ok := rule.Definition.GetTag("ruleset"); ok && val == "threat_score" {
		return false // if the triggered rule is only meant to tag secdumps, dont send it
	}

	// needs to be resolved here, outside of the callback as using process tree
	// which can be modified during queuing
	service := e.probe.GetService(ev)

	var extTagsCb func() []string

	if ev.ContainerContext.ID != "" {
		// copy the container ID here to avoid later data race
		containerID := ev.ContainerContext.ID

		// the container tags might not be resolved yet
		if time.Unix(0, int64(ev.ContainerContext.CreatedAt)).Add(TagMaxResolutionDelay).After(time.Now()) {
			extTagsCb = func() []string {
				return e.probe.GetEventTags(containerID)
			}
		}
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
		if !ruleSet.Evaluate(event) {
			ruleSet.EvaluateDiscarders(event)
		}
	}
}

// StopEventCollector stops the event collector
func (e *RuleEngine) StopEventCollector() []rules.CollectedEvent {
	return e.GetRuleSet().StopEventCollector()
}

func logLoadingErrors(msg string, m *multierror.Error) {
	for _, err := range m.Errors {
		if rErr, ok := err.(*rules.ErrRuleLoad); ok {
			if !errors.Is(rErr.Err, rules.ErrEventTypeNotEnabled) && !errors.Is(rErr.Err, rules.ErrRuleAgentFilter) {
				seclog.Errorf(msg, rErr.Error())
			} else {
				seclog.Warnf(msg, rErr.Error())
			}
		} else {
			seclog.Errorf(msg, err.Error())
		}
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
