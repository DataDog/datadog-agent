// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/benbjohnson/clock"
)

// Discovery owns PAR scope polling in core; RC supplies only org/key status.
type Discovery struct {
	cfg           model.Reader
	clock         clock.Clock
	rc            Source
	demand        atomic.Bool
	client        *http.Client
	key, endpoint string
	jitter        func(time.Duration) time.Duration
	mu            sync.Mutex
	observation   PARScopeObservation
	next          time.Time
	backoff       time.Duration
	cancelRequest context.CancelFunc
	cancel        context.CancelFunc
	done          chan struct{}
}

// NewDiscovery creates an opt-in worker without starting background work.
// Unsupported configuration or absent RC leaves discovery unavailable.
func NewDiscovery(cfg model.Reader, rc Source) *Discovery {
	if !Enabled(cfg) || rc == nil {
		return nil
	}
	p := &Discovery{cfg: cfg, clock: clock.New(), rc: rc, key: cfg.GetString("api_key"), endpoint: enrollmentEndpoint(cfg),
		jitter: func(d time.Duration) time.Duration { return time.Duration(rand.Int64N(int64(d) + 1)) },
		client: &http.Client{Timeout: 10 * time.Second, Transport: httputils.CreateHTTPTransport(cfg), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	p.next = p.clock.Now().Add(p.jitter(time.Minute))
	p.observation = PARScopeObservation{Reason: "par_scope_unchecked", NextCheck: p.next}
	return p
}

// GetPARScope checks the current gate without forcing a discovery request.
func (p *Discovery) GetPARScope() PARScopeObservation {
	if p == nil {
		return PARScopeObservation{Reason: "par_scope_unchecked"}
	}
	now := p.clock.Now()
	ready := p.gate()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !ready {
		p.clearLocked()
		p.observation.Reason = "par_scope_waiting_for_rc_or_demand"
	}
	result := p.observation
	if result.Reason == "scope_present" && !result.Granted(now) {
		result.Reason = "par_scope_expired"
	}
	return result
}

// SetPARDiscoveryDemand expresses interest, not permission to issue a request.
func (p *Discovery) SetPARDiscoveryDemand(wanted bool) {
	if p != nil && p.demand.Swap(wanted) != wanted {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.clearLocked()
	}
}

func (p *Discovery) gate() bool {
	return p.demand.Load() && Enabled(p.cfg) && p.rc.Read(p.clock.Now()).Ready
}

// Start runs the worker once, under the PAR component's lifecycle.
func (p *Discovery) Start() {
	if p == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		defer p.client.CloseIdleConnections()
		for {
			p.step(ctx)
			timer := p.clock.Timer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

// Stop cancels discovery and waits for the worker started by Start.
func (p *Discovery) Stop() {
	if p == nil {
		return
	}
	p.SetPARDiscoveryDemand(false)
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
}

func (p *Discovery) clearLocked() {
	p.observation = PARScopeObservation{Reason: "par_scope_unchecked", NextCheck: p.next}
	if p.cancelRequest != nil {
		p.cancelRequest()
	}
}

// Check again at HTTP dispatch; the Agent transport still handles proxy/TLS.
type parGatedTransport struct {
	base    http.RoundTripper
	allowed func() bool
}

func (t parGatedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Context().Err() != nil || !t.allowed() {
		return nil, errors.New("PAR discovery prerequisite unavailable")
	}
	return t.base.RoundTrip(r)
}

func (p *Discovery) step(ctx context.Context) {
	now := p.clock.Now()
	ready := p.gate()
	p.mu.Lock()
	if !ready || ctx.Err() != nil {
		p.clearLocked()
		p.mu.Unlock()
		return
	}
	if now.Before(p.next) || p.cancelRequest != nil {
		p.mu.Unlock()
		return
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	p.cancelRequest = cancel
	// Reserve a cadence so pause/reopen cannot trigger a request burst.
	p.next = now.Add(max(pollInterval, p.backoff))
	p.observation.NextCheck = p.next
	p.mu.Unlock()
	stopWatch, watchDone := make(chan struct{}), make(chan struct{})
	// Arm before dispatch so the first cancellation tick cannot be missed.
	ticker := p.clock.Ticker(time.Second)
	go func() {
		defer close(watchDone)
		defer ticker.Stop()
		for {
			select {
			case <-stopWatch:
				return
			case <-requestCtx.Done():
				return
			case <-ticker.C:
				if !p.gate() {
					cancel()
					return
				}
			}
		}
	}()
	client := *p.client
	client.Transport = parGatedTransport{base: p.client.Transport, allowed: p.gate}
	result := validate(requestCtx, &client, p.endpoint, p.key, p.clock.Now())
	close(stopWatch)
	<-watchDone
	ready = p.gate()
	finished := p.clock.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelRequest = nil
	valid := ready && ctx.Err() == nil && requestCtx.Err() == nil
	if result.transient && requestCtx.Err() == nil {
		p.backoff = min(slowRetry, max(pollInterval, 2*p.backoff))
		result.delay = p.backoff
	} else if valid {
		p.backoff = 0
	}
	// Retain timing hints even when the response cannot supply a grant.
	if result.delay > 0 {
		delay := result.delay
		extra := p.jitter(delay / 5)
		// Avoid overflow for saturated Retry-After.
		if extra <= time.Duration(1<<63-1)-delay {
			delay += extra
		}
		if next := finished.Add(delay); next.After(p.next) {
			p.next = next
		}
	}
	if valid {
		p.observation = PARScopeObservation{Reason: result.reason, CheckedAt: now}
	} else {
		p.observation = PARScopeObservation{Reason: "par_scope_unchecked"}
	}
	p.observation.NextCheck = p.next
}
