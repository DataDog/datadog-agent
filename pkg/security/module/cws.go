// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/common"
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
	"github.com/DataDog/datadog-agent/pkg/security/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	grpcutils "github.com/DataDog/datadog-agent/pkg/security/utils/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

const (
	// selftest
	selftestMaxRetry    = 25 // more than 5 minutes so that we can get host tags
	selftestStartAfter  = 15 * time.Second
	selftestDelay       = 15 * time.Second
	selftestPassedDelay = 60 * time.Minute
)

// CWSConsumer represents the system-probe module for the runtime security agent
type CWSConsumer struct {
	sync.RWMutex
	config       *config.RuntimeSecurityConfig
	probe        *probe.Probe
	statsdClient statsd.ClientInterface

	// internals
	wg              sync.WaitGroup
	ctx             context.Context
	cancelFnc       context.CancelFunc
	apiServer       *APIServer
	rateLimiter     *events.RateLimiter
	sendStatsChan   chan chan bool
	eventSender     events.EventSender
	grpcCmdServer   *grpcutils.Server
	grpcEventServer *grpcutils.Server
	ruleEngine      *rulesmodule.RuleEngine
	selfTester      *selftests.SelfTester
	selfTestCount   int
	selfTestPassed  bool
	reloader        ReloaderInterface
	crtelemetry     *telemetry.ContainersRunningTelemetry
}

// NewCWSConsumer initializes the module with options
func NewCWSConsumer(evm *eventmonitor.EventMonitor, cfg *config.RuntimeSecurityConfig, wmeta workloadmeta.Component, filterStore workloadfilter.Component, opts Opts, compression compression.Component, ipc ipc.Component, hostname string) (*CWSConsumer, error) {
	crtelemcfg := telemetry.ContainersRunningTelemetryConfig{
		RuntimeEnabled: cfg.RuntimeEnabled,
		FIMEnabled:     cfg.FIMEnabled,
	}

	var (
		crtelemetry *telemetry.ContainersRunningTelemetry
		err         error
	)

	// filterStore can be nil, especially in the case of functional tests
	if filterStore != nil {
		crtelemetry, err = telemetry.NewContainersRunningTelemetry(crtelemcfg, evm.StatsdClient, wmeta, filterStore)
		if err != nil {
			return nil, err
		}
	}

	var selfTester *selftests.SelfTester
	if cfg.SelfTestEnabled {
		selfTester, err = selftests.NewSelfTester(cfg, evm.Probe)
		if err != nil {
			seclog.Errorf("unable to instantiate self tests: %s", err)
		}
	}

	cmdSocketPath, err := common.GetCmdSocketPath(cfg.SocketPath, cfg.CmdSocketPath)
	if err != nil {
		return nil, err
	}

	family, socketPath := socket.GetSocketAddress(cmdSocketPath)
	apiServer, err := NewAPIServer(cfg, evm.Probe, opts.MsgSender, evm.StatsdClient, selfTester, compression, hostname)
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
		grpcCmdServer: grpcutils.NewServer(family, socketPath),
		selfTester:    selfTester,
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

	c.ruleEngine, err = rulesmodule.NewRuleEngine(evm, cfg, evm.Probe, c.rateLimiter, c.apiServer, c, c.statsdClient, hostname, ipc, listeners...)
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

	// setup gRPC servers
	api.RegisterSecurityModuleCmdServer(c.grpcCmdServer.ServiceRegistrar(), c.apiServer)
	if cfg.EventGRPCServer != "security-agent" {
		seclog.Infof("start security module event grpc server")

		family := common.GetFamilyAddress(cfg.SocketPath)
		c.grpcEventServer = grpcutils.NewServer(family, cfg.SocketPath)

		api.RegisterSecurityModuleEventServer(c.grpcEventServer.ServiceRegistrar(), c.apiServer)
	}

	// platform specific initialization
	if err := c.init(evm, cfg, opts); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *CWSConsumer) onAPIConnectionEstablished() {
	seclog.Infof("api client connected, starts sending events")
	c.startRunningMetrics()
}

func (c *CWSConsumer) startRunningMetrics() {
	c.ruleEngine.StartRunningMetrics(c.ctx)

	if c.crtelemetry != nil {
		// Send containers running telemetry
		go c.crtelemetry.Run(c.ctx)
	}
}

// ID returns id for CWS
func (c *CWSConsumer) ID() string {
	return "CWS"
}

