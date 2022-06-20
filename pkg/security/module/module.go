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
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sapi "github.com/DataDog/datadog-agent/pkg/security/api"
	sconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/selftests"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	statsdPoolSize = 64
)

// Opts define module options
type Opts struct {
	StatsdClient statsd.ClientInterface
}

// Module represents the system-probe module for the runtime security agent
type Module struct {
	sync.RWMutex
	wg               sync.WaitGroup
	probe            *sprobe.Probe
	config           *sconfig.Config
	currentRuleSet   *atomic.Value
	reloading        *atomic.Bool
	statsdClient     statsd.ClientInterface
	apiServer        *APIServer
	grpcServer       *grpc.Server
	listener         net.Listener
	rateLimiter      *RateLimiter
	sigupChan        chan os.Signal
	ctx              context.Context
	cancelFnc        context.CancelFunc
	rulesLoaded      func(rs *rules.RuleSet, err *multierror.Error)
	policiesVersions []string
	policyProviders  []rules.PolicyProvider
	policyLoader     *rules.PolicyLoader
	selfTester       *selftests.SelfTester
}

// Register the runtime security agent module
func (m *Module) Register(_ *module.Router) error {
	if err := m.Init(); err != nil {
		return err
	}

	return m.Start()
}

// Init initializes the module
func (m *Module) Init() error {
	// force socket cleanup of previous socket not cleanup
	os.Remove(m.config.SocketPath)

	ln, err := net.Listen("unix", m.config.SocketPath)
	if err != nil {
		return errors.Wrap(err, "unable to register security runtime module")
	}
	if err := os.Chmod(m.config.SocketPath, 0700); err != nil {
		return errors.Wrap(err, "unable to register security runtime module")
	}

	m.listener = ln

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		if err := m.grpcServer.Serve(ln); err != nil {
			log.Error(err)
		}
	}()

	// start api server
	m.apiServer.Start(m.ctx)

	m.probe.AddEventHandler(model.UnknownEventType, m)

	// initialize extra event monitors
	if m.config.EventMonitoring {
		InitEventMonitors(m)
	}

	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.probe.Init(); err != nil {
		return errors.Wrap(err, "failed to init probe")
	}

	// policy loader
	m.policyLoader = rules.NewPolicyLoader()

	return nil
}

// Start the module
func (m *Module) Start() error {
	// setup the manager and its probes / perf maps
	if err := m.probe.Setup(); err != nil {
		return errors.Wrap(err, "failed to setup probe")
	}

	// fetch the current state of the system (example: mount points, running processes, ...) so that our user space
	// context is ready when we start the probes
	if err := m.probe.Snapshot(); err != nil {
		return err
	}

	m.probe.Start()

	// runtime security is disabled but might be used by other component like process
	if !m.config.IsEnabled() {
		return nil
	}

	if m.config.SelfTestEnabled && m.selfTester != nil {
		_ = m.RunSelfTest(true, false)
	}

	var policyProviders []rules.PolicyProvider

	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		log.Errorf("failed to parse agent version: %v", err)
	}

	// directory policy provider
	if provider, err := rules.NewPoliciesDirProvider(m.config.PoliciesDir, m.config.WatchPoliciesDir, agentVersion); err != nil {
		log.Errorf("failed to load policies: %s", err)
	} else {
		policyProviders = append(policyProviders, provider)
	}

	// add remote config as config provider if enabled
	if m.config.RemoteConfigurationEnabled {
		rcPolicyProvider, err := rconfig.NewRCPolicyProvider("security-agent", agentVersion)
		if err != nil {
			log.Errorf("will be unable to load remote policy: %s", err)
		} else {
			policyProviders = append(policyProviders, rcPolicyProvider)
		}
	}

	if err := m.LoadPolicies(policyProviders, true); err != nil {
		log.Errorf("failed to load policies: %s", err)
	}

	m.wg.Add(1)
	go m.metricsSender()

	signal.Notify(m.sigupChan, syscall.SIGHUP)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		for range m.sigupChan {
			if err := m.ReloadPolicies(); err != nil {
				log.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		for range m.policyLoader.NewPolicyReady() {
			if err := m.ReloadPolicies(); err != nil {
				log.Errorf("failed to reload policies: %s", err)
			}
		}
	}()

	for _, provider := range m.policyProviders {
		provider.Start()
	}

	return nil
}

