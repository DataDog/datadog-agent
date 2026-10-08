// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"errors"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// schedulerOptions are the agent-wide settings the scheduler needs.
type schedulerOptions struct {
	// Workers is the global sweep worker budget, shared by every range.
	Workers      int64
	MaxAddresses int
	Defaults     rangeDefaults
	// Credentials resolves a range's credential ids once per cycle.
	Credentials credentialStore
}

// scheduledRange is one active range and the goroutine sweeping it.
type scheduledRange struct {
	cfg    rangeConfig
	cancel context.CancelFunc
}

// scheduler owns the sweep schedule. Every range runs its own goroutine and
// they share one worker budget.
type scheduler struct {
	sweeper *sweeper
	log     log.Component
	opts    schedulerOptions

	// newTimer is injectable so tests can drive time.
	newTimer func(d time.Duration) (<-chan time.Time, func())

	mu     sync.Mutex
	ranges map[string]*scheduledRange
	// cycles holds, per range id, the completion channel of the most recently
	// launched goroutine, which a replacement waits on before it starts.
	cycles map[string]chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	// wg is reused across start/stop pairs, which the sequential fx lifecycle
	// hooks never call concurrently.
	wg sync.WaitGroup
}

func newScheduler(sw *sweeper, logger log.Component, opts schedulerOptions) *scheduler {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	// A share larger than the semaphore would block on Acquire until the
	// context is done, so the sweeper's budget caps the schedule.
	if sw != nil && opts.Workers > sw.budget {
		opts.Workers = sw.budget
	}
	return &scheduler{
		sweeper: sw,
		log:     logger,
		opts:    opts,
		ranges:  map[string]*scheduledRange{},
		cycles:  map[string]chan struct{}{},
		newTimer: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTimer(d)
			return t.C, func() { t.Stop() }
		},
	}
}

// start makes the scheduler accept ranges. Ranges added before start are not
// swept until it is called.
func (s *scheduler) start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
}

// stop cancels every range and waits for the running sweeps to unwind. It is
// idempotent.
func (s *scheduler) stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.ctx = nil
	s.ranges = map[string]*scheduledRange{}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

// ids returns the ids of the active ranges.
func (s *scheduler) ids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.ranges))
	for id := range s.ranges {
		ids = append(ids, id)
	}
	return ids
}

func (s *scheduler) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ranges)
}

// workerShare is this dispatch's slice of the global budget. It is recomputed
// per dispatch, so adding a range retimes the split without a restart.
func (s *scheduler) workerShare() int64 {
	s.mu.Lock()
	active := int64(len(s.ranges))
	s.mu.Unlock()

	if active < 1 {
		active = 1
	}
	share := s.opts.Workers / active
	if share < 1 {
		share = 1
	}
	return share
}

// set adds or replaces a range. A range that is too large is rejected.
func (s *scheduler) set(cfg rangeConfig) error {
	// Validate before touching any state so a bad update cannot take down a
	// range that is already running well.
	if _, err := newChunkPlan(cfg.NetworkAddress, cfg.IgnoredIPAddresses, s.opts.MaxAddresses); err != nil {
		return err
	}

	s.mu.Lock()
	if s.ctx == nil {
		s.mu.Unlock()
		return errors.New("the discovery scheduler is not running")
	}
	// Cancelled after the lock is released, the way remove does it.
	var replaced context.CancelFunc
	if existing, ok := s.ranges[cfg.AutodiscoveryID]; ok {
		replaced = existing.cancel
	}
	rangeCtx, rangeCancel := context.WithCancel(s.ctx)
	s.ranges[cfg.AutodiscoveryID] = &scheduledRange{cfg: cfg, cancel: rangeCancel}
	previous := s.cycles[cfg.AutodiscoveryID]
	done := make(chan struct{})
	s.cycles[cfg.AutodiscoveryID] = done
	// Counted under the lock: stop takes the same lock before it waits, so a
	// concurrent stop cannot start draining before this goroutine is counted.
	s.wg.Add(1)
	s.mu.Unlock()

	if replaced != nil {
		replaced()
	}

	go func() {
		defer s.wg.Done()
		defer s.finishCycles(cfg.AutodiscoveryID, done)

		if previous != nil {
			// Unconditional: waiting for the cancelled predecessor to unwind
			// keeps one cursor under one writer.
			<-previous
		}
		if rangeCtx.Err() != nil {
			return
		}
		s.run(rangeCtx, cfg)
	}()
	return nil
}

