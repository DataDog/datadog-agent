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

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

const (
	defaultShadowInterval    = time.Second
	defaultShadowStopTimeout = 5 * time.Second
)

// SenderManagerFactory returns a sender manager scoped to one shadow check.
type SenderManagerFactory func(context.Context) sender.SenderManager

// RunStatsRecorder records completed shadow check runs.
type RunStatsRecorder interface {
	AddCheckStats(check.Check, time.Duration, error, []error, stats.SenderStats)
	RemoveCheckStats(checkid.ID)
}

// ShadowSchedulerOptions configures the shadow scheduler component.
type ShadowSchedulerOptions struct {
	Loader           check.Loader
	NewSenderManager SenderManagerFactory
	Interval         time.Duration
	StopTimeout      time.Duration
	NewTicker        func(time.Duration) ShadowTicker
	RunStatsRecorder RunStatsRecorder
}

// ShadowScheduler runs lookback shadow checks outside the normal collector
// scheduler and runner.
type ShadowScheduler struct {
	loader           check.Loader
	newSenderManager SenderManagerFactory
	interval         time.Duration
	stopTimeout      time.Duration
	newTicker        func(time.Duration) ShadowTicker
	statsRecorder    RunStatsRecorder

	mu     sync.Mutex
	checks map[shadowKey]*shadowCheckHandle
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
	statsRecorder := opts.RunStatsRecorder
	if statsRecorder == nil {
		statsRecorder = noopRunStatsRecorder{}
	}

	return &ShadowScheduler{
		loader:           opts.Loader,
		newSenderManager: opts.NewSenderManager,
		interval:         interval,
		stopTimeout:      stopTimeout,
		newTicker:        newTicker,
		statsRecorder:    statsRecorder,
		checks:           make(map[shadowKey]*shadowCheckHandle),
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

	var firstErr error
	for _, config := range configs {
		key := newShadowKey(config)

		s.mu.Lock()
		if _, exists := s.checks[key]; exists {
			s.mu.Unlock()
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
		if _, exists := s.checks[key]; exists {
			s.mu.Unlock()
			_ = handle.stop(s.stopTimeout)
			continue
		}
		s.checks[key] = handle
		handle.start()
		s.mu.Unlock()
	}

	return firstErr
}

func (s *ShadowScheduler) load(config ShadowConfig) (*shadowCheckHandle, error) {
	ctx, cancel := context.WithCancel(context.Background())
	shadowSenderManager := &shadowSenderManager{
		delegate: s.newSenderManager(ctx),
		sourceID: config.SourceCheckID,
		shadowID: config.ShadowCheckID,
	}

	loadedCheck, err := s.loader.Load(shadowSenderManager, config.SourceConfig, config.Instance, config.InstanceIndex)
	if err != nil {
		shadowSenderManager.DestroySender(config.ShadowCheckID)
		cancel()
		return nil, fmt.Errorf("load shadow check %s: %w", config.ShadowCheckID, err)
	}

	return &shadowCheckHandle{
		check:         &shadowCheck{Check: loadedCheck, id: config.ShadowCheckID, interval: s.interval},
		cancel:        cancel,
		senderManager: shadowSenderManager,
		ticker:        s.newTicker(s.interval),
		stopCh:        make(chan struct{}),
		stoppedCh:     make(chan struct{}),
		statsRecorder: s.statsRecorder,
	}, nil
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
	handles := make([]*shadowCheckHandle, 0, len(s.checks))
	for key, handle := range s.checks {
		handles = append(handles, handle)
		delete(s.checks, key)
	}
	s.mu.Unlock()

	var firstErr error
	for _, handle := range handles {
		if err := handle.stop(s.stopTimeout); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
	ticker        ShadowTicker
	stopCh        chan struct{}
	stoppedCh     chan struct{}
	stopOnce      sync.Once
	statsRecorder RunStatsRecorder

	mu      sync.Mutex
	running bool
	started bool
	stopped bool
	runWG   sync.WaitGroup
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
				h.runIfIdle()
			}
		}
	}()
}

func (h *shadowCheckHandle) runIfIdle() {
	h.mu.Lock()
	if h.stopped || h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.runWG.Add(1)
	h.mu.Unlock()

	go func() {
		defer h.runWG.Done()
		defer func() {
			h.mu.Lock()
			h.running = false
			h.mu.Unlock()
		}()

		start := time.Now()
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("check panicked: %v", r)
				}
			}()
			err = h.check.Run()
		}()
		execTime := time.Since(start)

		senderStats, statsErr := h.check.GetSenderStats()
		if err == nil {
			err = statsErr
		}
		if !h.isStopped() {
			h.statsRecorder.AddCheckStats(h.check, execTime, err, h.check.GetWarnings(), senderStats)
		}
	}()
}

func (h *shadowCheckHandle) stop(timeout time.Duration) error {
	var started bool
	h.stopOnce.Do(func() {
		h.mu.Lock()
		started = h.started
		h.stopped = true
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
		if err := waitGroupWithTimeout(&h.runWG, timeout); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("timeout while waiting for shadow check %s to stop: %w", h.check.ID(), err)
		}
	}

	h.senderManager.DestroySender(h.check.ID())
	h.statsRecorder.RemoveCheckStats(h.check.ID())
	return firstErr
}

func (h *shadowCheckHandle) isStopped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
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

func waitGroupWithTimeout(wg *sync.WaitGroup, timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	return waitWithTimeout(done, timeout)
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

type noopRunStatsRecorder struct{}

func (noopRunStatsRecorder) AddCheckStats(check.Check, time.Duration, error, []error, stats.SenderStats) {
}
func (noopRunStatsRecorder) RemoveCheckStats(checkid.ID) {}
