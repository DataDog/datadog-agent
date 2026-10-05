// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	initialBackoff = time.Second
	maxBackoff     = 30 * time.Second
)

// Option configures NewReconnecting.
type Option func(*reconnecting)

// WithDialer replaces Dial, so tests can connect to a fake share.
func WithDialer(dial DialFunc) Option {
	return func(r *reconnecting) { r.dial = dial }
}

// WithClock replaces the clock that schedules reconnection attempts.
func WithClock(clk clock.Clock) Option {
	return func(r *reconnecting) { r.clock = clk }
}

// NewReconnecting returns a Client that dials lazily, on its first call, and
// replaces its session after a Transient error.
//
// After a Transient error the session is dropped and the next call dials
// again right away. A failed dial makes the following calls fail fast, without
// dialing, for an exponential backoff (1s, 2s, 4s, ... capped at 30s) that
// resets after a successful dial. An Auth dial failure (bad password, locked
// account, unknown share) waits the full 30s between attempts, so a wrong
// password does not hammer the domain controller. Calls made during a backoff
// return an error that classifies like the dial failure that started it.
//
// Errors other than Transient (NotFound, Sharing, Auth on one file) keep the
// session. So does an error caused by the caller's own context ending, because
// the session may be shared with other callers. The password never appears in
// a log line or a returned error.
func NewReconnecting(cfg Config, opts ...Option) Client {
	r := &reconnecting{
		cfg:    cfg,
		target: cfg.withDefaults().target(),
		dial:   Dial,
		clock:  clock.New(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type reconnecting struct {
	cfg    Config
	target string
	dial   DialFunc
	clock  clock.Clock

	mu      sync.Mutex
	cur     Client        // nil when not connected
	dialing chan struct{} // closed when the dial in progress ends
	closed  bool

	// Backoff state, set by failed dials and reset by a successful one.
	failures int
	retryAt  time.Time
	lastErr  error
}

// ListDir implements Client.
func (r *reconnecting) ListDir(ctx context.Context, dir string) ([]Entry, error) {
	c, err := r.session(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := c.ListDir(ctx, dir)
	if err != nil {
		return nil, r.failed(ctx, c, err)
	}
	return entries, nil
}

// ReadAt implements Client.
func (r *reconnecting) ReadAt(ctx context.Context, path string, off int64, maxLen int) (ReadResult, error) {
	c, err := r.session(ctx)
	if err != nil {
		return ReadResult{}, err
	}
	res, err := c.ReadAt(ctx, path, off, maxLen)
	if err != nil {
		return ReadResult{}, r.failed(ctx, c, err)
	}
	return res, nil
}

// Close implements Client. It does not wait for a dial in progress; that dial
// closes its session as soon as it completes.
func (r *reconnecting) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	c := r.cur
	r.cur = nil
	r.mu.Unlock()
	if c == nil {
		return nil
	}
	return r.redact(c.Close())
}

// String implements fmt.Stringer, so printing the client never prints its
// configuration's password.
func (r *reconnecting) String() string { return r.target }

// GoString implements fmt.GoStringer for the same reason.
func (r *reconnecting) GoString() string { return "client.reconnecting{" + r.target + "}" }

// session returns the current session, dialing one if needed. Concurrent
// callers share a single dial.
func (r *reconnecting) session(ctx context.Context) (Client, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, ErrClosed
		}
		if r.cur != nil {
			c := r.cur
			r.mu.Unlock()
			return c, nil
		}
		if wait := r.dialing; wait != nil {
			r.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if now := r.clock.Now(); now.Before(r.retryAt) {
			err := &backoffError{target: r.target, retryIn: r.retryAt.Sub(now), err: r.lastErr}
			r.mu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		r.dialing = done
		r.mu.Unlock()

		c, err := r.dial(ctx, r.cfg)
		err = r.redact(err)

		r.mu.Lock()
		r.dialing = nil
		close(done)
		switch {
		case err != nil && ctx.Err() != nil:
			// Our caller gave up (e.g. the launcher is stopping): this says
			// nothing about the server, so do not back off.
			r.mu.Unlock()
			return nil, err
		case err != nil:
			delay := r.recordFailure(err)
			r.mu.Unlock()
			log.Warnf("smb: cannot connect to %s, next attempt in %s: %v", r.target, delay, err)
			return nil, err
		case r.closed:
			r.mu.Unlock()
			_ = c.Close()
			return nil, ErrClosed
		}
		r.cur = c
		r.failures, r.retryAt, r.lastErr = 0, time.Time{}, nil
		r.mu.Unlock()
		log.Infof("smb: connected to %s", r.target)
		return c, nil
	}
}

// recordFailure schedules the next dial attempt. r.mu must be held.
func (r *reconnecting) recordFailure(err error) time.Duration {
	r.failures++
	delay := maxBackoff
	if Classify(err) != ErrAuth && r.failures <= 5 { // 1s << 5 > 30s
		delay = min(initialBackoff<<(r.failures-1), maxBackoff)
	}
	r.retryAt = r.clock.Now().Add(delay)
	r.lastErr = err
	return delay
}

// failed drops c after a Transient error and returns err, redacted.
func (r *reconnecting) failed(ctx context.Context, c Client, err error) error {
	err = r.redact(err)
	if ctx.Err() != nil || Classify(err) != ErrTransient {
		return err
	}
	r.mu.Lock()
	if r.cur != c { // already dropped by a concurrent call, or closed
		r.mu.Unlock()
		return err
	}
	r.cur = nil
	r.mu.Unlock()
	log.Infof("smb: session to %s lost, reconnecting: %v", r.target, err)
	if a, ok := c.(interface{ abort() }); ok {
		a.abort() // the session is broken: skip LOGOFF
	} else {
		_ = c.Close()
	}
	return err
}

func (r *reconnecting) redact(err error) error {
	return redactErr(err, r.cfg.Password)
}

// backoffError is returned while a reconnection attempt is not due yet. It
// unwraps to the dial error that started the backoff, so it classifies the
// same way.
type backoffError struct {
	target  string
	retryIn time.Duration
	err     error
}

func (e *backoffError) Error() string {
	// Round up so a sub-second wait does not read as "0s".
	retryIn := (e.retryIn + time.Second - 1).Truncate(time.Second)
	return fmt.Sprintf("smb: not connected to %s (next attempt in %s): %v", e.target, retryIn, e.err)
}

func (e *backoffError) Unwrap() error { return e.err }