// finishCycles releases the goroutines waiting on this range's turn, and drops
// the bookkeeping entry when no newer goroutine has claimed it.
func (s *scheduler) finishCycles(autodiscoveryID string, done chan struct{}) {
	s.mu.Lock()
	if s.cycles[autodiscoveryID] == done {
		delete(s.cycles, autodiscoveryID)
	}
	s.mu.Unlock()
	close(done)
}

// remove stops a range and drops its cursor once the range has unwound.
// Removing an unknown range is a no-op.
func (s *scheduler) remove(autodiscoveryID string) {
	s.mu.Lock()
	r, ok := s.ranges[autodiscoveryID]
	delete(s.ranges, autodiscoveryID)
	done := s.cycles[autodiscoveryID]
	if ok {
		s.wg.Add(1)
	}
	s.mu.Unlock()

	if !ok {
		return
	}
	r.cancel()

	go func() {
		defer s.wg.Done()
		if done != nil {
			<-done
		}
		if err := s.sweeper.cursors.Clear(autodiscoveryID); err != nil {
			s.log.Warnf("ndmdiscovery: failed to clear the cursor of range %s: %v", autodiscoveryID, err)
		}
	}()
}

// run sweeps one range once per interval, counting from the end of the last
// completed cycle, so an Agent restart resumes the schedule instead of
// resetting it. Cycles are sequential.
func (s *scheduler) run(ctx context.Context, cfg rangeConfig) {
	// A non-positive interval would spin.
	interval := time.Duration(cfg.IntervalSec) * time.Second
	if floor := time.Duration(minIntervalSec) * time.Second; interval < floor {
		interval = floor
	}

	for {
		wait := s.runCycle(ctx, cfg, interval)
		if wait <= 0 {
			wait = interval
		}

		tick, stopTimer := s.newTimer(wait)
		select {
		case <-ctx.Done():
			stopTimer()
			return
		case <-tick:
		}
		stopTimer()
	}
}

// runCycle sweeps the range when it is due, and otherwise returns how long is
// left before it is.
func (s *scheduler) runCycle(ctx context.Context, cfg rangeConfig, interval time.Duration) time.Duration {
	if ctx.Err() != nil {
		return 0
	}

	plan, err := newChunkPlan(cfg.NetworkAddress, cfg.IgnoredIPAddresses, s.opts.MaxAddresses)
	if err != nil {
		s.log.Warnf("ndmdiscovery: skipping range %s: %v", cfg.AutodiscoveryID, err)
		return 0
	}

	// Resolved per cycle, so a credential rotation lands without a restart.
	opts, dropped := cfg.Probes.resolve(s.opts.Credentials)
	for _, reason := range dropped {
		s.log.Warnf("ndmdiscovery: range %s: %s", cfg.AutodiscoveryID, reason)
	}
	if opts.Empty() {
		s.log.Warnf("ndmdiscovery: skipping range %s: it has no usable probe", cfg.AutodiscoveryID)
		return 0
	}

	req := sweepRequest{
		Config:  cfg,
		Options: opts,
		Plan:    plan,
		Digest:  rangeDigest(cfg, opts.Fingerprints()),
		Workers: s.workerShare(),
	}

	if wait := s.sweeper.dueIn(req, interval); wait > 0 {
		s.log.Debugf("ndmdiscovery: range %s is not due for another %s", cfg.AutodiscoveryID, wait)
		return wait
	}

	if err := s.sweeper.sweep(ctx, req); err != nil && ctx.Err() == nil {
		s.log.Warnf("ndmdiscovery: sweep of range %s failed: %v", cfg.AutodiscoveryID, err)
	}
	return 0
}
