// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/rules/bundled"
	"github.com/DataDog/datadog-agent/pkg/security/rules/filtermodel"
	"github.com/DataDog/datadog-agent/pkg/security/rules/monitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// TagMaxResolutionDelay maximum tag resolution delay
	TagMaxResolutionDelay = 5 * time.Second

	// ContainerMaxTagResolutionDelay maximum container resolution delay
	ContainerMaxTagResolutionDelay = 1 * time.Minute
)

// RuleEngine defines a rule engine
type RuleEngine struct {
	sync.RWMutex
	config           *config.RuntimeSecurityConfig
	probe            *probe.Probe
	apiServer        APIServer
	reloading        *atomic.Bool
	rateLimiter      *events.RateLimiter
	currentRuleSet   *atomic.Value
	rulesLoaded      func(rs *rules.RuleSet, err *multierror.Error)
	policiesVersions []string
	policyProviders  []rules.PolicyProvider
	policyLoader     *rules.PolicyLoader
	policyOpts       rules.PolicyLoaderOpts
	policyMonitor    *monitor.PolicyMonitor
	statsdClient     statsd.ClientInterface
	eventSender      events.EventSender
	rulesetListeners []rules.RuleSetListener
	pid              uint32
	wg               sync.WaitGroup
	ipc              ipc.Component
	hostname         string
	bundledProvider  *bundled.PolicyProvider

	// userspace filtering metrics (avoid statsd calls in event hot path)
	noMatchCounters []atomic.Uint64
}

// APIServer defines the API server
type APIServer interface {
	ApplyRuleIDs([]rules.RuleID)
	ApplyPolicyStates([]*monitor.PolicyState)
	GetSECLVariables() map[string]*api.SECLVariableState
}

// NewRuleEngine returns a new rule engine
func NewRuleEngine(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, probe *probe.Probe, rateLimiter *events.RateLimiter, apiServer APIServer, sender events.EventSender, statsdClient statsd.ClientInterface, hostname string, ipc ipc.Component, rulesetListeners ...rules.RuleSetListener) (*RuleEngine, error) {
	engine := &RuleEngine{
		probe:            probe,
		config:           config,
		apiServer:        apiServer,
		eventSender:      sender,
		rateLimiter:      rateLimiter,
		reloading:        atomic.NewBool(false),
		policyMonitor:    monitor.NewPolicyMonitor(evm.StatsdClient, config.PolicyMonitorPerRuleEnabled),
		currentRuleSet:   new(atomic.Value),
		policyLoader:     rules.NewPolicyLoader(),
		statsdClient:     statsdClient,
		rulesetListeners: rulesetListeners,
		pid:              utils.Getpid(),
		hostname:         hostname,
		ipc:              ipc,
	}

	engine.noMatchCounters = make([]atomic.Uint64, model.MaxAllEventType)

	// register as event handler
	if err := probe.AddEventHandler(engine); err != nil {
		return nil, err
	}

	engine.policyProviders = engine.gatherDefaultPolicyProviders()

	return engine, nil
}

// SendStats flushes per-event counters as metrics
func (e *RuleEngine) SendStats() {
	if e.statsdClient == nil || len(e.noMatchCounters) == 0 {
		return
	}

	for i := range e.noMatchCounters {
		value := e.noMatchCounters[i].Swap(0)
		if value == 0 {
			continue
		}

		eventType := model.EventType(i).String()
		tags := []string{
			"event_type:" + eventType,
			"category:" + model.GetEventTypeCategory(eventType).String(),
		}
		_ = e.statsdClient.Count(metrics.MetricRulesNoMatch, int64(value), tags, 1.0)
	}
}

