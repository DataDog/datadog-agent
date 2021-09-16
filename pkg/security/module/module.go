// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sapi "github.com/DataDog/datadog-agent/pkg/security/api"
	sconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/statsd"
)

// Module represents the system-probe module for the runtime security agent
type Module struct {
	sync.RWMutex
	wg               sync.WaitGroup
	probe            *sprobe.Probe
	config           *sconfig.Config
	ruleSets         [2]*rules.RuleSet
	currentRuleSet   uint64
	reloading        uint64
	statsdClient     *statsd.Client
	apiServer        *APIServer
	grpcServer       *grpc.Server
	listener         net.Listener
	rateLimiter      *RateLimiter
	sigupChan        chan os.Signal
	ctx              context.Context
	cancelFnc        context.CancelFunc
	rulesLoaded      func(rs *rules.RuleSet)
	policiesVersions []string

	selfTester *SelfTester
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

	m.probe.SetEventHandler(m)

	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.probe.Init(m.statsdClient); err != nil {
		return errors.Wrap(err, "failed to init probe")
	}

	return nil
}

// Start the module
func (m *Module) Start() error {
	// start the manager and its probes / perf maps
	if err := m.probe.Start(); err != nil {
		return errors.Wrap(err, "failed to start probe")
	}

	// fetch the current state of the system (example: mount points, running processes, ...) so that our user space
	// context is ready when we start the probes
	if err := m.probe.Snapshot(); err != nil {
		return err
	}

	if err := m.Reload(); err != nil {
		return err
	}

	m.wg.Add(1)
	go m.metricsSender()

	signal.Notify(m.sigupChan, syscall.SIGHUP)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		for range m.sigupChan {
			log.Info("Reload configuration")

			if err := m.Reload(); err != nil {
				log.Errorf("failed to reload configuration: %s", err)
			}
		}
	}()

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

	if m.config.RuntimeEnabled {
		if eventTypes, exists := categories[model.RuntimeCategory]; exists {
			for _, eventType := range eventTypes {
				enabled[eventType] = true
			}
		}
	}

	return enabled
}

func logMultiErrors(msg string, m *multierror.Error) {
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

// Reload the rule set
func (m *Module) Reload() error {
	m.Lock()
	defer m.Unlock()

	atomic.StoreUint64(&m.reloading, 1)
	defer atomic.StoreUint64(&m.reloading, 0)

	policiesDir := m.config.PoliciesDir
	rsa := sprobe.NewRuleSetApplier(m.config, m.probe)

	newRuleSetOpts := func() *rules.Opts {
		return rules.NewOptsWithParams(
			model.SECLConstants,
			sprobe.SupportedDiscarders,
			m.getEventTypeEnabled(),
			sprobe.AllCustomRuleIDs(),
			model.SECLLegacyAttributes,
			&seclog.PatternLogger{})
	}

	ruleSet := m.probe.NewRuleSet(newRuleSetOpts())

	loadErr := rules.LoadPolicies(policiesDir, ruleSet)

	model := &model.Model{}
	approverRuleSet := rules.NewRuleSet(model, model.NewEvent, newRuleSetOpts())
	loadApproversErr := rules.LoadPolicies(policiesDir, approverRuleSet)

	if loadErr.ErrorOrNil() != nil {
		logMultiErrors("error while loading policies: %+v", loadErr)
	} else if loadApproversErr.ErrorOrNil() != nil {
		logMultiErrors("error while loading policies for Approvers: %+v", loadApproversErr)
	}

	monitor := m.probe.GetMonitor()
	ruleSetLoadedReport := monitor.PrepareRuleSetLoadedReport(ruleSet, loadErr)

	if m.selfTester != nil {
		if err := m.selfTester.CreateTargetFileIfNeeded(); err != nil {
			log.Errorf("failed to create self-test target file: %+v", err)
		}
		m.selfTester.AddSelfTestRulesToRuleSets(ruleSet, approverRuleSet)
	}

	approvers, err := approverRuleSet.GetApprovers(sprobe.GetCapababilities())
	if err != nil {
		return err
	}

	m.policiesVersions = getPoliciesVersions(ruleSet)

	ruleSet.AddListener(m)
	if m.rulesLoaded != nil {
		m.rulesLoaded(ruleSet)
	}

	currentRuleSet := 1 - atomic.LoadUint64(&m.currentRuleSet)
	m.ruleSets[currentRuleSet] = ruleSet
	atomic.StoreUint64(&m.currentRuleSet, currentRuleSet)

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

	// report that a new policy was loaded
	monitor.ReportRuleSetLoaded(ruleSetLoadedReport)

	return nil
}

// Close the module
func (m *Module) Close() {
	close(m.sigupChan)
	m.cancelFnc()

	if m.grpcServer != nil {
		m.grpcServer.Stop()
	}

	if m.listener != nil {
		m.listener.Close()
		os.Remove(m.config.SocketPath)
	}

	if m.selfTester != nil {
		_ = m.selfTester.Cleanup()
	}

	m.probe.Close()

	m.wg.Wait()
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (m *Module) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if atomic.LoadUint64(&m.reloading) == 1 {
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

	if m.selfTester != nil {
		m.selfTester.SendEventIfExpecting(rule, event)
	}
	m.SendEvent(rule, event, extTagsCb, service)
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
	return m.ruleSets[atomic.LoadUint64(&m.currentRuleSet)]
}

// SetRulesetLoadedCallback allows setting a callback called when a rule set is loaded
func (m *Module) SetRulesetLoadedCallback(cb func(rs *rules.RuleSet)) {
	m.rulesLoaded = cb
}

// NewModule instantiates a runtime security system-probe module
func NewModule(cfg *sconfig.Config) (module.Module, error) {
	var statsdClient *statsd.Client
	var err error
	if cfg != nil {
		statsdAddr := os.Getenv("STATSD_URL")
		if statsdAddr == "" {
			statsdAddr = cfg.StatsdAddr
		}

		if statsdClient, err = statsd.New(statsdAddr); err != nil {
			return nil, err
		}
	} else {
		log.Warn("metrics won't be sent to DataDog")
	}

	probe, err := sprobe.NewProbe(cfg, statsdClient)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	// custom limiters
	limits := make(map[rules.RuleID]Limit)

	var selfTester *SelfTester
	if cfg.SelfTestEnabled {
		selfTester = NewSelfTester()
	}

	m := &Module{
		config:         cfg,
		probe:          probe,
		statsdClient:   statsdClient,
		apiServer:      NewAPIServer(cfg, probe, statsdClient),
		grpcServer:     grpc.NewServer(),
		rateLimiter:    NewRateLimiter(statsdClient, LimiterOpts{Limits: limits}),
		sigupChan:      make(chan os.Signal, 1),
		currentRuleSet: 1,
		ctx:            ctx,
		cancelFnc:      cancelFnc,
		selfTester:     selfTester,
	}
	m.apiServer.module = m

	seclog.SetPatterns(cfg.LogPatterns)

	sapi.RegisterSecurityModuleServer(m.grpcServer, m.apiServer)

	return m, nil
}
