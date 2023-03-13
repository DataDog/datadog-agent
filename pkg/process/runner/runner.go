// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/types"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type checkResult struct {
	name        string
	payloads    []checkPayload
	sizeInBytes int64
}

func (cr *checkResult) Weight() int64 {
	return cr.sizeInBytes
}

func (cr *checkResult) Type() string {
	return cr.name
}

var _ api.WeightedItem = &checkResult{}

type checkPayload struct {
	body    []byte
	headers http.Header
}

type Runner interface {
}

// CheckRunner will collect metrics from the local system and ship to the backend.
type CheckRunner struct {
	config ddconfig.ConfigReader

	// required for being able to start and stop the collector
	wg   *sync.WaitGroup
	stop chan struct{}

	// true if real-time is enabled
	realTimeEnabled *atomic.Bool

	// the next groupID to be issued
	groupID *atomic.Int32

	rtIntervalCh chan time.Duration

	orchestrator *oconfig.OrchestratorConfig

	// counters for each type of check
	runCounters   sync.Map
	enabledChecks []checks.Check

	// Controls the real-time interval, can change live.
	realTimeInterval time.Duration

	// Enables running realtime checks
	runRealTime bool

	// Submits check payloads to datadog
	Submitter Submitter

	// listens for when to enable and disable realtime mode
	rtNotifierChan <-chan types.RTResponse
}

func (l *CheckRunner) RunRealTime() bool {
	return l.runRealTime
}

// NewRunner creates a new CheckRunner
func NewRunner(config ddconfig.ConfigReader, sysCfg *sysconfig.Config, hostInfo *checks.HostInfo, enabledChecks []checks.Check, rtNotifierChan <-chan types.RTResponse) (*CheckRunner, error) {
	runRealTime := !config.GetBool("process_config.disable_realtime_checks")

	cfg := &checks.SysProbeConfig{}
	if sysCfg != nil && sysCfg.Enabled {
		// If the sysprobe module is enabled, the process check can call out to the sysprobe for privileged stats
		_, processModuleEnabled := sysCfg.EnabledModules[sysconfig.ProcessModule]
		cfg.ProcessModuleEnabled = processModuleEnabled
		cfg.MaxConnsPerMessage = sysCfg.MaxConnsPerMessage
		cfg.SystemProbeAddress = sysCfg.SocketAddress
	}

	for _, c := range enabledChecks {
		if err := c.Init(cfg, hostInfo); err != nil {
			return nil, err
		}
	}

	return NewRunnerWithChecks(config, enabledChecks, runRealTime, rtNotifierChan)
}

// NewRunnerWithChecks creates a new CheckRunner
func NewRunnerWithChecks(config ddconfig.ConfigReader, checks []checks.Check, runRealTime bool, rtNotifierChan <-chan types.RTResponse) (*CheckRunner, error) {
	orchestrator := oconfig.NewDefaultOrchestratorConfig()
	if err := orchestrator.Load(); err != nil {
		return nil, err
	}

	return &CheckRunner{
		config: config,

		wg:   &sync.WaitGroup{},
		stop: make(chan struct{}),

		rtIntervalCh:  make(chan time.Duration),
		orchestrator:  orchestrator,
		groupID:       atomic.NewInt32(rand.Int31()),
		enabledChecks: checks,

		// Defaults for real-time on start
		realTimeInterval: 2 * time.Second,
		realTimeEnabled:  atomic.NewBool(false),

		runRealTime:    runRealTime,
		rtNotifierChan: rtNotifierChan,
	}, nil
}

func (l *CheckRunner) runCheck(c checks.Check) {
	runCounter := l.nextRunCounter(c.Name())
	start := time.Now()
	// update the last collected timestamp for info
	status.UpdateLastCollectTime(start)

	result, err := c.Run(l.nextGroupID, nil)
	if err != nil {
		log.Errorf("Unable to run check '%s': %s", c.Name(), err)
		return
	}

	if result == nil {
		// Check returned nothing
		return
	}

	if c.ShouldSaveLastRun() {
		checks.StoreCheckOutput(c.Name(), result.Payloads())
	} else {
		checks.StoreCheckOutput(c.Name(), nil)
	}

	msg := &types.Payload{
		CheckName: c.Name(),
		Message:   result.Payloads(),
	}
	l.Submitter.Submit(start, c.Name(), msg)

	if !c.Realtime() {
		logCheckDuration(c.Name(), start, runCounter)
	}
}

