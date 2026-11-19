// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

var (
	// errUnknownRange is returned for a range this Agent does not sweep.
	errUnknownRange = errors.New("unknown autodiscovery id")
	// errAlreadySweeping is returned for a range whose sweep is in flight.
	errAlreadySweeping = errors.New("a sweep is already running")
)

// schedulerOptions are the agent-wide settings the scheduler needs.
type schedulerOptions struct {
	MaxAddresses int
	Defaults     rangeDefaults
	// Credentials resolves a range's credential ids once per cycle.
	Credentials credentialStore
}

// scheduledRange is one active range and the goroutine sweeping it. The fields
// after cancel are guarded by scheduler.mu.
type scheduledRange struct {
	cfg    rangeConfig
	cancel context.CancelFunc

	// trigger carries an out-of-band request to sweep now. Buffered, so a
	// second request while one is pending is dropped rather than queued twice.
	trigger chan struct{}
	// prepared is what the latest wake found unusable, refreshed so it tracks credential rotations.
	prepared []string
	// sweepErr is the failure of the last sweep that ran.
	sweepErr string
	// sweeping is whether a sweep of this range is running now.
	sweeping bool
}

// scheduler owns the sweep schedule: one goroutine per range, sharing one worker budget.
type scheduler struct {
	sweeper *sweeper
	log     log.Component
	opts    schedulerOptions

	newTimer func(d time.Duration) (<-chan time.Time, func())

	mu     sync.Mutex
	ranges map[string]*scheduledRange
	// cycles holds, per range id, the completion channel a replacement waits on.
	cycles map[string]chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newScheduler(sw *sweeper, logger log.Component, opts schedulerOptions) *scheduler {
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

// start makes the scheduler accept ranges.
func (s *scheduler) start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
}

// stop cancels every range and waits for the running sweeps to unwind.
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

// set adds or replaces a range. A range that is too large is rejected.
func (s *scheduler) set(cfg rangeConfig) error {
	if _, err := newChunkPlan(cfg.NetworkAddress, cfg.IgnoredIPAddresses, s.opts.MaxAddresses); err != nil {
		return err
	}

	s.mu.Lock()
	if s.ctx == nil {
		s.mu.Unlock()
		return errors.New("the discovery scheduler is not running")
	}
	var replaced context.CancelFunc
	if existing, ok := s.ranges[cfg.AutodiscoveryID]; ok {
		if existing.cfg.equal(cfg) {
			s.mu.Unlock()
			return nil
		}
		replaced = existing.cancel
	}
	rangeCtx, rangeCancel := context.WithCancel(s.ctx)
	sr := &scheduledRange{cfg: cfg, cancel: rangeCancel, trigger: make(chan struct{}, 1)}
	s.ranges[cfg.AutodiscoveryID] = sr
	previous := s.cycles[cfg.AutodiscoveryID]
	done := make(chan struct{})
	s.cycles[cfg.AutodiscoveryID] = done
	// Counted under the lock, so a concurrent stop cannot start draining first.
	s.wg.Add(1)
	s.mu.Unlock()

	if replaced != nil {
		replaced()
	}

	go func() {
		defer s.wg.Done()
		defer s.finishCycles(cfg.AutodiscoveryID, done)

		if previous != nil {
			// Waiting for the predecessor keeps one cursor under one writer.
			<-previous
		}
		if rangeCtx.Err() != nil {
			return
		}
		s.run(rangeCtx, sr)
	}()
	return nil
}

// triggerNow asks a scheduled range to sweep immediately. It refuses a range
// that is already sweeping; the caller is expected to retry later.
func (s *scheduler) triggerNow(autodiscoveryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.ranges[autodiscoveryID]
	if !ok {
		return fmt.Errorf("%w: no range %q is scheduled on this Agent", errUnknownRange, autodiscoveryID)
	}
	if r.sweeping {
		return fmt.Errorf("%w: range %q is already sweeping", errAlreadySweeping, autodiscoveryID)
	}

	select {
	case r.trigger <- struct{}{}:
	default:
		// A request is already pending, and one sweep satisfies both.
	}
	return nil
}

// finishCycles releases the goroutines waiting on this range's turn.
func (s *scheduler) finishCycles(autodiscoveryID string, done chan struct{}) {
	s.mu.Lock()
	if s.cycles[autodiscoveryID] == done {
		delete(s.cycles, autodiscoveryID)
	}
	s.mu.Unlock()
	close(done)
}

// remove stops a range and drops its cursor once the range has unwound.
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

// run sweeps one range once per interval, sequentially, counting from the end
// of the last completed cycle.
func (s *scheduler) run(ctx context.Context, sr *scheduledRange) {
	// A non-positive interval would spin.
	interval := time.Duration(sr.cfg.IntervalSec) * time.Second
	if floor := time.Duration(minIntervalSec) * time.Second; interval < floor {
		interval = floor
	}

	// fresh carries a request to sweep now into the cycle that follows it.
	fresh := false
	for {
		wait := s.runCycle(ctx, sr, interval, fresh)
		fresh = false
		if wait <= 0 {
			wait = interval
		}

		tick, stopTimer := s.newTimer(wait)
		select {
		case <-ctx.Done():
			stopTimer()
			return
		case <-tick:
		case <-sr.trigger:
			fresh = true
			s.log.Infof("ndmdiscovery: sweeping range %s now, on request", sr.cfg.AutodiscoveryID)
		}
		stopTimer()
	}
}

// runCycle sweeps the range when it is due, and otherwise returns how long is
// left before it is.
func (s *scheduler) runCycle(ctx context.Context, sr *scheduledRange, interval time.Duration, fresh bool) time.Duration {
	if ctx.Err() != nil {
		return 0
	}

	cfg := sr.cfg
	id := cfg.AutodiscoveryID

	plan, err := newChunkPlan(cfg.NetworkAddress, cfg.IgnoredIPAddresses, s.opts.MaxAddresses)
	if err != nil {
		s.log.Warnf("ndmdiscovery: skipping range %s: %v", id, err)
		s.locked(func() { sr.prepared = []string{"skipped: " + err.Error()} })
		return 0
	}

	// Resolved per cycle, so a credential rotation lands without a restart.
	opts, dropped := cfg.Probes.resolve(s.opts.Credentials)
	for _, reason := range dropped {
		s.log.Warnf("ndmdiscovery: range %s: %s", id, reason)
	}
	if opts.Empty() {
		s.log.Warnf("ndmdiscovery: skipping range %s: it has no usable probe", id)
		s.locked(func() { sr.prepared = append(dropped, "skipped: it has no usable probe") })
		return 0
	}
	s.locked(func() { sr.prepared = dropped })

	req := sweepRequest{
		Config:   cfg,
		Options:  opts,
		Plan:     plan,
		Digest:   rangeDigest(cfg, opts.Fingerprints()),
		Interval: interval,
		Fresh:    fresh,
	}

	if wait := s.sweeper.dueIn(req); wait > 0 {
		s.log.Debugf("ndmdiscovery: range %s is not due for another %s", cfg.AutodiscoveryID, wait)
		return wait
	}

	s.locked(func() { sr.sweeping = true })
	err = s.sweeper.sweep(ctx, req)
	s.locked(func() {
		sr.sweeping = false
		switch {
		case err == nil:
			sr.sweepErr = ""
		case ctx.Err() == nil:
			sr.sweepErr = err.Error()
		}
	})
	if err != nil && ctx.Err() == nil {
		s.log.Warnf("ndmdiscovery: sweep of range %s failed: %v", id, err)
	}
	return 0
}

// locked runs fn under the scheduler lock.
func (s *scheduler) locked(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// rangeSnapshot is one scheduled range as the status page shows it.
type rangeSnapshot struct {
	cfg      rangeConfig
	sweeping bool
	issues   []string
}

// snapshot returns every scheduled range, with whether it is sweeping and its latest issues.
func (s *scheduler) snapshot() []rangeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]rangeSnapshot, 0, len(s.ranges))
	for _, r := range s.ranges {
		issues := append([]string(nil), r.cfg.Probes.Dropped...)
		issues = append(issues, r.prepared...)
		if r.sweepErr != "" {
			issues = append(issues, "last sweep failed: "+r.sweepErr)
		}
		out = append(out, rangeSnapshot{cfg: r.cfg, sweeping: r.sweeping, issues: issues})
	}
	return out
}
