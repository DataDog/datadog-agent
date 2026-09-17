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
	creds   credentialStore
	log     log.Component
	opts    schedulerOptions

	// newTicker is injectable so tests can drive time.
	newTicker func(d time.Duration) (<-chan time.Time, func())

	mu     sync.Mutex
	ranges map[string]*scheduledRange
	// cycles holds, per range id, the completion channel of the most recently
	// launched goroutine, which a replacement waits on before it starts.
	cycles      map[string]chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	pingEnabled bool
	// wg is reused across start/stop pairs, which the sequential fx lifecycle
	// hooks never call concurrently.
	wg sync.WaitGroup
}

func newScheduler(sw *sweeper, creds credentialStore, logger log.Component, opts schedulerOptions) *scheduler {
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
		creds:   creds,
		log:     logger,
		opts:    opts,
		ranges:  map[string]*scheduledRange{},
		cycles:  map[string]chan struct{}{},
		newTicker: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
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

func (s *scheduler) setPingEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingEnabled = enabled
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

// set adds or replaces a range. A range whose credentials are unavailable or
// which is too large is rejected.
func (s *scheduler) set(cfg rangeConfig) error {
	// Validate before touching any state so a bad update cannot take down a
	// range that is already running well.
	if _, err := resolveCredentials(s.creds, cfg.CredentialIDs); err != nil {
		return err
	}
	if _, err := newChunkPlan(cfg.CIDR, cfg.IgnoredIPAddresses, s.opts.MaxAddresses); err != nil {
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

// remove stops a range. Removing an unknown range is a no-op.
func (s *scheduler) remove(autodiscoveryID string) {
	s.mu.Lock()
	r, ok := s.ranges[autodiscoveryID]
	delete(s.ranges, autodiscoveryID)
	s.mu.Unlock()

	if ok {
		r.cancel()
	}
}

// run sweeps one range immediately, then once per interval. Cycles are
// sequential: a tick during a cycle starts the next one once that cycle ends.
func (s *scheduler) run(ctx context.Context, cfg rangeConfig) {
	// A non-positive interval would panic in time.NewTicker.
	floor := time.Duration(minIntervalSec) * time.Second
	d := time.Duration(cfg.IntervalSec) * time.Second
	if d < floor {
		d = floor
	}

	tick, stopTicker := s.newTicker(d)
	defer stopTicker()

	for {
		s.runCycle(ctx, cfg)

		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

func (s *scheduler) runCycle(ctx context.Context, cfg rangeConfig) {
	if ctx.Err() != nil {
		return
	}

	// The store re-reads the configuration here, so a Fleet-pushed rotation
	// applies without an agent restart.
	creds, err := resolveCredentials(s.creds, cfg.CredentialIDs)
	if err != nil {
		s.log.Warnf("ndmdiscovery: skipping range %s: %v", cfg.AutodiscoveryID, err)
		return
	}

	plan, err := newChunkPlan(cfg.CIDR, cfg.IgnoredIPAddresses, s.opts.MaxAddresses)
	if err != nil {
		s.log.Warnf("ndmdiscovery: skipping range %s: %v", cfg.AutodiscoveryID, err)
		return
	}

	s.mu.Lock()
	pingEnabled := s.pingEnabled
	s.mu.Unlock()

	req := sweepRequest{
		Config:      cfg,
		Credentials: creds,
		Plan:        plan,
		Digest:      rangeDigest(cfg, creds),
		Workers:     s.workerShare(),
		PingEnabled: pingEnabled,
	}

	if err := s.sweeper.sweep(ctx, req); err != nil && ctx.Err() == nil {
		s.log.Warnf("ndmdiscovery: sweep of range %s failed: %v", cfg.AutodiscoveryID, err)
	}
}
