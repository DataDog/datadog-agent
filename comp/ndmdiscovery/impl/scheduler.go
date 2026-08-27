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
// they share one worker budget, so a large range cannot starve the others the
// way the legacy sequential listener does.
type scheduler struct {
	sweeper *sweeper
	creds   credentialStore
	log     log.Component
	opts    schedulerOptions

	// newTicker is injectable so tests can drive time.
	newTicker func(d time.Duration) (<-chan time.Time, func())

	mu     sync.Mutex
	ranges map[string]*scheduledRange
	// cycles holds, per autodiscovery ID, the completion channel of the most
	// recently launched goroutine for that ID, including one that has been
	// cancelled but has not unwound yet. A replacement waits on it so that two
	// cycles for one range can never interleave their cursor writes.
	cycles      map[string]chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	pingEnabled bool
	wg          sync.WaitGroup
}

func newScheduler(sw *sweeper, creds credentialStore, logger log.Component, opts schedulerOptions) *scheduler {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	// The sweeper's budget is the size of the semaphore every range contends
	// on. Handing out a larger share would block on Acquire until the context
	// is done, so the schedule is capped by what the sweeper can actually
	// grant rather than by a second, independent worker number.
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

// set adds or replaces a range. A config whose credentials are unavailable or
// whose range is too large is rejected, and the error is surfaced to the
// backend through applyStateCallback.
func (s *scheduler) set(cfg rangeConfig) error {
	// Validate before touching any state so a bad update cannot take down a
	// range that is already running well.
	if err := s.creds.Reload(); err != nil {
		return err
	}
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
	if existing, ok := s.ranges[cfg.AutodiscoveryID]; ok {
		existing.cancel()
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

	go func() {
		defer s.wg.Done()
		defer s.finishCycles(cfg.AutodiscoveryID, done)

		if previous != nil {
			// The range this one replaces is cancelled but may still be inside
			// a probe. Waiting for it keeps one cursor under one writer.
			select {
			case <-previous:
			case <-rangeCtx.Done():
				return
			}
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
// sequential: a tick that arrives while a cycle is still running starts the
// next one as soon as that cycle ends rather than alongside it.
func (s *scheduler) run(ctx context.Context, cfg rangeConfig) {
	tick, stopTicker := s.newTicker(time.Duration(cfg.IntervalSec) * time.Second)
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

	// Re-read the credentials at the start of every cycle so a Fleet-pushed
	// rotation applies without an agent restart.
	if err := s.creds.Reload(); err != nil {
		s.log.Warnf("ndmdiscovery: failed to reload credentials for range %s: %v", cfg.AutodiscoveryID, err)
	}
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
