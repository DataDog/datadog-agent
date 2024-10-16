// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/selftests"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	rulesmodule "github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	maxSelftestRetry = 3
	selftestDelay    = 5 * time.Second
)

// CWSConsumer represents the system-probe module for the runtime security agent
type CWSConsumer struct {
	sync.RWMutex
	config       *config.RuntimeSecurityConfig
	probe        *probe.Probe
	statsdClient statsd.ClientInterface

	// internals
	wg            sync.WaitGroup
	ctx           context.Context
	cancelFnc     context.CancelFunc
	apiServer     *APIServer
	rateLimiter   *events.RateLimiter
	sendStatsChan chan chan bool
	eventSender   events.EventSender
	grpcServer    *GRPCServer
	ruleEngine    *rulesmodule.RuleEngine
	selfTester    *selftests.SelfTester
	selfTestRetry *atomic.Int32
	reloader      ReloaderInterface
	crtelemetry   *telemetry.ContainersRunningTelemetry
}

// NewCWSConsumer initializes the module with options
func NewCWSConsumer(evm *eventmonitor.EventMonitor, cfg *config.RuntimeSecurityConfig, wmeta workloadmeta.Component, opts Opts) (*CWSConsumer, error) {
	crtelemetry, err := telemetry.NewContainersRunningTelemetry(cfg, evm.StatsdClient, wmeta)
	if err != nil {
		return nil, err
	}

	var selfTester *selftests.SelfTester
	if cfg.SelfTestEnabled {
		selfTester, err = selftests.NewSelfTester(cfg, evm.Probe)
		if err != nil {
			seclog.Errorf("unable to instantiate self tests: %s", err)
		}
	}

	family, address := config.GetFamilyAddress(cfg.SocketPath)

	apiServer, err := NewAPIServer(cfg, evm.Probe, opts.MsgSender, evm.StatsdClient, selfTester)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	c := &CWSConsumer{
		config:       cfg,
		probe:        evm.Probe,
		statsdClient: evm.StatsdClient,
		// internals
		ctx:           ctx,
		cancelFnc:     cancelFnc,
		apiServer:     apiServer,
		rateLimiter:   events.NewRateLimiter(cfg, evm.StatsdClient),
		sendStatsChan: make(chan chan bool, 1),
		grpcServer:    NewGRPCServer(family, address),
		selfTester:    selfTester,
		selfTestRetry: atomic.NewInt32(0),
		reloader:      NewReloader(),
		crtelemetry:   crtelemetry,
	}

	// set sender
	if opts.EventSender != nil {
		c.eventSender = opts.EventSender
	} else {
		c.eventSender = c.APIServer()
	}

	seclog.Infof("Instantiating CWS rule engine")

	var listeners []rules.RuleSetListener
	if selfTester != nil {
		listeners = append(listeners, selfTester)
	}

	c.ruleEngine, err = rulesmodule.NewRuleEngine(evm, cfg, evm.Probe, c.rateLimiter, c.apiServer, c, c.statsdClient, listeners...)
	if err != nil {
		return nil, err
	}
	c.apiServer.SetCWSConsumer(c)

	// add self test as rule provider
	if c.selfTester != nil {
		c.ruleEngine.AddPolicyProvider(c.selfTester)
	}

	if err := evm.Probe.AddCustomEventHandler(model.UnknownEventType, c); err != nil {
		return nil, err
	}

	seclog.SetPatterns(cfg.LogPatterns...)
	seclog.SetTags(cfg.LogTags...)

	api.RegisterSecurityModuleServer(c.grpcServer.server, c.apiServer)

	// platform specific initialization
	if err := c.init(evm, cfg, opts); err != nil {
		return nil, err
	}

	return c, nil
}

// ID returns id for CWS
func (c *CWSConsumer) ID() string {
	return "CWS"
}

// Start the module
func (c *CWSConsumer) Start() error {
	if err := c.grpcServer.Start(); err != nil {
		return err
	}

	if err := c.reloader.Start(); err != nil {
		return err
	}

	// start api server
	c.apiServer.Start(c.ctx)

	if err := c.ruleEngine.Start(c.ctx, c.reloader.Chan(), &c.wg); err != nil {
		return err
	}

	c.wg.Add(1)
	go c.statsSender()

	if c.crtelemetry != nil {
		// Send containers running telemetry
		go c.crtelemetry.Run(c.ctx)
	}

	seclog.Infof("runtime security started")

	// we can now wait for self test events
	cb := func(success []eval.RuleID, fails []eval.RuleID, testEvents map[eval.RuleID]*serializers.EventSerializer) {
		seclog.Debugf("self-test results : success : %v, failed : %v, retry %d/%d", success, fails, c.selfTestRetry.Load()+1, maxSelftestRetry)

		if len(fails) > 0 && c.selfTestRetry.Load() < maxSelftestRetry {
			c.selfTestRetry.Inc()

			time.Sleep(selftestDelay)

			if _, err := c.RunSelfTest(false); err != nil {
				seclog.Errorf("self-test error: %s", err)
			}
			return
		}

		if c.config.SelfTestSendReport {
			c.reportSelfTest(success, fails, testEvents)
		}
	}
	if c.selfTester != nil {
		go c.selfTester.WaitForResult(cb)
	}

	return nil
}

