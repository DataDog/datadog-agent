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

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sapi "github.com/DataDog/datadog-agent/pkg/security/api"
	sconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	agentLogger "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
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
	probe          *sprobe.Probe
	config         *sconfig.Config
	ruleSets       [2]*rules.RuleSet
	currentRuleSet uint64
	reloading      uint64
	statsdClient   *statsd.Client
	apiServer      *APIServer
	grpcServer     *grpc.Server
	listener       net.Listener
	rateLimiter    *RateLimiter
	sigupChan      chan os.Signal
	ctx            context.Context
	cancelFnc      context.CancelFunc
}

// Register the runtime security agent module
func (m *Module) Register(_ *module.Router) error {
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

	go func() {
		if err := m.grpcServer.Serve(ln); err != nil {
			log.Error(err)
		}
	}()

	// start api server
	m.apiServer.Start(m.ctx)

	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.probe.Init(m.statsdClient); err != nil {
		return errors.Wrap(err, "failed to init probe")
	}

	// start the manager and its probes / perf maps
	if err := m.probe.Start(); err != nil {
		return errors.Wrap(err, "failed to start probe")
	}

	// fetch the current state of the system (example: mount points, running processes, ...) so that our user space
	// context is ready when we start the probes
	if err := m.probe.Snapshot(); err != nil {
		return err
	}

	m.probe.SetEventHandler(m)

	if err := m.Reload(); err != nil {
		return err
	}

	go m.metricsSender()

	signal.Notify(m.sigupChan, syscall.SIGHUP)

	go func() {
		for range m.sigupChan {
			log.Info("Reload configuration")

			if err := m.Reload(); err != nil {
				log.Errorf("failed to reload configuration: %s", err)
			}
		}
	}()

	return nil
}

func (m *Module) displayReport(report *probe.Report) {
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

// Reload the rule set
func (m *Module) Reload() error {
	m.Lock()
	defer m.Unlock()

	atomic.StoreUint64(&m.reloading, 1)
	defer atomic.StoreUint64(&m.reloading, 0)

	rsa := sprobe.NewRuleSetApplier(m.config, m.probe)

	newRuleSetOpts := func() *rules.Opts {
		return rules.NewOptsWithParams(
			model.SECLConstants,
			sprobe.SupportedDiscarders,
			m.getEventTypeEnabled(),
			sprobe.AllCustomRuleIDs(),
			model.SECLLegacyAttributes,
			agentLogger.DatadogAgentLogger{})
	}

	ruleSet := m.probe.NewRuleSet(newRuleSetOpts())

	loadErr := rules.LoadPolicies(m.config.PoliciesDir, ruleSet)
	if loadErr.ErrorOrNil() != nil {
		log.Errorf("error while loading policies: %+v", loadErr.Error())
	}

	model := &model.Model{}
	approverRuleSet := rules.NewRuleSet(model, model.NewEvent, newRuleSetOpts())
	loadErr = rules.LoadPolicies(m.config.PoliciesDir, approverRuleSet)
	if loadErr.ErrorOrNil() != nil {
		log.Errorf("error while loading policies: %+v", loadErr.Error())
	}

	approvers, err := approverRuleSet.GetApprovers(sprobe.GetCapababilities())
	if err != nil {
		return err
	}

	atomic.StoreUint64(&m.currentRuleSet, 1-m.currentRuleSet)
	m.ruleSets[m.currentRuleSet] = ruleSet

	// analyze the ruleset, push default policies in the kernel and generate the policy report
	report, err := rsa.Apply(ruleSet, approvers)
	if err != nil {
		return err
	}

	ruleSet.AddListener(m)

	// full list of IDs, user rules + custom
	var ruleIDs []rules.RuleID
	ruleIDs = append(ruleIDs, ruleSet.ListRuleIDs()...)
	ruleIDs = append(ruleIDs, sprobe.AllCustomRuleIDs()...)

	m.apiServer.Apply(ruleIDs)
	m.rateLimiter.Apply(ruleIDs)

	m.displayReport(report)

	// report that a new policy was loaded
	monitor := m.probe.GetMonitor()
	monitor.ReportRuleSetLoaded(ruleSet, loadErr)

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

	m.probe.Close()
}

// EventDiscarderFound is called by the ruleset when a new discarder discovered
func (m *Module) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if atomic.LoadUint64(&m.reloading) == 1 {
		return
	}

	if err := m.probe.OnNewDiscarder(rs, event.(*probe.Event), field, eventType); err != nil {
		log.Trace(err)
	}
}

// HandleEvent is called by the probe when an event arrives from the kernel
func (m *Module) HandleEvent(event *sprobe.Event) {
	if ruleSet := m.ruleSets[atomic.LoadUint64(&m.currentRuleSet)]; ruleSet != nil {
		ruleSet.Evaluate(event)
	}
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (m *Module) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
	m.SendEvent(rule, event, func() []string { return nil })
}

// RuleMatch is called by the ruleset when a rule matches
func (m *Module) RuleMatch(rule *rules.Rule, event eval.Event) {
	// prepare the event
	m.probe.OnRuleMatch(rule, event.(*probe.Event))

	// needs to be resolved here, outside of the callback as using process tree
	// which can be modified during queuing
	service := event.(*probe.Event).GetProcessServiceTag()

	id := event.(*probe.Event).ContainerContext.ID

	extTagsCb := func() []string {
		var tags []string

		// check from tagger
		if service == "" {
			service = m.probe.GetResolvers().TagsResolver.GetValue(id, "service")
		}

		if service == "" {
			service = m.config.HostServiceName
		}

		if service != "" {
			tags = append(tags, "service:"+service)
		}
		return append(tags, m.probe.GetResolvers().TagsResolver.Resolve(id)...)
	}

	m.SendEvent(rule, event, extTagsCb)
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
func (m *Module) SendEvent(rule *rules.Rule, event Event, extTagsCb func() []string) {
	if m.rateLimiter.Allow(rule.ID) {
		m.apiServer.SendEvent(rule, event, extTagsCb)
	} else {
		log.Tracef("Event on rule %s was dropped due to rate limiting", rule.ID)
	}
}

func (m *Module) metricsSender() {
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
func (m *Module) GetRuleSet() *rules.RuleSet {
	return m.ruleSets[atomic.LoadUint64(&m.currentRuleSet)]
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
	}

	sapi.RegisterSecurityModuleServer(m.grpcServer, m.apiServer)

	return m, nil
}
