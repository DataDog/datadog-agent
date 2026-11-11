// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"sync"
	"time"
)

// resizableSemaphore is a counting semaphore whose limit can be changed while
// it is in use. It bounds the number of concurrent outgoing requests (and
// therefore connections) to a domain.
//
// Acquire blocks while the number of held tokens has reached the current
// limit. Lowering the limit never cancels in-flight holders: it only prevents
// new acquisitions until enough tokens have been released. Raising the limit
// immediately wakes any blocked acquirers.
//
// The semaphore also measures how much wall-clock time it spends fully
// saturated (all tokens in use). This is the signal used to decide whether the
// concurrency limit should grow: time spent saturated means callers wanted to
// send but could not, i.e. we are not keeping up at the current limit.
type resizableSemaphore struct {
	mu       sync.Mutex
	cond     *sync.Cond
	limit    int64
	inFlight int64

	// now is the clock used for saturation accounting; overridable in tests.
	now func() time.Time
	// saturatedSince is the time saturation began, or the zero value when the
	// semaphore is not currently saturated.
	saturatedSince time.Time
	// saturatedAccum accumulates saturated time since the last read.
	saturatedAccum time.Duration
}

var _ weightedSemaphore = (*resizableSemaphore)(nil)

// newResizableSemaphore returns a semaphore with the given initial limit.
func newResizableSemaphore(limit int64) *resizableSemaphore {
	s := &resizableSemaphore{limit: limit, now: time.Now}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Acquire takes n tokens, blocking until they are available or ctx is done.
// It returns ctx.Err() if the context is cancelled before the tokens are
// acquired, including when ctx is already done on entry even if tokens are
// available, matching semaphore.Weighted.
func (s *resizableSemaphore) Acquire(ctx context.Context, n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Wake this waiter if the context is cancelled while it is blocked, since
	// sync.Cond has no context-aware Wait.
	stop := context.AfterFunc(ctx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cond.Broadcast()
	})
	defer stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.inFlight+n <= s.limit {
			break
		}
		s.cond.Wait()
	}
	s.inFlight += n
	s.updateSaturation()
	return nil
}

// Release returns n tokens to the semaphore.
func (s *resizableSemaphore) Release(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inFlight -= n
	s.updateSaturation()
	s.cond.Broadcast()
}

// SetLimit changes the maximum number of tokens that can be held concurrently.
// It is safe to call at runtime. Raising the limit wakes blocked acquirers.
func (s *resizableSemaphore) SetLimit(limit int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.limit = limit
	s.updateSaturation()
	s.cond.Broadcast()
}

// updateSaturation records the transition into or out of the saturated state
// (all tokens in use). It must be called under s.mu after any change to
// inFlight or limit.
func (s *resizableSemaphore) updateSaturation() {
	saturated := s.inFlight >= s.limit
	switch {
	case saturated && s.saturatedSince.IsZero():
		s.saturatedSince = s.now()
	case !saturated && !s.saturatedSince.IsZero():
		s.saturatedAccum += s.now().Sub(s.saturatedSince)
		s.saturatedSince = time.Time{}
	}
}

// takeSaturatedDuration returns the time the semaphore spent saturated since the
// last call and resets the accumulator. An in-progress saturated span is folded
// in up to now and keeps running, so ongoing saturation accrues into the next
// window.
func (s *resizableSemaphore) takeSaturatedDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.saturatedSince.IsZero() {
		n := s.now()
		s.saturatedAccum += n.Sub(s.saturatedSince)
		s.saturatedSince = n
	}
	d := s.saturatedAccum
	s.saturatedAccum = 0
	return d
}
