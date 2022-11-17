// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sapi "github.com/DataDog/datadog-agent/pkg/security/api"
	sconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	statsdPoolSize = 64
)

// Module represents the system-probe module for the runtime security agent
type Module struct {
	sync.RWMutex
	wg             sync.WaitGroup
	probe          *sprobe.Probe
	config         *sconfig.Config
	currentRuleSet *atomic.Value
	reloading      *atomic.Bool
	statsdClient   statsd.ClientInterface
	//	apiServer        *APIServer
	grpcServer *grpc.Server
	listener   net.Listener
	//	rateLimiter      *RateLimiter
	sigupChan        chan os.Signal
	ctx              context.Context
	cancelFnc        context.CancelFunc
	rulesLoaded      func(rs *rules.RuleSet, err *multierror.Error)
	policiesVersions []string
	policyProviders  []rules.PolicyProvider
	policyLoader     *rules.PolicyLoader
	policyOpts       rules.PolicyLoaderOpts
	//	selfTester       *selftests.SelfTester
	//	policyMonitor    *PolicyMonitor
	sendStatsChan chan chan bool
	eventSender   EventSender
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
		return fmt.Errorf("unable to register security runtime module: %w", err)
	}
	if err := os.Chmod(m.config.SocketPath, 0700); err != nil {
		return fmt.Errorf("unable to register security runtime module: %w", err)
	}

	m.listener = ln

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		if err := m.grpcServer.Serve(ln); err != nil {
			seclog.Errorf("error launching the grpc server: %v", err)
		}
	}()
	/*
		// start api server
		m.apiServer.Start(m.ctx)

		// monitor policies
		if m.config.PolicyMonitorEnabled {
			m.policyMonitor.Start(m.ctx)
		}

		m.probe.AddEventHandler(model.UnknownEventType, m)
		m.probe.AddActivityDumpHandler(m)

		// initialize extra event monitors
		if m.config.EventMonitoring {
			InitEventMonitors(m)
		}

		// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
		// running yet.
		if err := m.probe.Init(); err != nil {
			return fmt.Errorf("failed to init probe: %w", err)
		}

		// policy loader
		m.policyLoader = rules.NewPolicyLoader()
	*/
	return nil
}

// Start the module
func (m *Module) Start() error {
	// setup the manager and its probes / perf maps
	if err := m.probe.Setup(); err != nil {
		return fmt.Errorf("failed to setup probe: %w", err)
	}

	// fetch the current state of the system (example: mount points, running processes, ...) so that our user space
	// context is ready when we start the probes
	if err := m.probe.Snapshot(); err != nil {
		return err
	}

	if err := m.probe.Start(); err != nil {
		return err
	}

	// runtime security is disabled but might be used by other component like process
	if !m.config.IsEnabled() {
		if m.config.EventMonitoring {
			// Currently select process related event type.
			// TODO external monitors should be allowed to select the event types
			return m.probe.SelectProbes([]eval.EventType{
				model.ForkEventType.String(),
				model.ExecEventType.String(),
				model.ExitEventType.String(),
			})
		}
		return nil
	}

	return nil
}

func (m *Module) displayReport(report *sprobe.Report) {
	content, _ := json.Marshal(report)
	seclog.Debugf("Policy report: %s", content)
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
	seclog.Infof("reload policies")

	return m.LoadPolicies(m.policyProviders, true)
}

func (m *Module) newRuleOpts() (opts rules.Opts) {
	//opts.
	//	WithSupportedDiscarders(sprobe.SupportedDiscarders).
	//	WithEventTypeEnabled(m.getEventTypeEnabled()).
	//	WithReservedRuleIDs(sprobe.AllCustomRuleIDs()).
	//	WithLogger(seclog.DefaultLogger)
	return
}

func (m *Module) newEvalOpts() (evalOpts eval.Opts) {
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)
	return evalOpts
}

func (m *Module) getApproverRuleset(policyProviders []rules.PolicyProvider) (*rules.RuleSet, *multierror.Error) {

	return nil, nil
}

// LoadPolicies loads the policies
func (m *Module) LoadPolicies(policyProviders []rules.PolicyProvider, sendLoadedReport bool) error {
	seclog.Infof("load policies")

	return nil
}