// PostProbeStart is called after the event stream is started
func (c *CWSConsumer) PostProbeStart() error {
	if c.selfTester != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()

			select {
			case <-c.ctx.Done():

			case <-time.After(15 * time.Second):
				if _, err := c.RunSelfTest(false); err != nil {
					seclog.Warnf("failed to run self test: %s", err)
				}
			}
		}()
	}

	return nil
}

// RunSelfTest runs the self tests
func (c *CWSConsumer) RunSelfTest(gRPC bool) (bool, error) {
	if c.config.EBPFLessEnabled && gRPC {
		return false, errors.New("self-tests through gRPC are not supported with eBPF less")
	}

	if c.selfTester == nil {
		return false, nil
	}

	if err := c.selfTester.RunSelfTest(selftests.DefaultTimeout); err != nil {
		return true, err
	}

	return true, nil
}

func (c *CWSConsumer) reportSelfTest(success []eval.RuleID, fails []eval.RuleID, testEvents map[eval.RuleID]*serializers.EventSerializer) {
	// send metric with number of success and fails
	tags := []string{
		fmt.Sprintf("success:%d", len(success)),
		fmt.Sprintf("fails:%d", len(fails)),
		fmt.Sprintf("os:%s", runtime.GOOS),
		fmt.Sprintf("arch:%s", utils.RuntimeArch()),
		fmt.Sprintf("origin:%s", c.probe.Origin()),
	}
	if err := c.statsdClient.Count(metrics.MetricSelfTest, 1, tags, 1.0); err != nil {
		seclog.Errorf("failed to send self_test metric: %s", err)
	}

	// send the custom event with the list of succeed and failed self tests
	rule, event := selftests.NewSelfTestEvent(success, fails, testEvents)
	c.SendEvent(rule, event, nil, "")
}

// Stop closes the module
func (c *CWSConsumer) Stop() {
	c.reloader.Stop()

	if c.apiServer != nil {
		c.apiServer.Stop()
	}

	c.ruleEngine.Stop()

	c.cancelFnc()
	c.wg.Wait()

	c.grpcServer.Stop()
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (c *CWSConsumer) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	c.SendEvent(rule, event, nil, "")
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
// Implements the EventSender interface
func (c *CWSConsumer) SendEvent(rule *rules.Rule, event events.Event, extTagsCb func() []string, service string) {
	if c.rateLimiter.Allow(rule.ID, event) {
		c.eventSender.SendEvent(rule, event, extTagsCb, service)
	} else {
		seclog.Tracef("Event on rule %s was dropped due to rate limiting", rule.ID)
	}
}

// APIServer returns the api server
func (c *CWSConsumer) APIServer() *APIServer {
	return c.apiServer
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
	for statsTags, counter := range c.ruleEngine.AutoSuppression.GetStats() {
		if counter > 0 {
			tags := []string{
				fmt.Sprintf("rule_id:%s", statsTags.RuleID),
				fmt.Sprintf("suppression_type:%s", statsTags.SuppressionType),
			}
			_ = c.statsdClient.Count(metrics.MetricRulesSuppressed, counter, tags, 1.0)
		}
	}
}

func (c *CWSConsumer) statsSender() {
	defer c.wg.Done()

	statsTicker := time.NewTicker(c.probe.StatsPollingInterval())
	defer statsTicker.Stop()

	for {
		select {
		case ackChan := <-c.sendStatsChan:
			c.sendStats()
			ackChan <- true
		case <-statsTicker.C:
			c.sendStats()
		case <-c.ctx.Done():
			return
		}
	}
}

// GetRuleEngine returns new current rule engine
func (c *CWSConsumer) GetRuleEngine() *rulesmodule.RuleEngine {
	return c.ruleEngine
}

// PrepareForFunctionalTests tweaks the module to be ready for functional tests
// currently it:
// - disables the container running telemetry
func (c *CWSConsumer) PrepareForFunctionalTests() {
	// no need for container running telemetry in functional tests
	c.crtelemetry = nil
}
