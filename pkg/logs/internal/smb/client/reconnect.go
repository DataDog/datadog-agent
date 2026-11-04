// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	initialBackoff = time.Second
	maxBackoff     = 30 * time.Second
	// stableSession is how long a session must stay up for its loss to be
	// routine (an expired session, a server failover): it is then replaced
	// right away. A session lost sooner counts as a failed connection attempt.
	stableSession = 30 * time.Second
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
// A failed dial, and a session lost to a Transient error less than 30s after
// it was dialed, count as failed connection attempts: the following calls fail
// fast, without dialing, for an exponential backoff (1s, 2s, 4s, ... capped at
// 30s). So a server that accepts sessions but fails every request (overloaded,
// throttling) gets one new session per backoff period, not one per call. The
// backoff resets when a session stays up for 30s; the loss of such a session
// is routine (it expired, the server failed over), and the next call dials
// again right away. An Auth dial failure (bad password, locked account,
// unknown share) waits the full 30s between attempts, so a wrong password does
// not hammer the domain controller; with a Guard (WithGuard), a refused
// password is not sent again at all (see Guard). Calls made during a backoff return an
// error that classifies like the failure that started it.
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
	guard  *Guard // nil sends every logon

	mu      sync.Mutex
	cur     Client        // nil when not connected
	since   time.Time     // when cur was dialed
	dialing chan struct{} // closed when the dial in progress ends
	closed  bool
	closing Client // the session Close logs off, which Abort can cut short

	// Backoff state, set by failed connection attempts and reset by a stable
	// session (see NewReconnecting).
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

// Close implements Client. It logs the session off, which can take a few
// seconds with a server that stopped answering; Abort cuts it short. It does
// not wait for a dial in progress; that dial closes its session as soon as it
// completes.
func (r *reconnecting) Close() error {
	c := r.shutdown()
	if c == nil {
		return nil
	}
	return r.redact(c.Close())
}

// Abort closes the client like Close, but drops the session's connection
// without logging off, so it never waits for the server, and cuts short a
// Close waiting for its LOGOFF. See the Abort function.
func (r *reconnecting) Abort() error {
	r.shutdown()
	r.mu.Lock()
	c := r.closing
	r.mu.Unlock()
	if c == nil {
		return nil
	}
	return r.redact(Abort(c))
}

// shutdown marks the client closed and returns its session, which the caller
// must close, or nil if it has none or was already closed.
func (r *reconnecting) shutdown() Client {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	c := r.cur
	r.cur = nil
	r.closing = c
	return c
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
		if r.clock.Now().Before(r.retryAt) {
			err := &backoffError{err: r.lastErr}
			r.mu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		r.dialing = done
		r.mu.Unlock()

		c, err := r.connect(ctx)
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
			_ = Abort(c) // never used: dropping the connection is enough
			return nil, ErrClosed
		}
		r.cur, r.since = c, r.clock.Now()
		r.mu.Unlock()
		log.Infof("smb: connected to %s", r.target)
		return c, nil
	}
}

// connect dials a session, through the guard of the account when there is one.
func (r *reconnecting) connect(ctx context.Context) (Client, error) {
	if r.guard == nil {
		return r.dial(ctx, r.cfg)
	}
	// The guard keeps the error of the refused logon, which it hands to every
	// source of the account: redact it first.
	return r.guard.Dial(ctx, r.cfg, func(ctx context.Context, cfg Config) (Client, error) {
		c, err := r.dial(ctx, cfg)
		return c, r.redact(err)
	})
}

// recordFailure records a failed connection attempt and schedules the next
// dial. r.mu must be held.
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
	var delay time.Duration
	if r.clock.Since(r.since) >= stableSession {
		r.failures, r.retryAt, r.lastErr = 0, time.Time{}, nil
	} else {
		delay = r.recordFailure(err)
	}
	r.mu.Unlock()
	if delay > 0 {
		log.Warnf("smb: session to %s lost shortly after it was established, next attempt in %s: %v", r.target, delay, err)
	} else {
		log.Infof("smb: session to %s lost, reconnecting: %v", r.target, err)
	}
	_ = Abort(c) // the session is broken: skip LOGOFF
	return err
}

func (r *reconnecting) redact(err error) error {
	return redactErr(err, r.cfg.Password)
}

// backoffError is returned while a reconnection attempt is not due yet. It
// unwraps to the error that started the backoff, so it classifies the same
// way.
type backoffError struct {
	err error
}

// Error returns the message of the dial error that started the backoff. It
// leaves out the wait, which the dial failure's log line already gives: a
// message that changed on every call would defeat the deduplication of the
// launcher's status warnings, which would then log one per poll while the
// server is down.
func (e *backoffError) Error() string {
	return e.err.Error()
}

func (e *backoffError) Unwrap() error { return e.err }