func (m *Module) displayReport(report *sprobe.Report) {
	content, _ := json.Marshal(report)
	log.Debugf("Policy report: %s", content)
}

func (m *Module) getEventTypeEnabled() map[eval.EventType]bool {
	enabled := make(map[eval.EventType]bool)

	categories := model.GetEventTypePerCategory()

	if m.config.FIMEnabled {
		if eventTypes, exists := categories[model.FIMCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	if m.config.NetworkEnabled {
		if eventTypes, exists := categories[model.NetworkCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	if m.config.RuntimeEnabled {
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
func (m *Module) ReloadPolicies() error {
	log.Info("reload policies")

	return m.LoadPolicies(m.policyProviders, true)
}

// LoadPolicies loads the policies
func (m *Module) LoadPolicies(policyProviders []rules.PolicyProvider, sendLoadedReport bool) error {
	log.Info("load policies")

	m.Lock()
	defer m.Unlock()

	m.reloading.Store(true)
	defer m.reloading.Store(false)

	rsa := sprobe.NewRuleSetApplier(m.config, m.probe)

	probeVariables := make(map[string]eval.VariableValue, len(model.SECLVariables))
	for name, value := range model.SECLVariables {
		probeVariables[name] = value
	}

	var opts rules.Opts
	opts.
		WithConstants(model.SECLConstants).
		WithVariables(probeVariables).
		WithSupportedDiscarders(sprobe.SupportedDiscarders).
		WithEventTypeEnabled(m.getEventTypeEnabled()).
		WithReservedRuleIDs(sprobe.AllCustomRuleIDs()).
		WithLegacyFields(model.SECLLegacyFields).
		WithStateScopes(map[rules.Scope]rules.VariableProviderFactory{
			"process": func() rules.VariableProvider {
				return eval.NewScopedVariables(func(ctx *eval.Context) unsafe.Pointer {
					return unsafe.Pointer(&(*model.Event)(ctx.Object).ProcessContext)
				}, nil)
			},
		}).
		WithLogger(&seclog.PatternLogger{})

	// approver ruleset
	model := &model.Model{}
	approverRuleSet := rules.NewRuleSet(model, model.NewEvent, &opts)

	// switch SECLVariables to use the real Event structure and not the mock model.Event one
	opts.WithVariables(sprobe.SECLVariables)
	opts.WithStateScopes(map[rules.Scope]rules.VariableProviderFactory{
		"process": m.probe.GetResolvers().ProcessResolver.NewProcessVariables,
	})

	// standard ruleset
	ruleSet := m.probe.NewRuleSet(&opts)

	// load policies
	m.policyLoader.SetProviders(policyProviders)

	loadErrs := approverRuleSet.LoadPolicies(m.policyLoader)
	loadApproversErrs := ruleSet.LoadPolicies(m.policyLoader)

	// non fatal error, just log
	if loadErrs.ErrorOrNil() != nil {
		logLoadingErrors("error while loading policies: %+v", loadErrs)
	} else if loadApproversErrs.ErrorOrNil() != nil {
		logLoadingErrors("error while loading policies for Approvers: %+v", loadApproversErrs)
	}

	approvers, err := approverRuleSet.GetApprovers(sprobe.GetCapababilities())
	if err != nil {
		return err
	}

	// update current policies related module attributes
	m.policiesVersions = getPoliciesVersions(ruleSet)
	m.policyProviders = policyProviders
	m.currentRuleSet.Store(ruleSet)

	// notify listeners
	if m.rulesLoaded != nil {
		m.rulesLoaded(ruleSet, loadErrs)
	}

	// add module as listener for ruleset events
	ruleSet.AddListener(m)

	// analyze the ruleset, push default policies in the kernel and generate the policy report
	report, err := rsa.Apply(ruleSet, approvers)
	if err != nil {
		return err
	}

	// full list of IDs, user rules + custom
	var ruleIDs []rules.RuleID
	ruleIDs = append(ruleIDs, ruleSet.ListRuleIDs()...)
	ruleIDs = append(ruleIDs, sprobe.AllCustomRuleIDs()...)

	m.apiServer.Apply(ruleIDs)
	m.rateLimiter.Apply(ruleIDs)

	m.displayReport(report)

	if sendLoadedReport {
		// report that a new policy was loaded
		monitor := m.probe.GetMonitor()
		ruleSetLoadedReport := monitor.PrepareRuleSetLoadedReport(ruleSet, loadErrs)
		monitor.ReportRuleSetLoaded(ruleSetLoadedReport)
	}

	return nil
}

// Close the module
func (m *Module) Close() {
	close(m.sigupChan)

	for _, provider := range m.policyProviders {
		_ = provider.Close()
	}

	// close the policy loader and all the related providers
	m.policyLoader.Close()

	m.cancelFnc()

	if m.grpcServer != nil {
		m.grpcServer.Stop()
	}

	if m.listener != nil {
		m.listener.Close()
		os.Remove(m.config.SocketPath)
	}

	if m.selfTester != nil {
		_ = m.selfTester.Close()
	}

	m.probe.Close()

	m.wg.Wait()
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (m *Module) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if m.reloading.Load() {
		return
	}

	if err := m.probe.OnNewDiscarder(rs, event.(*sprobe.Event), field, eventType); err != nil {
		seclog.Trace(err)
	}
}

// HandleEvent is called by the probe when an event arrives from the kernel
func (m *Module) HandleEvent(event *sprobe.Event) {
	if ruleSet := m.GetRuleSet(); ruleSet != nil {
		ruleSet.Evaluate(event)
	}
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (m *Module) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
	m.SendEvent(rule, event, func() []string { return nil }, "")
}

// RuleMatch is called by the ruleset when a rule matches
func (m *Module) RuleMatch(rule *rules.Rule, event eval.Event) {
	// prepare the event
	m.probe.OnRuleMatch(rule, event.(*sprobe.Event))

	// needs to be resolved here, outside of the callback as using process tree
	// which can be modified during queuing
	service := event.(*sprobe.Event).GetProcessServiceTag()

	id := event.(*sprobe.Event).ContainerContext.ID

	extTagsCb := func() []string {
		var tags []string

		// check from tagger
		if service == "" {
			service = m.probe.GetResolvers().TagsResolver.GetValue(id, "service")
		}

		if service == "" {
			service = m.config.HostServiceName
		}

		return append(tags, m.probe.GetResolvers().TagsResolver.Resolve(id)...)
	}

	// send if not selftest related events
	if m.selfTester == nil || !m.selfTester.IsExpectedEvent(rule, event) {
		m.SendEvent(rule, event, extTagsCb, service)
	}
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
func (m *Module) SendEvent(rule *rules.Rule, event Event, extTagsCb func() []string, service string) {
	if m.rateLimiter.Allow(rule.ID) {
		m.apiServer.SendEvent(rule, event, extTagsCb, service)
	} else {
		seclog.Tracef("Event on rule %s was dropped due to rate limiting", rule.ID)
	}
}

func (m *Module) metricsSender() {
	defer m.wg.Done()

	statsTicker := time.NewTicker(m.config.StatsPollingInterval)
	defer statsTicker.Stop()

	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-statsTicker.C:
			if os.Getenv("RUNTIME_SECURITY_TESTSUITE") == "true" {
				continue
			}

			if err := m.probe.SendStats(); err != nil {
				log.Debug(err)
			}
			if err := m.rateLimiter.SendStats(); err != nil {
				log.Debug(err)
			}
			if err := m.apiServer.SendStats(); err != nil {
				log.Debug(err)
			}
		case <-heartbeatTicker.C:
			tags := []string{fmt.Sprintf("version:%s", version.AgentVersion)}

			m.RLock()
			for _, version := range m.policiesVersions {
				tags = append(tags, fmt.Sprintf("policies_version:%s", version))
			}
			m.RUnlock()

			if m.config.RuntimeEnabled {
				_ = m.statsdClient.Gauge(metrics.MetricSecurityAgentRuntimeRunning, 1, tags, 1)
			} else if m.config.FIMEnabled {
				_ = m.statsdClient.Gauge(metrics.MetricSecurityAgentFIMRunning, 1, tags, 1)
			}

			// Event monitoring may run independently of CWS products
			if m.config.EventMonitoring {
				_ = m.statsdClient.Gauge(metrics.MetricEventMonitoringRunning, 1, tags, 1)
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// GetStats returns statistics about the module
func (m *Module) GetStats() map[string]interface{} {
	debug := map[string]interface{}{}

	if m.probe != nil {
		debug["probe"] = m.probe.GetDebugStats()
	} else {
		debug["probe"] = "not_running"
	}

	return debug
}

// GetProbe returns the module's probe
func (m *Module) GetProbe() *sprobe.Probe {
	return m.probe
}

// GetRuleSet returns the set of loaded rules
func (m *Module) GetRuleSet() (rs *rules.RuleSet) {
	if ruleSet := m.currentRuleSet.Load(); ruleSet != nil {
		return ruleSet.(*rules.RuleSet)
	}
	return nil
}

// SetRulesetLoadedCallback allows setting a callback called when a rule set is loaded
func (m *Module) SetRulesetLoadedCallback(cb func(rs *rules.RuleSet, err *multierror.Error)) {
	m.rulesLoaded = cb
}

func getStatdClient(cfg *sconfig.Config, opts ...Opts) (statsd.ClientInterface, error) {
	if len(opts) != 0 && opts[0].StatsdClient != nil {
		return opts[0].StatsdClient, nil
	}

	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = cfg.StatsdAddr
	}

	return statsd.New(statsdAddr, statsd.WithBufferPoolSize(statsdPoolSize))
}

// NewModule instantiates a runtime security system-probe module
func NewModule(cfg *sconfig.Config, opts ...Opts) (module.Module, error) {
	statsdClient, err := getStatdClient(cfg, opts...)
	if err != nil {
		return nil, err
	}

	probe, err := sprobe.NewProbe(cfg, statsdClient)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	// custom limiters
	limits := make(map[rules.RuleID]Limit)

	selfTester, err := selftests.NewSelfTester()
	if err != nil {
		log.Errorf("unable to instantiate self tests: %s", err)
	}

	m := &Module{
		config:         cfg,
		probe:          probe,
		currentRuleSet: new(atomic.Value),
		reloading:      atomic.NewBool(false),
		statsdClient:   statsdClient,
		apiServer:      NewAPIServer(cfg, probe, statsdClient),
		grpcServer:     grpc.NewServer(),
		rateLimiter:    NewRateLimiter(statsdClient, LimiterOpts{Limits: limits}),
		sigupChan:      make(chan os.Signal, 1),
		ctx:            ctx,
		cancelFnc:      cancelFnc,
		selfTester:     selfTester,
	}
	m.apiServer.module = m

	seclog.SetPatterns(cfg.LogPatterns...)
	seclog.SetTags(cfg.LogTags...)

	sapi.RegisterSecurityModuleServer(m.grpcServer, m.apiServer)

	return m, nil
}

// RunSelfTest runs the self tests
func (m *Module) RunSelfTest(sendLoadedReport bool, thenRevertPolicies bool) error {
	prevProviders, providers := m.policyProviders, m.policyProviders

	// add selftests as provider
	providers = append(providers, m.selfTester)
	if thenRevertPolicies {
		defer func() {
			if err := m.LoadPolicies(prevProviders, false); err != nil {
				log.Errorf("failed to load policies: %s", err)
			}
		}()
	}

	if err := m.LoadPolicies(providers, false); err != nil {
		return err
	}

	success, fails, err := m.selfTester.RunSelfTest()
	if err != nil {
		return err
	}

	// send the report
	monitor := m.probe.GetMonitor()
	monitor.ReportSelfTest(success, fails)
	return err
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
		log.Errorf(msg, m.Error())
	} else {
		log.Warnf(msg, m.Error())
	}
}