// Start the rule engine
func (e *RuleEngine) Start(ctx context.Context, reloadChan <-chan struct{}) error {
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

	rfmCfg := filtermodel.RuleFilterEventConfig{
		COREEnabled: e.probe.Config.Probe.EnableCORE,
		Origin:      e.probe.Origin(),
	}
	ruleFilterModel, err := filtermodel.NewRuleFilterModel(rfmCfg, e.hostname)
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

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		for range reloadChan {
			if err := e.ReloadPolicies(); err != nil {
				seclog.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		for notification := range e.policyLoader.NewPolicyReady() {
			// Reload policies regardless of whether the notification is silent
			// The silent flag only affects heartbeat reporting
			_ = notification
			if err := e.ReloadPolicies(); err != nil {
				seclog.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		ruleSetCleanupTicker := time.NewTicker(5 * time.Minute)
		defer ruleSetCleanupTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ruleSetCleanupTicker.C:
				if ruleSet := e.GetRuleSet(); ruleSet != nil {
					ruleSet.CleanupExpiredVariables()
				}
			}
		}
	}()

	for _, provider := range e.policyProviders {
		provider.Start()
	}

	e.startSendHeartbeatEvents(ctx)

	// Connect the SBOM resolver to the bundled policy provider
	// This allows SBOM-generated policies to be automatically loaded
	e.ConnectSBOMResolver()

	return nil
}

func (e *RuleEngine) startSendHeartbeatEvents(ctx context.Context) {
	// Sending an heartbeat event every minute
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		// 5 heartbeats with a period of 1 min, after that we move the period to 10 min
		// if the policies change we go back to 5 beats every 1 min

		heartbeatTicker := time.NewTicker(1 * time.Minute)
		defer heartbeatTicker.Stop()

		heartBeatCounter := 5

		for {
			select {
			case <-ctx.Done():
				return
			case notification := <-e.policyLoader.NewPolicyReady():
				// Only send heartbeat and reset counter for non-silent reloads
				// Silent reloads (like SBOM updates) don't trigger heartbeat events
				if !notification.Silent {
					heartBeatCounter = 5
					heartbeatTicker.Reset(1 * time.Minute)
					// we report a heartbeat anyway
					e.policyMonitor.ReportHeartbeatEvent(e.probe.GetAgentContainerContext(), e.eventSender)
				}
			case <-heartbeatTicker.C:
				e.policyMonitor.ReportHeartbeatEvent(e.probe.GetAgentContainerContext(), e.eventSender)
				if heartBeatCounter > 0 {
					heartBeatCounter--
					if heartBeatCounter == 0 {
						heartbeatTicker.Reset(10 * time.Minute)
					}
				}
			}
		}
	}()
}

// ConnectSBOMResolver connects the SBOM resolver to the bundled policy provider
// so that SBOM-generated policies are automatically loaded when SBOMs are computed
func (e *RuleEngine) ConnectSBOMResolver() {
	if e.bundledProvider == nil {
		return
	}

	// Get the eBPF probe to access resolvers
	ebpfProbe, ok := e.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok || ebpfProbe.Resolvers == nil || ebpfProbe.Resolvers.SBOMResolver == nil {
		return
	}

	ebpfProbe.Resolvers.SBOMResolver.SetPolicyGeneratorCallback(func(workloadKey string, containerID containerutils.ContainerID, policyDef *rules.PolicyDef) {
		// Set the SBOM-generated policy definition on the bundled provider
		// This will trigger a silent reload (no heartbeat event)
		if policyDef != nil {
			seclog.Infof("Setting SBOM-generated policy for workload %s (container %s) with %d macros and %d rules",
				workloadKey, containerID, len(policyDef.Macros), len(policyDef.Rules))
			e.bundledProvider.SetSBOMPolicyDef(workloadKey, policyDef)
		}
	})

	seclog.Infof("SBOM resolver connected to bundled policy provider")
}

