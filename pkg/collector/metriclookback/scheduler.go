// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultShadowInterval    = time.Second
	defaultShadowStopTimeout = 5 * time.Second
)

var errShadowSchedulerStopped = errors.New("shadow scheduler is stopped")

// SenderManagerFactory returns a sender manager scoped to one shadow check.
type SenderManagerFactory func(context.Context) sender.SenderManager

// CheckInstanceLoader loads a fresh check instance using the provided sender
// manager and normal collector loader-selection semantics.
type CheckInstanceLoader interface {
	LoadInstance(sender.SenderManager, integration.Config, integration.Data, int) (check.Check, bool, error)
}

// ShadowRunner runs shadow checks on an isolated execution pipeline.
type ShadowRunner interface {
	GetChan() chan<- check.Check
	Stop()
}

// ShadowRunnerFactory creates one runner scoped to all shadow check execution.
// The runner must suppress normal integration status emission; shadow checks
// must not emit datadog.agent.check_status service checks to the backend.
type ShadowRunnerFactory func(runner.ScheduledChecks) ShadowRunner

// ShadowSchedulerOptions configures the shadow scheduler component.
type ShadowSchedulerOptions struct {
	Loader           CheckInstanceLoader
	NewSenderManager SenderManagerFactory
	NewRunner        ShadowRunnerFactory
	Interval         time.Duration
	StopTimeout      time.Duration
	NewTicker        func(time.Duration) ShadowTicker
}

// ShadowScheduler reuses normal check-loading semantics through CheckInstanceLoader,
// but owns the shadow lifecycle policy. Keep this separate from the normal
// scheduler/runner instances so :shadow identity, lookback sender isolation,
// dedicated runner backpressure, and bounded stop/cancel behavior do not affect
// normal checks.
type ShadowScheduler struct {
	loader           CheckInstanceLoader
	newSenderManager SenderManagerFactory
	newRunner        ShadowRunnerFactory
	interval         time.Duration
	stopTimeout      time.Duration
	newTicker        func(time.Duration) ShadowTicker

	mu         sync.Mutex
	checks     map[shadowKey]*shadowCheckHandle
	checksByID map[checkid.ID]shadowKey
	runner     ShadowRunner
	stopped    bool
}

// NewShadowScheduler creates an isolated scheduler for lookback shadow checks.
func NewShadowScheduler(opts ShadowSchedulerOptions) *ShadowScheduler {
	interval := opts.Interval
	if interval == 0 {
		interval = defaultShadowInterval
	}
	stopTimeout := opts.StopTimeout
	if stopTimeout == 0 {
		stopTimeout = defaultShadowStopTimeout
	}
	newTicker := opts.NewTicker
	if newTicker == nil {
		newTicker = newRealShadowTicker
	}
	return &ShadowScheduler{
		loader:           opts.Loader,
		newSenderManager: opts.NewSenderManager,
		newRunner:        opts.NewRunner,
		interval:         interval,
		stopTimeout:      stopTimeout,
		newTicker:        newTicker,
		checks:           make(map[shadowKey]*shadowCheckHandle),
		checksByID:       make(map[checkid.ID]shadowKey),
	}
}

// Schedule loads and starts shadow checks for the provided derived configs.
func (s *ShadowScheduler) Schedule(configs []ShadowConfig) error {
	if s.loader == nil {
		return errors.New("shadow scheduler loader is nil")
	}
	if s.newSenderManager == nil {
		return errors.New("shadow scheduler sender manager factory is nil")
	}
	if s.newRunner == nil {
		return errors.New("shadow scheduler runner factory is nil")
	}

	var firstErr error
	for _, config := range configs {
		key := newShadowKey(config)

		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return errShadowSchedulerStopped
		}
		if _, exists := s.checks[key]; exists {
			s.mu.Unlock()
			log.Warnf("shadow check %s is already scheduled", config.ShadowCheckID)
			continue
		}
		s.mu.Unlock()

		handle, err := s.load(config)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		s.mu.Lock()
		// LoadInstance is synchronous and does not accept a context, so re-check
		// scheduler state after loading to avoid late-starting after Stop.
		if s.stopped {
			s.mu.Unlock()
			_ = handle.stop(s.stopTimeout)
			if firstErr == nil {
				firstErr = errShadowSchedulerStopped
			}
			continue
		}
		if _, exists := s.checks[key]; exists {
			s.mu.Unlock()
			log.Warnf("shadow check %s was already scheduled while loading", config.ShadowCheckID)
			_ = handle.stop(s.stopTimeout)
			continue
		}
		s.checks[key] = handle
		s.checksByID[config.ShadowCheckID] = key
		handle.runner = s.getOrCreateRunnerLocked()
		handle.start()
		s.mu.Unlock()
	}

	return firstErr
}

