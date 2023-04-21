// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"

	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/selftests"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// CWSConsumer represents the system-probe module for the runtime security agent
type CWSConsumer struct {
	sync.RWMutex
	config       *config.RuntimeSecurityConfig
	probe        *probe.Probe
	statsdClient statsd.ClientInterface

	// internals
	wg               sync.WaitGroup
	ctx              context.Context
	cancelFnc        context.CancelFunc
	currentRuleSet   *atomic.Value
	reloading        *atomic.Bool
	apiServer        *APIServer
	rateLimiter      *RateLimiter
	sigupChan        chan os.Signal
	rulesLoaded      func(rs *rules.RuleSet, err *multierror.Error)
	policiesVersions []string
	policyProviders  []rules.PolicyProvider
	policyLoader     *rules.PolicyLoader
	policyOpts       rules.PolicyLoaderOpts
	selfTester       *selftests.SelfTester
	policyMonitor    *PolicyMonitor
	sendStatsChan    chan chan bool
	eventSender      EventSender
	grpcServer       *GRPCServer
}

// Init initializes the module with options
func NewCWSConsumer(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, opts ...Opts) (*CWSConsumer, error) {

	selfTester, err := selftests.NewSelfTester()
	if err != nil {
		seclog.Errorf("unable to instantiate self tests: %s", err)
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	c := &CWSConsumer{
		config:       config,
		probe:        evm.Probe,
		statsdClient: evm.StatsdClient,
		// internals
		ctx:            ctx,
		cancelFnc:      cancelFnc,
		currentRuleSet: new(atomic.Value),
		reloading:      atomic.NewBool(false),
		apiServer:      NewAPIServer(config, evm.Probe, evm.StatsdClient),
		rateLimiter:    NewRateLimiter(evm.StatsdClient),
		sigupChan:      make(chan os.Signal, 1),
		selfTester:     selfTester,
		policyMonitor:  NewPolicyMonitor(evm.StatsdClient),
		sendStatsChan:  make(chan chan bool, 1),
		grpcServer:     NewGRPCServer(config.SocketPath),
	}
	c.apiServer.cwsConsumer = c

	// set sender
	if len(opts) > 0 && opts[0].EventSender != nil {
		c.eventSender = opts[0].EventSender
	} else {
		c.eventSender = c
	}

	seclog.SetPatterns(config.LogPatterns...)
	seclog.SetTags(config.LogTags...)

	api.RegisterSecurityModuleServer(c.grpcServer.server, c.apiServer)

	// register as event handler
	if err := evm.Probe.AddEventHandler(model.UnknownEventType, c); err != nil {
		return nil, err
	}
	if err := evm.Probe.AddCustomEventHandler(model.UnknownEventType, c); err != nil {
		return nil, err
	}

	// Activity dumps related
	evm.Probe.AddActivityDumpHandler(c)

	// policy loader
	c.policyLoader = rules.NewPolicyLoader()

	return c, nil
}

// ID returns id for CWS
func (c *CWSConsumer) ID() string {
	return "CWS"
}

// Start the module
func (c *CWSConsumer) Start() error {
	err := c.grpcServer.Start()
	if err != nil {
		return err
	}

	// start api server
	c.apiServer.Start(c.ctx)

	// monitor policies
	if c.config.PolicyMonitorEnabled {
		c.policyMonitor.Start(c.ctx)
	}

	if c.config.SelfTestEnabled && c.selfTester != nil {
		if triggerred, err := c.RunSelfTest(true); err != nil {
			err = fmt.Errorf("failed to run self test: %w", err)
			if !triggerred {
				return err
			}
			seclog.Warnf("%s", err)
		}
	}

	var policyProviders []rules.PolicyProvider

	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		seclog.Errorf("failed to parse agent version: %v", err)
	}

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

	c.policyOpts = rules.PolicyLoaderOpts{
		MacroFilters: macroFilters,
		RuleFilters:  ruleFilters,
	}

	// directory policy provider
	if provider, err := rules.NewPoliciesDirProvider(c.config.PoliciesDir, c.config.WatchPoliciesDir); err != nil {
		seclog.Errorf("failed to load policies: %s", err)
	} else {
		policyProviders = append(policyProviders, provider)
	}

	// add remote config as config provider if enabled
	if c.config.RemoteConfigurationEnabled {
		rcPolicyProvider, err := rconfig.NewRCPolicyProvider("security-agent", agentVersion)
		if err != nil {
			seclog.Errorf("will be unable to load remote policy: %s", err)
		} else {
			policyProviders = append(policyProviders, rcPolicyProvider)
		}
	}

	if err := c.LoadPolicies(policyProviders, true); err != nil {
		return fmt.Errorf("failed to load policies: %s", err)
	}

	c.wg.Add(1)
	go c.statsSender()

	signal.Notify(c.sigupChan, syscall.SIGHUP)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		for range c.sigupChan {
			if err := c.ReloadPolicies(); err != nil {
				seclog.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		for range c.policyLoader.NewPolicyReady() {
			if err := c.ReloadPolicies(); err != nil {
				seclog.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	for _, provider := range c.policyProviders {
		provider.Start()
	}

	seclog.Infof("runtime security started")

	return nil
}

func (c *CWSConsumer) displayApplyRuleSetReport(report *kfilters.ApplyRuleSetReport) {
	content, _ := json.Marshal(report)
	seclog.Debugf("Policy report: %s", content)
}

func (c *CWSConsumer) getEventTypeEnabled() map[eval.EventType]bool {
	enabled := make(map[eval.EventType]bool)

	categories := model.GetEventTypePerCategory()

	if c.config.FIMEnabled {
		if eventTypes, exists := categories[model.FIMCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	if c.probe.IsNetworkEnabled() {
		if eventTypes, exists := categories[model.NetworkCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	if c.config.RuntimeEnabled {
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

func getPoliciesVersions(rs *rules.RuleSet) []string {
	var versions []string

	cache := make(map[string]bool)
	for _, rule := range rs.GetRules() {
		version := rule.Definition.Policy.Version

		if _, exists := cache[version]; !exists {
			cache[version] = true

			versions = append(versions, version)
		}
	}

	return versions
}

// ReloadPolicies reloads the policies
func (c *CWSConsumer) ReloadPolicies() error {
	seclog.Infof("reload policies")

	return c.LoadPolicies(c.policyProviders, true)
}

// LoadPolicies loads the policies
func (c *CWSConsumer) LoadPolicies(policyProviders []rules.PolicyProvider, sendLoadedReport bool) error {
	seclog.Infof("load policies")

	c.Lock()
	defer c.Unlock()

	c.reloading.Store(true)
	defer c.reloading.Store(false)

	// load policies
	c.policyLoader.SetProviders(policyProviders)

	// standard ruleset
	ruleSet := c.probe.NewRuleSet(c.getEventTypeEnabled())

	loadErrs := ruleSet.LoadPolicies(c.policyLoader, c.policyOpts)
	if loadErrs.ErrorOrNil() != nil {
		logLoadingErrors("error while loading policies: %+v", loadErrs)
	}

	// update current policies related module attributes
	c.policiesVersions = getPoliciesVersions(ruleSet)
	c.policyProviders = policyProviders
	c.currentRuleSet.Store(ruleSet)

	// notify listeners
	if c.rulesLoaded != nil {
		c.rulesLoaded(ruleSet, loadErrs)
	}

	// add module as listener for ruleset events
	ruleSet.AddListener(c)

	// analyze the ruleset, push default policies in the kernel and generate the policy report
	report, err := c.probe.ApplyRuleSet(ruleSet)
	if err != nil {
		return err
	}
	c.displayApplyRuleSetReport(report)

	// set the rate limiters
	c.rateLimiter.Apply(ruleSet, events.AllCustomRuleIDs())

	// full list of IDs, user rules + custom
	var ruleIDs []rules.RuleID
	ruleIDs = append(ruleIDs, ruleSet.ListRuleIDs()...)
	ruleIDs = append(ruleIDs, events.AllCustomRuleIDs()...)

	c.apiServer.Apply(ruleIDs)

	if sendLoadedReport {
		ReportRuleSetLoaded(c.eventSender, c.statsdClient, ruleSet, loadErrs)
		c.policyMonitor.AddPolicies(ruleSet.GetPolicies(), loadErrs)
	}

	return nil
}

// Close the module
func (c *CWSConsumer) Stop() {
	signal.Stop(c.sigupChan)
	close(c.sigupChan)

	for _, provider := range c.policyProviders {
		_ = provider.Close()
	}

	// close the policy loader and all the related providers
	if c.policyLoader != nil {
		c.policyLoader.Close()
	}

	if c.selfTester != nil {
		_ = c.selfTester.Close()
	}

	c.cancelFnc()
	c.wg.Wait()

	c.grpcServer.Stop()
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (c *CWSConsumer) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if c.reloading.Load() {
		return
	}

	c.probe.OnNewDiscarder(rs, event.(*model.Event), field, eventType)
}

// HandleEvent is called by the probe when an event arrives from the kernel
func (c *CWSConsumer) HandleEvent(event *model.Event) {
	// if the event should have been discarded in kernel space, we don't need to evaluate it
	if event.IsSavedByActivityDumps() {
		return
	}

	if ruleSet := c.GetRuleSet(); ruleSet != nil {
		if (event.SecurityProfileContext.Status.IsEnabled(model.AutoSuppression) && event.IsInProfile()) || !ruleSet.Evaluate(event) {
			ruleSet.EvaluateDiscarders(event)
		}
	}
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (c *CWSConsumer) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	c.eventSender.SendEvent(rule, event, func() []string { return nil }, "")
}

// RuleMatch is called by the ruleset when a rule matches
func (c *CWSConsumer) RuleMatch(rule *rules.Rule, event eval.Event) {
	ev := event.(*model.Event)

	// ensure that all the fields are resolved before sending
	ev.FieldHandlers.ResolveContainerID(ev, &ev.ContainerContext)
	ev.FieldHandlers.ResolveContainerTags(ev, &ev.ContainerContext)

	if ev.ContainerContext.ID != "" && c.config.ActivityDumpTagRulesEnabled {
		ev.Rules = append(ev.Rules, model.NewMatchedRule(rule.Definition.ID, rule.Definition.Version, rule.Definition.Tags, rule.Definition.Policy.Name, rule.Definition.Policy.Version))
	}
	if ok, val := rule.Definition.GetTag("ruleset"); ok && val == "threat_score" {
		return // if the triggered rule is only meant to tag secdumps, dont send it
	}

	// needs to be resolved here, outside of the callback as using process tree
	// which can be modified during queuing
	service := c.probe.GetService(ev)

	extTagsCb := func() []string {
		return c.probe.GetEventTags(ev)

	}

	// send if not selftest related events
	if c.selfTester == nil || !c.selfTester.IsExpectedEvent(rule, event, c.probe) {
		c.eventSender.SendEvent(rule, event, extTagsCb, service)
	}
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
func (c *CWSConsumer) SendEvent(rule *rules.Rule, event Event, extTagsCb func() []string, service string) {
	if c.rateLimiter.Allow(rule.ID) {
		c.apiServer.SendEvent(rule, event, extTagsCb, service)
	} else {
		seclog.Tracef("Event on rule %s was dropped due to rate limiting", rule.ID)
	}
}

// HandleActivityDump sends an activity dump to the backend
func (c *CWSConsumer) HandleActivityDump(dump *api.ActivityDumpStreamMessage) {
	c.apiServer.SendActivityDump(dump)
}

// SendStats send stats
func (c *CWSConsumer) SendStats() {
	ackChan := make(chan bool, 1)
	c.sendStatsChan <- ackChan
	<-ackChan
}

func (c *CWSConsumer) sendStats() {
	if err := c.rateLimiter.SendStats(); err != nil {
		seclog.Debugf("failed to send rate limiter stats: %s", err)
	}
	if err := c.apiServer.SendStats(); err != nil {
		seclog.Debugf("failed to send api server stats: %s", err)
	}
}

func (c *CWSConsumer) statsSender() {
	defer c.wg.Done()

	statsTicker := time.NewTicker(c.probe.StatsPollingInterval())
	defer statsTicker.Stop()

	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case ackChan := <-c.sendStatsChan:
			c.sendStats()
			ackChan <- true
		case <-statsTicker.C:
			c.sendStats()
		case <-heartbeatTicker.C:
			tags := []string{fmt.Sprintf("version:%s", version.AgentVersion)}

			c.RLock()
			for _, version := range c.policiesVersions {
				tags = append(tags, fmt.Sprintf("policies_version:%s", version))
			}
			c.RUnlock()

			if c.config.RuntimeEnabled {
				_ = c.statsdClient.Gauge(metrics.MetricSecurityAgentRuntimeRunning, 1, tags, 1)
			} else if c.config.FIMEnabled {
				_ = c.statsdClient.Gauge(metrics.MetricSecurityAgentFIMRunning, 1, tags, 1)
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// GetRuleSet returns the set of loaded rules
func (c *CWSConsumer) GetRuleSet() (rs *rules.RuleSet) {
	if ruleSet := c.currentRuleSet.Load(); ruleSet != nil {
		return ruleSet.(*rules.RuleSet)
	}
	return nil
}

// SetRulesetLoadedCallback allows setting a callback called when a rule set is loaded
func (c *CWSConsumer) SetRulesetLoadedCallback(cb func(rs *rules.RuleSet, err *multierror.Error)) {
	c.rulesLoaded = cb
}

// RunSelfTest runs the self tests
func (c *CWSConsumer) RunSelfTest(sendLoadedReport bool) (bool, error) {
	prevProviders, providers := c.policyProviders, c.policyProviders
	if len(prevProviders) > 0 {
		defer func() {
			if err := c.LoadPolicies(prevProviders, false); err != nil {
				seclog.Errorf("failed to load policies: %s", err)
			}
		}()
	}

	// add selftests as provider
	providers = append(providers, c.selfTester)

	if err := c.LoadPolicies(providers, false); err != nil {
		return false, err
	}

	success, fails, err := c.selfTester.RunSelfTest()
	if err != nil {
		return true, err
	}

	seclog.Debugf("self-test results : success : %v, failed : %v", success, fails)

	// send the report
	if c.config.SelfTestSendReport {
		ReportSelfTest(c.eventSender, c.statsdClient, success, fails)
	}

	return true, nil
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

// UpdateEventMonitorOpts adapt the event monitor options
func UpdateEventMonitorOpts(opts *eventmonitor.Opts) {
	opts.ProbeOpts.PathResolutionEnabled = true
}