// StartRunningMetrics starts sending the running metrics
func (e *RuleEngine) StartRunningMetrics(ctx context.Context) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		heartbeatTicker := time.NewTicker(15 * time.Second)
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				tags := []string{
					"version:" + version.AgentVersion,
					"os:" + runtime.GOOS,
					constants.CardinalityTagPrefix + "none",
				}

				var (
					runtimeMetric = metrics.MetricSecurityAgentRuntimeRunning
					fimMetric     = metrics.MetricSecurityAgentFIMRunning
				)

				if os.Getenv("ECS_FARGATE") == "true" || os.Getenv("DD_ECS_FARGATE") == "true" {
					tags = append(tags, []string{
						"uuid:" + uuid.GetUUID(),
						"mode:fargate_ecs",
					}...)
					runtimeMetric = metrics.MetricSecurityAgentFargateRuntimeRunning
					fimMetric = metrics.MetricSecurityAgentFargateFIMRunning
				} else if os.Getenv("DD_EKS_FARGATE") == "true" {
					tags = append(tags, []string{
						"uuid:" + uuid.GetUUID(),
						"mode:fargate_eks",
					}...)
					runtimeMetric = metrics.MetricSecurityAgentFargateRuntimeRunning
					fimMetric = metrics.MetricSecurityAgentFargateFIMRunning
				} else {
					tags = append(tags, "mode:default")
				}

				e.RLock()
				for _, version := range e.policiesVersions {
					tags = append(tags, "policies_version:"+version)
				}
				e.RUnlock()

				if e.config.RuntimeEnabled {
					_ = e.statsdClient.Gauge(runtimeMetric, 1, tags, 1)
				} else if e.config.FIMEnabled {
					_ = e.statsdClient.Gauge(fimMetric, 1, tags, 1)
				}
			}
		}
	}()
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

	rs := e.probe.NewRuleSet(e.getEventTypeEnabled())

	filteredRules, loadErrs := rs.LoadPolicies(e.policyLoader, e.policyOpts)
	if loadErrs.ErrorOrNil() != nil {
		logLoadingErrors("error while loading policies: %+v", loadErrs)
	}

	// update current policies related module attributes
	e.policiesVersions = getPoliciesVersions(rs, e.config.PolicyMonitorReportInternalPolicies)

	// notify listeners
	if e.rulesLoaded != nil {
		e.rulesLoaded(rs, loadErrs)
	}

	for _, listener := range e.rulesetListeners {
		rs.AddListener(listener)
	}

	rs.AddListener(e)

	// full list of IDs, user rules + custom
	var ruleIDs []rules.RuleID
	ruleIDs = append(ruleIDs, events.AllCustomRuleIDs()...)

	// analyze the ruleset, push probe evaluation rule sets to the kernel and generate the policy report
	filterReport, replayEvents, err := e.probe.ApplyRuleSet(rs)
	if err != nil {
		return err
	}
	seclog.Debugf("Filter Report: %s", filterReport)

	policies := monitor.NewPoliciesState(rs, filteredRules, loadErrs, e.config.PolicyMonitorReportInternalPolicies)
	rulesetLoadedEvent := monitor.NewRuleSetLoadedEvent(e.probe.GetAgentContainerContext(), rs, policies, filterReport)

	ruleIDs = append(ruleIDs, rs.ListRuleIDs()...)

	e.currentRuleSet.Store(rs)

	if err := e.probe.FlushDiscarders(); err != nil {
		return fmt.Errorf("failed to flush discarders: %w", err)
	}

	// reset the probe process killer state once the new ruleset is loaded
	e.probe.OnNewRuleSetLoaded(rs)

	// set the rate limiters on sending events to the backend
	e.rateLimiter.Apply(rs, events.AllCustomRuleIDs())

	if replayEvents {
		e.probe.ReplayEvents()
	}

	e.notifyAPIServer(ruleIDs, policies)

	if sendLoadedReport {
		monitor.ReportRuleSetLoaded(rulesetLoadedEvent, e.eventSender, e.statsdClient)
		e.policyMonitor.SetPolicies(policies)
	}

	return nil
}

func (e *RuleEngine) notifyAPIServer(ruleIDs []rules.RuleID, policies []*monitor.PolicyState) {
	e.apiServer.ApplyRuleIDs(ruleIDs)
	e.apiServer.ApplyPolicyStates(policies)
}

type seclVariableEventPreparator struct {
	ctxPool *eval.ContextPool
	event   *model.Event
}

func (e *RuleEngine) newSECLVariableEventPreparator() *seclVariableEventPreparator {
	return &seclVariableEventPreparator{
		ctxPool: eval.NewContextPool(),
		event:   e.probe.PlatformProbe.NewEvent(),
	}
}