// Start the module
func (c *CWSConsumer) Start() error {
	if err := c.grpcCmdServer.Start(); err != nil {
		return err
	}

	if c.grpcEventServer != nil {
		if err := c.grpcEventServer.Start(); err != nil {
			return err
		}
	}

	if err := c.reloader.Start(); err != nil {
		return err
	}

	// start api server
	c.apiServer.Start(c.ctx)

	if err := c.ruleEngine.Start(c.ctx, c.reloader.Chan()); err != nil {
		return err
	}

	c.wg.Add(1)
	go c.statsSender()

	seclog.Infof("runtime security started")

	// we can now wait for self test events
	cb := func(success []eval.RuleID, fails []eval.RuleID) {
		c.selfTestCount++

		seclog.Debugf("self-test results : success : %v, failed : %v, run %d", success, fails, c.selfTestCount)

		delay := selftestDelay
		if c.selfTestPassed {
			delay = selftestPassedDelay
		}

		if !c.selfTestPassed && c.selfTestCount == selftestMaxRetry {
			c.reportSelfTest(success, fails)
		} else if len(fails) == 0 {
			c.selfTestPassed = true

			c.reportSelfTest(success, fails)
			delay = selftestPassedDelay
		}

		time.Sleep(delay)

		if _, err := c.RunSelfTest(false); err != nil {
			seclog.Errorf("self-test error: %s", err)
		}
	}
	if c.selfTester != nil {
		go c.selfTester.WaitForResult(cb)
	}

	// do not wait external api connection, send directly running metrics
	if c.config.SendPayloadsFromSystemProbe {
		c.startRunningMetrics()
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

			case <-time.After(selftestStartAfter):
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

	if err := c.selfTester.RunSelfTest(c.ctx, selftests.DefaultTimeout); err != nil {
		return true, err
	}

	return true, nil
}

func (c *CWSConsumer) reportSelfTest(success []eval.RuleID, fails []eval.RuleID) {
	if !c.config.SelfTestSendReport {
		return
	}

	// send metric with number of success and fails
	tags := []string{
		"success:" + strconv.Itoa(len(success)),
		"fails:" + strconv.Itoa(len(fails)),
		"os:" + runtime.GOOS,
		"arch:" + utils.RuntimeArch(),
		"origin:" + c.probe.Origin(),
	}
	if err := c.statsdClient.Gauge(metrics.MetricSelfTest, 1.0, tags, 1.0); err != nil {
		seclog.Errorf("failed to send self_test metric: %s", err)
	}

	// send the custom event with the list of succeed and failed self tests
	rule, event := selftests.NewSelfTestEvent(c.probe.GetAgentContainerContext(), success, fails)
	c.SendEvent(rule, event, nil, "")
}

// Stop closes the module
func (c *CWSConsumer) Stop() {
	c.reloader.Stop()

	c.cancelFnc()

	// Stop the rule engine first to stop goroutines that send heartbeat/ruleset loaded events to the reporter
	c.ruleEngine.Stop()

	c.wg.Wait()

	// Now we shouldn't have anymore events to send so we can safely stop the API server to close reporter channels
	if c.apiServer != nil {
		c.apiServer.Stop()
	}

	c.grpcCmdServer.Stop()
	if c.grpcEventServer != nil {
		c.grpcEventServer.Stop()
	}
}

// HandleCustomEvent is called by the probe when an event should be sent to Datadog but doesn't need evaluation
func (c *CWSConsumer) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	c.SendEvent(rule, event, nil, "")
}

// SendEvent sends an event to the backend after checking that the rate limiter allows it for the provided rule
// Implements the EventSender interface
func (c *CWSConsumer) SendEvent(rule *rules.Rule, event events.Event, extTagsCb func() ([]string, bool), service string) {
	if c.rateLimiter.Allow(rule.ID, event) {
		c.eventSender.SendEvent(rule, event, extTagsCb, service)
	}
}

// APIServer returns the api server
func (c *CWSConsumer) APIServer() *APIServer {
	return c.apiServer
}

// HandleActivityDump sends an activity dump to the backend
func (c *CWSConsumer) HandleActivityDump(imageName string, imageTag string, header []byte, data []byte) error {
	c.apiServer.SendActivityDump(imageName, imageTag, header, data)
	return nil
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

	c.ruleEngine.SendStats()
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

// GetStatus returns the status of the module
func (c *CWSConsumer) GetStatus(ctx context.Context) (*api.Status, error) {
	return c.apiServer.GetStatus(ctx, &api.GetStatusParams{})
}