func (l *CheckRunner) runCheckWithRealTime(c checks.Check, options *checks.RunOptions) {
	start := time.Now()
	// update the last collected timestamp for info
	status.UpdateLastCollectTime(start)

	result, err := c.Run(l.nextGroupID, options)
	if err != nil {
		log.Errorf("Unable to run check '%s': %s", c.Name(), err)
		return
	}

	if result == nil {
		// Check returned nothing
		return
	}

	msg := &types.Payload{
		CheckName: c.Name(),
		Message:   result.Payloads(),
	}
	l.Submitter.Submit(start, c.Name(), msg)
	if options.RunStandard {
		// We are only updating the run counter for the standard check
		// since RT checks are too frequent and we only log standard check
		// durations
		runCounter := l.nextRunCounter(c.Name())
		checks.StoreCheckOutput(c.Name(), result.Payloads())
		logCheckDuration(c.Name(), start, runCounter)
	}

	rtName := checks.RTName(c.Name())
	msg = &types.Payload{
		Message: result.RealtimePayloads(),
	}
	l.Submitter.Submit(start, rtName, msg)
	if options.RunRealtime {
		checks.StoreCheckOutput(rtName, msg.Message)
	}
}

func (l *CheckRunner) nextRunCounter(name string) int32 {
	runCounter := int32(1)
	if rc, ok := l.runCounters.Load(name); ok {
		runCounter = rc.(int32) + 1
	}
	l.runCounters.Store(name, runCounter)
	return runCounter
}

func logCheckDuration(name string, start time.Time, runCounter int32) {
	d := time.Since(start)
	switch {
	case runCounter < 5:
		log.Infof("Finished %s check #%d in %s", name, runCounter, d)
	case runCounter == 5:
		log.Infof("Finished %s check #%d in %s. First 5 check runs finished, next runs will be logged every 20 runs.", name, runCounter, d)
	case runCounter%20 == 0:
		log.Infof("Finish %s check #%d in %s", name, runCounter, d)
	}
}

func (l *CheckRunner) nextGroupID() int32 {
	return l.groupID.Inc()
}

const (
	secondsNumberOfBits = 22
	hashNumberOfBits    = 28
	chunkNumberOfBits   = 14
	secondsMask         = 1<<secondsNumberOfBits - 1
	hashMask            = 1<<hashNumberOfBits - 1
	chunkMask           = 1<<chunkNumberOfBits - 1
)

func (l *CheckRunner) Run() error {
	realTimeAllowed := !l.config.GetBool("process_config.disable_realtime_checks")

	checkNamesLength := len(l.enabledChecks)
	if realTimeAllowed {
		// checkNamesLength is double when realtime checks is enabled as we append the Process real time name
		// as well as the original check name
		checkNamesLength = checkNamesLength * 2
	}

	checkNames := make([]string, 0, checkNamesLength)
	for _, check := range l.enabledChecks {
		checkNames = append(checkNames, check.Name())

		// Append `process_rt` if process check is enabled, and rt is enabled, so the customer doesn't get confused if
		// process_rt doesn't show up in the enabled checks
		if check.Name() == checks.ProcessCheckName && realTimeAllowed {
			checkNames = append(checkNames, checks.RTProcessCheckName)
		}
	}
	status.UpdateEnabledChecks(checkNames)
	log.Infof("Starting process-agent with enabled checks=%v", checkNames)

	if realTimeAllowed && l.rtNotifierChan != nil {
		l.listenForRTUpdates()
	}

	for _, c := range l.enabledChecks {
		runner, err := l.runnerForCheck(c)
		if err != nil {
			return fmt.Errorf("error starting check %s: %s", c.Name(), err)
		}

		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			runner()
		}()
	}

	return nil
}

func (l *CheckRunner) listenForRTUpdates() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		for {
			select {
			case response, ok := <-l.rtNotifierChan:
				if !ok {
					return
				}

				l.UpdateRTStatus(response)
			case <-l.stop:
				return
			}
		}
	}()
}

func (l *CheckRunner) runnerForCheck(c checks.Check) (func(), error) {
	if !l.runRealTime || !c.SupportsRunOptions() {
		return l.basicRunner(c), nil
	}

	rtName := checks.RTName(c.Name())
	interval := checks.GetInterval(c.Name())
	rtInterval := checks.GetInterval(rtName)

	if interval < rtInterval || interval%rtInterval != 0 {
		// Check interval must be greater or equal to realtime check interval and the intervals must be divisible
		// in order to be run on the same goroutine
		defaultInterval := checks.GetDefaultInterval(c.Name())
		defaultRTInterval := checks.GetDefaultInterval(rtName)
		log.Warnf(
			"Invalid %s check interval overrides [%s,%s], resetting to defaults [%s,%s]",
			c.Name(),
			interval,
			rtInterval,
			defaultInterval,
			defaultRTInterval,
		)
		interval = defaultInterval
		rtInterval = defaultRTInterval
	}

	return checks.NewRunnerWithRealTime(
		checks.RunnerConfig{
			CheckInterval:  interval,
			RtInterval:     rtInterval,
			ExitChan:       l.stop,
			RtIntervalChan: l.rtIntervalCh,
			RtEnabled: func() bool {
				return l.realTimeEnabled.Load()
			},
			RunCheck: func(options checks.RunOptions) {
				l.runCheckWithRealTime(c, &options)
			},
		},
	)
}