var eventZeroer = model.NewEventZeroer()

func (p *seclVariableEventPreparator) get(f func(event *model.Event)) *eval.Context {
	eventZeroer(p.event)
	f(p.event)
	return p.ctxPool.Get(p.event)
}

func (p *seclVariableEventPreparator) put(ctx *eval.Context) {
	p.ctxPool.Put(ctx)
}

func (e *RuleEngine) fillCommonSECLVariables(rsVariables map[string]eval.SECLVariable, seclVariables map[string]*api.SECLVariableState, preparator *seclVariableEventPreparator) {
	for name, value := range rsVariables {
		if strings.HasPrefix(name, "process.") {
			scopedVariable := value.(eval.ScopedVariable)
			if !scopedVariable.IsMutable() {
				continue
			}

			e.probe.Walk(func(entry *model.ProcessCacheEntry) {
				ctx := preparator.get(func(event *model.Event) {
					event.ProcessCacheEntry = entry
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx, true) // for status, let's not follow inheritance
				if !found {
					return
				}

				scopedName := fmt.Sprintf("%s.%d", name, entry.Pid)
				scopedValue := fmt.Sprintf("%+v", value)
				seclVariables[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: scopedValue,
				}
			})
		} else if strings.Contains(name, ".") { // other scopes
			continue
		} else { // global variables
			value, found := value.(eval.Variable).GetValue()
			if !found {
				continue
			}
			scopedValue := fmt.Sprintf("%+v", value)
			seclVariables[name] = &api.SECLVariableState{
				Name:  name,
				Value: scopedValue,
			}
		}
	}
}

func (e *RuleEngine) gatherDefaultPolicyProviders() []rules.PolicyProvider {
	var policyProviders []rules.PolicyProvider

	// Create and store bundled policy provider
	e.bundledProvider = bundled.NewPolicyProvider(e.config)
	policyProviders = append(policyProviders, e.bundledProvider)

	// add remote config as config provider if enabled.
	if e.config.RemoteConfigurationEnabled {
		rcPolicyProvider, err := rconfig.NewRCPolicyProvider(e.config.RemoteConfigurationDumpPolicies, e.rcStateCallback, e.ipc)
		if err != nil {
			seclog.Errorf("will be unable to load remote policies: %s", err)
		} else {
			policyProviders = append(policyProviders, rcPolicyProvider)
		}
	}

	// directory policy provider
	if provider, err := rules.NewPoliciesDirProvider(e.config.PoliciesDir); err != nil {
		seclog.Errorf("failed to load local policies: %s", err)
	} else {
		policyProviders = append(policyProviders, provider)
	}

	return policyProviders
}

func (e *RuleEngine) rcStateCallback(state bool) {
	if state {
		seclog.Infof("Connection to remote config established")
	} else {
		seclog.Infof("Connection to remote config lost")
	}
	e.probe.EnableEnforcement(state)
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (e *RuleEngine) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if e.reloading.Load() {
		return
	}

	e.probe.OnNewDiscarder(rs, event.(*model.Event), field, eventType)
}

// RuleMatch is called by the ruleset when a rule matches
func (e *RuleEngine) RuleMatch(ctx *eval.Context, rule *rules.Rule, event eval.Event) bool {
	ev := event.(*model.Event)

	// add matched rules to ensure that this information is available in activity dumps
	if ev.ProcessContext.Process.ContainerContext.ContainerID != "" && (e.config.ActivityDumpTagRulesEnabled || e.config.AnomalyDetectionTagRulesEnabled) {
		ev.Rules = append(ev.Rules, model.NewMatchedRule(rule.Def.ID, rule.Def.Version, rule.Def.Tags, rule.Policy.Name, rule.Policy.Version))
	}

	e.probe.HandleActions(rule, event)

	if rule.Def.Silent {
		return false
	}

	// ensure that all the fields are resolved before sending
	ev.FieldHandlers.ResolveContainerTags(ev, &ev.ProcessContext.Process.ContainerContext)

	// do not send event if a anomaly detection event will be sent
	if e.config.AnomalyDetectionSilentRuleEventsEnabled && ev.IsAnomalyDetectionEvent() {
		return false
	}

	// needs to be resolved here, outside of the callback as using process tree
	// which can be modified during queuing
	service := e.probe.GetService(ev)

	var extTagsCb func() ([]string, bool)

	if !ev.ProcessContext.Process.ContainerContext.IsNull() {
		// copy the container ID here to avoid later data race
		containerID := ev.ProcessContext.Process.ContainerContext.ContainerID
		retryable := time.Since(ev.ProcessContext.Process.ContainerContext.UnixCreatedAt()) < ContainerMaxTagResolutionDelay

		extTagsCb = func() ([]string, bool) {
			return e.probe.GetEventTags(containerID), retryable
		}
	}

	ev.RuleContext.Expression = rule.Expression
	ev.RuleContext.MatchingSubExprs = ctx.GetMatchingSubExprs()

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

	e.wg.Wait()
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

	if eventTypes, exists := categories[model.NetworkCategory]; exists {
		for _, eventType := range eventTypes {
			switch eventType {
			case model.RawPacketFilterEventType.String():
				enabled[eventType] = e.probe.IsNetworkRawPacketEnabled()
			case model.RawPacketActionEventType.String():
				enabled[eventType] = e.probe.IsNetworkRawPacketEnabled()
			case model.NetworkFlowMonitorEventType.String():
				enabled[eventType] = e.probe.IsNetworkFlowMonitorEnabled()
			default:
				if model.EventTypeDependsOnInterfaceTracking(eventType) {
					enabled[eventType] = e.probe.IsNetworkEnabled()
				} else {
					enabled[eventType] = true
				}
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

// SetRulesetLoadedCallback allows setting a callback called when a rule set is loaded
func (e *RuleEngine) SetRulesetLoadedCallback(cb func(es *rules.RuleSet, err *multierror.Error)) {
	e.rulesLoaded = cb
}

// HandleEvent is called by the probe when an event arrives from the kernel
func (e *RuleEngine) HandleEvent(event *model.Event) {
	// don't eval event originating from myself
	if !e.probe.Opts.DontDiscardRuntime && event.ProcessContext != nil && event.ProcessContext.Pid == e.pid {
		return
	}

	// event already marked with an error, skip it
	if event.Error != nil {
		return
	}

	// if the event should have been discarded in kernel space, we don't need to evaluate it
	if event.IsSavedByActivityDumps() {
		return
	}

	if ruleSet := e.GetRuleSet(); ruleSet != nil {
		if !ruleSet.Evaluate(event) {
			evtType := int(event.GetEventType())
			if evtType >= 0 && evtType < len(e.noMatchCounters) {
				e.noMatchCounters[evtType].Inc()
			}
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
		// Handle policy load errors
		if policyErr, ok := err.(*rules.ErrPolicyLoad); ok {
			// Empty policies are expected in some cases
			if errors.Is(policyErr.Err, rules.ErrPolicyIsEmpty) {
				seclog.Warnf(msg, policyErr.Error())
			} else {
				seclog.Errorf(msg, policyErr.Error())
			}
			continue
		}

		// Handle rule load errors
		if ruleErr, ok := err.(*rules.ErrRuleLoad); ok {
			// Some rule errors are accepted and should only generate warnings
			if errors.Is(ruleErr.Err, rules.ErrEventTypeNotEnabled) || errors.Is(ruleErr.Err, rules.ErrRuleAgentFilter) {
				seclog.Warnf(msg, ruleErr.Error())
			} else {
				seclog.Errorf(msg, ruleErr.Error())
			}
			continue
		}

		// Handle all other errors
		seclog.Errorf(msg, err.Error())
	}
}

func getPoliciesVersions(rs *rules.RuleSet, includeInternalPolicies bool) []string {
	var versions []string

	cache := make(map[string]bool)
	for _, rule := range rs.GetRules() {
		for pInfo := range rule.Policies(includeInternalPolicies) {
			version := pInfo.Version
			if _, exists := cache[version]; !exists {
				cache[version] = true
				versions = append(versions, version)
			}
		}
	}

	return versions
}
