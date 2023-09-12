// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/selftests"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	rulesmodule "github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
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
	reloader      ReloaderInterface
}

// NewCWSConsumer initializes the module with options
func NewCWSConsumer(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, opts Opts) (*CWSConsumer, error) {
	ctx, cancelFnc := context.WithCancel(context.Background())

	selfTester, err := selftests.NewSelfTester(evm.Probe)
	if err != nil {
		seclog.Errorf("unable to instantiate self tests: %s", err)
	}

	family, address := getFamilyAddress(config)

	c := &CWSConsumer{
		config:       config,
		probe:        evm.Probe,
		statsdClient: evm.StatsdClient,
		// internals
		ctx:           ctx,
		cancelFnc:     cancelFnc,
		apiServer:     NewAPIServer(config, evm.Probe, evm.StatsdClient, selfTester),
		rateLimiter:   events.NewRateLimiter(config, evm.StatsdClient),
		sendStatsChan: make(chan chan bool, 1),
		grpcServer:    NewGRPCServer(family, address),
		selfTester:    selfTester,
		reloader:      NewReloader(),
	}

	// set sender
	if opts.EventSender != nil {
		c.eventSender = opts.EventSender
	} else {
		c.eventSender = c
	}

	seclog.Infof("Instantiating CWS rule engine")

	c.ruleEngine, err = rulesmodule.NewRuleEngine(evm, config, evm.Probe, c.rateLimiter, c.apiServer, c.eventSender, c.statsdClient, selfTester)
	if err != nil {
		return nil, err
	}
	c.apiServer.SetCWSConsumer(c)

	if err := evm.Probe.AddCustomEventHandler(model.UnknownEventType, c); err != nil {
		return nil, err
	}

	seclog.SetPatterns(config.LogPatterns...)
	seclog.SetTags(config.LogTags...)

	api.RegisterSecurityModuleServer(c.grpcServer.server, c.apiServer)

	// platform specific initialization
	if err := c.init(evm, config, opts); err != nil {
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

	seclog.Infof("runtime security started")

	return nil
}

// PostProbeStart is called after the event stream is started
func (c *CWSConsumer) PostProbeStart() error {
	if c.config.SelfTestEnabled {
		if triggerred, err := c.RunSelfTest(true); err != nil {
			err = fmt.Errorf("failed to run self test: %w", err)
			if !triggerred {
				return err
			}
			seclog.Warnf("%s", err)
		}
	}

	return nil
}

// RunSelfTest runs the self tests
func (c *CWSConsumer) RunSelfTest(sendLoadedReport bool) (bool, error) {
	prevProviders, providers := c.ruleEngine.GetPolicyProviders(), c.ruleEngine.GetPolicyProviders()
	if len(prevProviders) > 0 {
		defer func() {
			if err := c.ruleEngine.LoadPolicies(prevProviders, false); err != nil {
				seclog.Errorf("failed to load policies: %s", err)
			}
		}()
	}

	// add selftests as provider
	if c.selfTester != nil {
		providers = append(providers, c.selfTester)
	}

	if err := c.ruleEngine.LoadPolicies(providers, false); err != nil {
		return false, err
	}

	if c.selfTester != nil {
		success, fails, testEvents, err := c.selfTester.RunSelfTest()
		if err != nil {
			return true, err
		}

		seclog.Debugf("self-test results : success : %v, failed : %v", success, fails)

		// send the report
		if c.config.SelfTestSendReport {
			ReportSelfTest(c.eventSender, c.statsdClient, success, fails, testEvents)
		}
	}

	return true, nil
}

// ReportSelfTest reports to Datadog that a self test was performed
func ReportSelfTest(sender events.EventSender, statsdClient statsd.ClientInterface, success []string, fails []string, testEvents map[string]*serializers.EventSerializer) {
	// send metric with number of success and fails
	tags := []string{
		fmt.Sprintf("success:%d", len(success)),
		fmt.Sprintf("fails:%d", len(fails)),
	}
	if err := statsdClient.Count(metrics.MetricSelfTest, 1, tags, 1.0); err != nil {
		seclog.Errorf("failed to send self_test metric: %s", err)
	}

	// send the custom event with the list of succeed and failed self tests
	rule, event := selftests.NewSelfTestEvent(success, fails, testEvents)
	sender.SendEvent(rule, event, nil, "")
}

// Stop closes the module
func (c *CWSConsumer) Stop() {
	c.reloader.Stop()

	if c.apiServer != nil {
		c.apiServer.Stop()
	}

	if c.selfTester != nil {
		_ = c.selfTester.Close()
	}

	c.ruleEngine.Stop()

	c.cancelFnc()
	c.wg.Wait()

	c.grpcServer.Stop()
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (c *CWSConsumer) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	c.eventSender.SendEvent(rule, event, nil, "")
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
func (c *CWSConsumer) SendEvent(rule *rules.Rule, event events.Event, extTagsCb func() []string, service string) {
	if c.rateLimiter.Allow(rule.ID, event) {
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