// Close the module
func (m *Module) Close() {
	signal.Stop(m.sigupChan)
	close(m.sigupChan)

	for _, provider := range m.policyProviders {
		_ = provider.Close()
	}

	// close the policy loader and all the related providers
	if m.policyLoader != nil {
		m.policyLoader.Close()
	}

	m.cancelFnc()

	if m.grpcServer != nil {
		m.grpcServer.Stop()
	}

	if m.listener != nil {
		m.listener.Close()
		os.Remove(m.config.SocketPath)
	}

	m.wg.Wait()

	// all the go routines should be stopped now we can safely call close the probe and remove the eBPF programs
	m.probe.Close()
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
	/*
		// if the event should have been discarded in kernel space, we don't need to evaluate it
		if event.SavedByActivityDumps {
			return
		}

		if ruleSet := m.GetRuleSet(); ruleSet != nil {
			ruleSet.Evaluate(event)
		}
	*/
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (m *Module) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
	//m.eventSender.SendEvent(rule, event, func() []string { return nil }, "")
}

// RuleMatch is called by the ruleset when a rule matches
func (m *Module) RuleMatch(rule *rules.Rule, event eval.Event) {
	/*
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
	*/
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
func (m *Module) SendEvent(rule *rules.Rule, event Event, extTagsCb func() []string, service string) {
	/*
		if m.rateLimiter.Allow(rule.ID) {
			m.apiServer.SendEvent(rule, event, extTagsCb, service)
		} else {
			seclog.Tracef("Event on rule %s was dropped due to rate limiting", rule.ID)
		}
	*/
}

// HandleActivityDump sends an activity dump to the backend
func (m *Module) HandleActivityDump(dump *sapi.ActivityDumpStreamMessage) {
	//m.apiServer.SendActivityDump(dump)
}

// SendStats send stats
func (m *Module) SendStats() {
	ackChan := make(chan bool, 1)
	m.sendStatsChan <- ackChan
	<-ackChan
}

func (m *Module) sendStats() {
	/*
		if err := m.probe.SendStats(); err != nil {
			seclog.Debugf("failed to send probe stats: %s", err)
		}
			if err := m.rateLimiter.SendStats(); err != nil {
				seclog.Debugf("failed to send rate limiter stats: %s", err)
			}
			if err := m.apiServer.SendStats(); err != nil {
				seclog.Debugf("failed to send api server stats: %s", err)
			}
	*/
}

func (m *Module) statsSender() {
	defer m.wg.Done()

	statsTicker := time.NewTicker(m.config.StatsPollingInterval)
	defer statsTicker.Stop()

	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case ackChan := <-m.sendStatsChan:
			m.sendStats()
			ackChan <- true
		case <-statsTicker.C:
			m.sendStats()
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
	/*
		if m.probe != nil {
			debug["probe"] = m.probe.GetDebugStats()
		} else {
			debug["probe"] = "not_running"
		}
	*/
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
	//limits := make(map[rules.RuleID]Limit)

	m := &Module{
		config:         cfg,
		probe:          probe,
		currentRuleSet: new(atomic.Value),
		reloading:      atomic.NewBool(false),
		statsdClient:   statsdClient,
		//apiServer:      NewAPIServer(cfg, probe, statsdClient),
		grpcServer: grpc.NewServer(),
		//rateLimiter:    NewRateLimiter(statsdClient, LimiterOpts{Limits: limits}),
		sigupChan: make(chan os.Signal, 1),
		ctx:       ctx,
		cancelFnc: cancelFnc,
		//selfTester:     selfTester,
		//policyMonitor:  NewPolicyMonitor(statsdClient),
		sendStatsChan: make(chan chan bool, 1),
	}
	//m.apiServer.module = m

	if len(opts) > 0 && opts[0].EventSender != nil {
		m.eventSender = opts[0].EventSender
	} else {
		m.eventSender = m
	}

	seclog.SetPatterns(cfg.LogPatterns...)
	seclog.SetTags(cfg.LogTags...)

	sapi.RegisterSecurityModuleServer(m.grpcServer, nil) //m.apiServer)

	return m, nil
}

// RunSelfTest runs the self tests
func (m *Module) RunSelfTest(sendLoadedReport bool) error {

	return nil
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
