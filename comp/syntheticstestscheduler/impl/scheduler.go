// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// syntheticsTestScheduler is responsible for scheduling and executing synthetics tests.
type syntheticsTestScheduler struct {
	log                          log.Component
	state                        runningState
	cancel                       context.CancelFunc
	running                      bool
	workers                      int
	workersDone                  chan struct{}
	timeNowFn                    func() time.Time
	syntheticsTestProcessingChan chan SyntheticsTestCtx
	flushInterval                time.Duration
	flushLoopDone                chan struct{}
	epForwarder                  eventplatform.Forwarder
	telemetry                    telemetry.Component
	generateTestResultID         func(func(rand io.Reader, max *big.Int) (n *big.Int, err error)) (string, error)
	ticker                       *time.Ticker
	tickerC                      <-chan time.Time
	runTraceroute                func(ctx context.Context, cfg config.Config, telemetry telemetry.Component) (payload.NetworkPath, error)
	sendResult                   func(w *workerResult) error
	hostNameService              hostname.Component
}

// newSyntheticsTestScheduler creates a scheduler and initializes its state.
func newSyntheticsTestScheduler(configs *schedulerConfigs, forwarder eventplatform.Forwarder, logger log.Component, hostNameService hostname.Component, timeFunc func() time.Time) *syntheticsTestScheduler {
	scheduler := &syntheticsTestScheduler{
		epForwarder:                  forwarder,
		log:                          logger,
		hostNameService:              hostNameService,
		state:                        runningState{tests: map[string]*runningTestState{}},
		workersDone:                  make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 100),
		timeNowFn:                    timeFunc,
		workers:                      configs.workers,
		flushInterval:                configs.flushInterval,
		generateTestResultID:         generateRandomStringUInt63,
		runTraceroute:                runTraceroute,
	}

	// by default, sendResult delegates to the real forwarder-backed implementation
	scheduler.sendResult = scheduler.sendSyntheticsTestResult

	scheduler.ticker = time.NewTicker(scheduler.flushInterval)
	scheduler.tickerC = scheduler.ticker.C

	return scheduler
}

// runningTestState represents in-memory runtime data for a scheduled test.
type runningTestState struct {
	cfg     common.SyntheticsTestConfig
	lastRun time.Time
	nextRun time.Time
}

type runningState struct {
	mu    sync.RWMutex
	tests map[string]*runningTestState // PublicID -> runtime state
}

// onConfigUpdate handles remote-config updates for synthetics tests.
func (s *syntheticsTestScheduler) onConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	s.log.Debugf("Updates received: count=%d", len(updates))

	newConfig := map[string]common.SyntheticsTestConfig{}
	for configPath, rawConfig := range updates {
		s.log.Debugf("received config %s: %s", configPath, string(rawConfig.Config))
		syntheticsTestCfg := common.SyntheticsTestConfig{}
		if err := json.Unmarshal(rawConfig.Config, &syntheticsTestCfg); err != nil {
			s.log.Warnf("skipping invalid Synthetics Test update %s: %v", configPath, err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			})
			continue
		}

		newConfig[syntheticsTestCfg.PublicID] = syntheticsTestCfg
		s.log.Debugf("Processed config %s: %v", configPath, syntheticsTestCfg)

		applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	s.updateRunningState(newConfig)
}

// updateRunningState synchronizes in-memory runtime state with a new configuration.
func (s *syntheticsTestScheduler) updateRunningState(newConfig map[string]common.SyntheticsTestConfig) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	seen := map[string]struct{}{}
	for _, newTestConfig := range newConfig {
		pubID := newTestConfig.PublicID
		seen[pubID] = struct{}{}
		current, exists := s.state.tests[pubID]
		if !exists {
			s.state.tests[pubID] = &runningTestState{
				cfg:     newTestConfig,
				lastRun: time.Time{},
				nextRun: s.timeNowFn().UTC().Add(time.Duration(newTestConfig.Interval) * time.Second),
			}
		} else {
			current.cfg = newTestConfig
		}
	}

	for pubID := range s.state.tests {
		if _, exists := seen[pubID]; !exists {
			delete(s.state.tests, pubID)
		}
	}
}

// start launches flush loop and workers.
func (s *syntheticsTestScheduler) start(ctx context.Context) error {
	if s.running {
		return errors.New("server already started")
	}
	s.running = true

	ctx, s.cancel = context.WithCancel(ctx)
	s.log.Info("start Synthetics Test Scheduler")

	go s.flushLoop(ctx)
	go s.runWorkers(ctx)

	return nil
}

// stop signals all goroutines to stop and waits for them to finish.
func (s *syntheticsTestScheduler) stop() {
	if !s.running {
		return
	}
	s.running = false

	s.log.Info("stopping synthetics test scheduler")

	// Signal stop
	s.cancel()

	// Wait for workers to stop
	<-s.workersDone
	s.log.Debug("all workers stopped")

	// Wait for flush loop to stop
	<-s.flushLoopDone
	s.ticker.Stop()
	s.log.Debug("flush loop stopped")

	s.log.Info("synthetics test scheduler stopped")
}