func (l *CheckRunner) basicRunner(c checks.Check) func() {
	return func() {
		// Run the check the first time to prime the caches.
		if !c.Realtime() {
			l.runCheck(c)
		}

		ticker := time.NewTicker(checks.GetInterval(c.Name()))
		for {
			select {
			case <-ticker.C:
				realTimeEnabled := l.runRealTime && l.realTimeEnabled.Load()
				if !c.Realtime() || realTimeEnabled {
					l.runCheck(c)
				}
			case d := <-l.rtIntervalCh:

				// Live-update the ticker.
				if c.Realtime() {
					ticker.Stop()
					ticker = time.NewTicker(d)
				}
			case _, ok := <-l.stop:
				if !ok {
					return
				}
			}
		}
	}
}

func (l *CheckRunner) UpdateRTStatus(statuses []*model.CollectorStatus) {
	// If realtime mode is disabled in the config, do not change the real time status.
	if !l.runRealTime {
		return
	}

	curEnabled := l.realTimeEnabled.Load()

	// If any of the endpoints wants real-time we'll do that.
	// We will pick the maximum interval given since generally this is
	// only set if we're trying to limit load on the backend.
	shouldEnableRT := false
	maxInterval := 0 * time.Second
	activeClients := int32(0)
	for _, s := range statuses {
		shouldEnableRT = shouldEnableRT || s.ActiveClients > 0
		if s.ActiveClients > 0 {
			activeClients += s.ActiveClients
		}
		interval := time.Duration(s.Interval) * time.Second
		if interval > maxInterval {
			maxInterval = interval
		}
	}

	if curEnabled && !shouldEnableRT {
		log.Info("Detected 0 clients, disabling real-time mode")
		l.realTimeEnabled.Store(false)
	} else if !curEnabled && shouldEnableRT {
		log.Infof("Detected %d active clients, enabling real-time mode", activeClients)
		l.realTimeEnabled.Store(true)
	}

	if maxInterval != l.realTimeInterval {
		l.realTimeInterval = maxInterval
		if l.realTimeInterval <= 0 {
			l.realTimeInterval = 2 * time.Second
		}
		// Pass along the real-time interval, one per check, so that every
		// check routine will see the new interval.
		for range l.enabledChecks {
			l.rtIntervalCh <- l.realTimeInterval
		}
		log.Infof("real time interval updated to %s", l.realTimeInterval)
	}
}

func (l *CheckRunner) Stop() {
	close(l.stop)
	l.wg.Wait()

	for _, check := range l.enabledChecks {
		log.Debugf("Cleaning up %s check", check.Name())
		check.Cleanup()
	}
}

func (l *CheckRunner) GetChecks() []checks.Check {
	return l.enabledChecks
}

func (l *CheckRunner) IsRealTimeEnabled() bool {
	return l.realTimeEnabled.Load()
}

// getContainerCount returns the number of containers in the message body
func getContainerCount(mb model.MessageBody) int {
	switch v := mb.(type) {
	case *model.CollectorProc:
		return len(v.GetContainers())
	case *model.CollectorRealTime:
		return len(v.GetContainerStats())
	case *model.CollectorContainer:
		return len(v.GetContainers())
	case *model.CollectorContainerRealTime:
		return len(v.GetStats())
	case *model.CollectorConnections:
		return 0
	}
	return 0
}

func readResponseStatuses(checkName string, responses <-chan forwarder.Response) []*model.CollectorStatus {
	var statuses []*model.CollectorStatus

	for response := range responses {
		if response.Err != nil {
			log.Errorf("[%s] Error from %s: %s", checkName, response.Domain, response.Err)
			continue
		}

		if response.StatusCode >= 300 {
			log.Errorf("[%s] Invalid response from %s: %d -> %s", checkName, response.Domain, response.StatusCode, response.Err)
			continue
		}

		// some checks don't receive a response with a status used to enable RT mode
		if ignoreResponseBody(checkName) {
			continue
		}

		r, err := model.DecodeMessage(response.Body)
		if err != nil {
			log.Errorf("[%s] Could not decode response body: %s", checkName, err)
			continue
		}

		switch r.Header.Type {
		case model.TypeResCollector:
			rm := r.Body.(*model.ResCollector)
			if len(rm.Message) > 0 {
				log.Errorf("[%s] Error in response from %s: %s", checkName, response.Domain, rm.Message)
			} else {
				statuses = append(statuses, rm.Status)
			}
		default:
			log.Errorf("[%s] Unexpected response type from %s: %d", checkName, response.Domain, r.Header.Type)
		}
	}

	return statuses
}

func ignoreResponseBody(checkName string) bool {
	switch checkName {
	case checks.PodCheckName, checks.PodCheckManifestName, checks.ProcessEventsCheckName:
		return true
	default:
		return false
	}
}