func (s *ShadowScheduler) getOrCreateRunnerLocked() ShadowRunner {
	if s.runner == nil {
		s.runner = s.newRunner(s)
	}
	return s.runner
}

func (s *ShadowScheduler) load(config ShadowConfig) (*shadowCheckHandle, error) {
	ctx, cancel := context.WithCancel(context.Background())
	shadowSenderManager := &shadowSenderManager{
		delegate: s.newSenderManager(ctx),
		sourceID: config.SourceCheckID,
		shadowID: config.ShadowCheckID,
	}

	loadedCheck, loaded, err := s.loader.LoadInstance(shadowSenderManager, config.SourceConfig, config.Instance, config.InstanceIndex)
	if err != nil {
		shadowSenderManager.DestroySender(config.ShadowCheckID)
		cancel()
		return nil, fmt.Errorf("load shadow check %s: %w", config.ShadowCheckID, err)
	}
	if !loaded {
		shadowSenderManager.DestroySender(config.ShadowCheckID)
		cancel()
		return nil, fmt.Errorf("load shadow check %s: no check loaded", config.ShadowCheckID)
	}

	handle := &shadowCheckHandle{
		check:         &shadowCheck{Check: loadedCheck, id: config.ShadowCheckID, interval: s.interval},
		cancel:        cancel,
		senderManager: shadowSenderManager,
		ticker:        s.newTicker(s.interval),
		stopCh:        make(chan struct{}),
		stoppedCh:     make(chan struct{}),
	}
	return handle, nil
}

