// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
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
	traceroute                   traceroute.Component
	generateTestResultID         func(func(rand io.Reader, max *big.Int) (n *big.Int, err error)) (string, error)
	ticker                       *time.Ticker
	tickerC                      <-chan time.Time
	sendResult                   func(w *workerResult) (string, error)
	hostNameService              hostname.Component
	statsdClient                 ddgostatsd.ClientInterface
	testPoller                   *testPoller
	namespace                    string
}

// newSyntheticsTestScheduler creates a scheduler and initializes its state.
func newSyntheticsTestScheduler(configs *schedulerConfigs, forwarder eventplatform.Forwarder, logger log.Component, hostNameService hostname.Component, timeFunc func() time.Time, statsd ddgostatsd.ClientInterface, traceroute traceroute.Component, poller *testPoller) *syntheticsTestScheduler {
	scheduler := &syntheticsTestScheduler{
		epForwarder:                  forwarder,
		log:                          logger,
		hostNameService:              hostNameService,
		traceroute:                   traceroute,
		state:                        runningState{tests: map[string]*runningTestState{}},
		workersDone:                  make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 100),
		timeNowFn:                    timeFunc,
		workers:                      configs.workers,
		flushInterval:                configs.flushInterval,
		generateTestResultID:         generateRandomStringUInt63,
		statsdClient:                 statsd,
		testPoller:                   poller,
		namespace:                    configs.namespace,
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
	nextRun time.Time
}

type runningState struct {
	mu    sync.RWMutex
	tests map[string]*runningTestState // PublicID -> runtime state
}

// upsertFallbackCache records a scheduled test config in the in-memory cache used
// when the test poller is unhealthy. New entries get nextRun = now + Interval (the
// test just fired via the poller). On version bump, nextRun is recomputed; otherwise
// the existing nextRun is preserved.
func (s *syntheticsTestScheduler) upsertFallbackCache(cfg common.SyntheticsTestConfig) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	ChecksReceived.Inc()
	s.statsdClient.Incr(syntheticsMetricPrefix+"checks_received", []string{fmt.Sprintf("org_id:%d", cfg.OrgID)}, 1) //nolint:errcheck

	now := s.timeNowFn().UTC()
	interval := time.Duration(cfg.Interval) * time.Second

	current, exists := s.state.tests[cfg.PublicID]
	if !exists {
		s.state.tests[cfg.PublicID] = &runningTestState{
			cfg:     cfg,
			nextRun: now.Add(interval),
		}
		return
	}
	if current.cfg.Version < cfg.Version {
		current.nextRun = now.Add(interval)
	}
	current.cfg = cfg
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
	s.testPoller.start(ctx)

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

	// Close the processing channel to unblock workers immediately
	close(s.syntheticsTestProcessingChan)

	// Wait for workers to stop
	<-s.workersDone
	s.log.Debug("all workers stopped")

	// Wait for flush loop to stop
	<-s.flushLoopDone
	s.ticker.Stop()
	s.log.Debug("flush loop stopped")

	// Wait for test poll loop to stop
	s.testPoller.stop()
	s.log.Debug("test poll loop stopped")

	s.log.Info("synthetics test scheduler stopped")
}
