// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const cacheKey = "synthetics-tests-scheduler"

// SyntheticsTestScheduler is responsible for scheduling and executing synthetics tests.
type SyntheticsTestScheduler struct {
	log                          log.Component
	runningConfig                map[string]common.SyntheticsTestConfig
	state                        runningState
	running                      bool
	stopChan                     chan struct{}
	workers                      int
	workersDone                  chan struct{}
	TimeNowFn                    func() time.Time
	syntheticsTestProcessingChan chan SyntheticsTestCtx
	flushInterval                time.Duration
	flushLoopDone                chan struct{}
	epForwarder                  eventplatform.Forwarder
	telemetry                    telemetry.Component
	configID                     string
	generateTestResultID         func() (string, error)
	ticker                       *time.Ticker
	tickerC                      <-chan time.Time
	runTraceroute                func(cfg config.Config, telemetry telemetry.Component) (payload.NetworkPath, error)
	sendResult                   func(w *WorkerResult) error
}

// newSyntheticsTestScheduler creates a scheduler and initializes its state.
func newSyntheticsTestScheduler(configs *schedulerConfigs, epForwarder eventplatform.Forwarder, logger log.Component, configID string, timeFunc func() time.Time) (*SyntheticsTestScheduler, error) {
	scheduler := &SyntheticsTestScheduler{
		epForwarder:                  epForwarder,
		log:                          logger,
		configID:                     configID,
		runningConfig:                map[string]common.SyntheticsTestConfig{},
		state:                        runningState{tests: map[string]*runningTestState{}},
		stopChan:                     make(chan struct{}),
		workersDone:                  make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		syntheticsTestProcessingChan: make(chan SyntheticsTestCtx, 100),
		TimeNowFn:                    timeFunc,
		workers:                      configs.workers,
		flushInterval:                configs.flushInterval,
		generateTestResultID:         generateRandomStringUInt63,
		runTraceroute:                runTraceroute,
	}

	// by default, sendResult delegates to the real forwarder-backed implementation
	scheduler.sendResult = scheduler.sendSyntheticsTestResult

	scheduler.ticker = time.NewTicker(scheduler.flushInterval)
	scheduler.tickerC = scheduler.ticker.C

	if err := scheduler.retrieveConfig(); err != nil {
		return nil, err
	}
	return scheduler, nil
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
func (s *SyntheticsTestScheduler) onConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
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
	s.runningConfig = newConfig

	if err := s.persistConfig(); err != nil {
		s.log.Warnf("unable to persist synthetics test config: %v", err)
		// TODO: what path should I provide if it is global to the config
		applyStateCallback("TODO", state.ApplyStatus{
			State: state.ApplyStateError,
			Error: "unable to persist synthetics test config",
		})
	}
}

// updateRunningState synchronizes in-memory runtime state with a new configuration.
func (s *SyntheticsTestScheduler) updateRunningState(newConfig map[string]common.SyntheticsTestConfig) {
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
				nextRun: s.TimeNowFn().Add(time.Duration(newTestConfig.Interval) * time.Second),
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

// persistConfig writes the runningConfig to persistent cache.
func (s *SyntheticsTestScheduler) persistConfig() error {
	cacheValue, err := json.Marshal(s.runningConfig)
	if err != nil {
		return err
	}
	if err := persistentcache.Write(cacheKey, string(cacheValue)); err != nil {
		return s.log.Errorf("couldn't write cache: %s", err)
	}
	return nil
}

// retrieveConfig reads scheduler config from persistent cache and initializes state.
func (s *SyntheticsTestScheduler) retrieveConfig() error {
	cacheValue, err := persistentcache.Read(cacheKey)
	if err != nil {
		return s.log.Errorf("couldn't read from cache: %s", err)
	}
	if len(cacheValue) == 0 || cacheValue == "{}" {
		return nil
	}

	var cfg map[string]common.SyntheticsTestConfig
	if err := json.Unmarshal([]byte(cacheValue), &cfg); err != nil {
		return err
	}
	s.runningConfig = cfg

	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	s.state.tests = make(map[string]*runningTestState, len(cfg))
	now := s.TimeNowFn()
	for id, testCfg := range cfg {
		s.state.tests[id] = &runningTestState{
			cfg:     testCfg,
			lastRun: time.Time{},
			nextRun: now.Add(time.Duration(testCfg.Interval) * time.Second),
		}
	}
	return nil
}

// start launches flush loop and workers.
func (s *SyntheticsTestScheduler) start() error {
	if s.running {
		return errors.New("server already started")
	}
	s.running = true

	s.log.Info("start Synthetics Test Scheduler")

	go s.flushLoop()
	go s.runWorkers()

	return nil
}

// stop signals all goroutines to stop and waits for them to finish.
func (s *SyntheticsTestScheduler) stop() {
	if !s.running {
		return
	}
	s.running = false

	s.log.Info("stopping synthetics test scheduler")

	// Signal stop
	close(s.stopChan)

	// Wait for workers to stop
	<-s.workersDone
	s.log.Debug("all workers stopped")

	// Wait for flush loop to stop
	<-s.flushLoopDone
	s.ticker.Stop()
	s.log.Debug("flush loop stopped")

	s.log.Info("synthetics test scheduler stopped")
}