// Unschedule stops and removes shadow checks matching the provided source
// configs. Waiting is bounded by the scheduler stop timeout.
func (s *ShadowScheduler) Unschedule(configs []ShadowConfig) error {
	var firstErr error
	for _, config := range configs {
		key := newShadowKey(config)

		s.mu.Lock()
		handle, found := s.checks[key]
		if found {
			delete(s.checks, key)
			delete(s.checksByID, config.ShadowCheckID)
		}
		s.mu.Unlock()

		if !found {
			continue
		}

		if err := handle.stop(s.stopTimeout); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Stop stops all scheduled shadow checks. Waiting for each check is bounded by
// the scheduler stop timeout.
func (s *ShadowScheduler) Stop() error {
	s.mu.Lock()
	s.stopped = true
	handles := make([]*shadowCheckHandle, 0, len(s.checks))
	for key, handle := range s.checks {
		handles = append(handles, handle)
		delete(s.checks, key)
		delete(s.checksByID, handle.check.ID())
	}
	shadowRunner := s.runner
	s.runner = nil
	s.stopped = true
	s.mu.Unlock()

	var firstErr error
	for _, handle := range handles {
		if err := handle.stop(s.stopTimeout); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if shadowRunner != nil {
		if err := callWithTimeout(shadowRunner.Stop, s.stopTimeout); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("timeout while stopping shadow runner: %w", err)
		}
	}
	return firstErr
}

// IsCheckScheduled returns whether a shadow check is still scheduled.
func (s *ShadowScheduler) IsCheckScheduled(id checkid.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.checksByID[id]
	return found
}

type shadowKey struct {
	sourceConfigDigest string
	instanceIndex      int
}

func newShadowKey(config ShadowConfig) shadowKey {
	return shadowKey{
		sourceConfigDigest: config.SourceConfigDigest,
		instanceIndex:      config.InstanceIndex,
	}
}

type shadowCheckHandle struct {
	check         check.Check
	cancel        context.CancelFunc
	senderManager sender.SenderManager
	runner        ShadowRunner
	ticker        ShadowTicker
	stopCh        chan struct{}
	stoppedCh     chan struct{}
	stopOnce      sync.Once

	mu      sync.Mutex
	started bool
}

func (h *shadowCheckHandle) start() {
	h.mu.Lock()
	h.started = true
	h.mu.Unlock()

	go func() {
		defer close(h.stoppedCh)
		for {
			select {
			case <-h.stopCh:
				return
			case <-h.ticker.C():
				checks := h.runner.GetChan()
				select {
				case checks <- h.check:
					continue
				default:
				}

				start := time.Now()
				select {
				case checks <- h.check:
					enqueueDuration := time.Since(start)
					checkName := checkid.IDToCheckName(h.check.ID())
					tlmShadowEnqueueDelays.Inc(checkName)
					tlmShadowEnqueueDelayDuration.Set(enqueueDuration.Seconds(), checkName)
					if enqueueDuration > h.check.Interval() {
						log.Debugf("Shadow check %s tick delayed by %s while enqueueing", h.check.ID(), enqueueDuration)
					}
				case <-h.stopCh:
					return
				}
			}
		}
	}()
}

func (h *shadowCheckHandle) stop(timeout time.Duration) error {
	var started bool
	h.stopOnce.Do(func() {
		h.mu.Lock()
		started = h.started
		h.mu.Unlock()
		close(h.stopCh)
		h.ticker.Stop()
		h.cancel()
	})

	var firstErr error
	if err := callWithTimeout(h.check.Stop, timeout); err != nil {
		firstErr = fmt.Errorf("timeout while calling Stop on shadow check %s: %w", h.check.ID(), err)
	}
	if err := callWithTimeout(h.check.Cancel, timeout); err != nil {
		err = fmt.Errorf("timeout while calling Cancel on shadow check %s: %w", h.check.ID(), err)
		if firstErr == nil {
			firstErr = err
		}
	}
	if started {
		if err := waitWithTimeout(h.stoppedCh, timeout); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("timeout while waiting for shadow check %s loop to stop: %w", h.check.ID(), err)
		}
	}
	h.senderManager.DestroySender(h.check.ID())
	// Shadow runs reuse the runner expvar stats source under the :shadow ID.
	// Remove those stats on unschedule/stop so status does not show stale
	// shadow checks after their source config is gone.
	if _, found := expvars.CheckStats(h.check.ID()); found {
		expvars.RemoveCheckStats(h.check.ID())
	}
	return firstErr
}

func callWithTimeout(fn func(), timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return nil
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

func waitWithTimeout(done <-chan struct{}, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return nil
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

type shadowCheck struct {
	check.Check
	id       checkid.ID
	interval time.Duration
}

func (c *shadowCheck) ID() checkid.ID {
	return c.id
}

func (c *shadowCheck) Interval() time.Duration {
	return c.interval
}

type shadowSenderManager struct {
	delegate sender.SenderManager
	sourceID checkid.ID
	shadowID checkid.ID
}

func (m *shadowSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	return m.delegate.GetSender(m.mapID(id))
}

func (m *shadowSenderManager) SetSender(s sender.Sender, id checkid.ID) error {
	return m.delegate.SetSender(s, m.mapID(id))
}

func (m *shadowSenderManager) DestroySender(id checkid.ID) {
	m.delegate.DestroySender(m.mapID(id))
}

func (m *shadowSenderManager) GetDefaultSender() (sender.Sender, error) {
	return m.delegate.GetDefaultSender()
}

func (m *shadowSenderManager) mapID(id checkid.ID) checkid.ID {
	if id == m.sourceID {
		return m.shadowID
	}
	return id
}

// ShadowTicker provides scheduler ticks. It is injectable for tests.
type ShadowTicker interface {
	C() <-chan time.Time
	Stop()
}

type realShadowTicker struct {
	ticker *time.Ticker
}

func newRealShadowTicker(interval time.Duration) ShadowTicker {
	return &realShadowTicker{ticker: time.NewTicker(interval)}
}

func (t *realShadowTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t *realShadowTicker) Stop() {
	t.ticker.Stop()
}
