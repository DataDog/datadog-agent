// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/benbjohnson/clock"
)

// WithPARScopeDiscovery attaches the opt-in worker only to the core RC component,
// not every user of the remote-config library (including PAR's own RC client).
func WithPARScopeDiscovery() func(*options) {
	return func(o *options) { o.parScopeDiscovery = true }
}

// This is deliberately RC-side. Demand never authorizes a request and cannot
// reset the independent AAA deadline. It is not a second enrollment controller.
type parScopePoller struct {
	cfg                       model.Reader
	clock                     clock.Clock
	status                    func() OrgStatus
	demand                    *atomic.Bool
	client                    *http.Client
	key, endpoint, rcEndpoint string
	binding                   [32]byte
	jitter                    func(time.Duration) time.Duration
	mu                        sync.Mutex
	observation               PARScopeObservation
	next                      time.Time
	backoff                   time.Duration
	epoch                     uint64
	cancelRequest             context.CancelFunc
	cancel                    context.CancelFunc
	done                      chan struct{}
}

func newPARScopePoller(cfg model.Reader, clk clock.Clock, status func() OrgStatus, demand *atomic.Bool) *parScopePoller {
	if os.Getenv("DD_PAR_PHONE_HOME_POC") != "true" || !cfg.GetBool("private_action_runner.enabled") {
		return nil
	}
	key, endpoint, rcEndpoint, compatible := parDiscoveryConfig(cfg)
	if !compatible {
		return nil
	}
	p := &parScopePoller{cfg: cfg, clock: clk, status: status, demand: demand, key: key, endpoint: endpoint, rcEndpoint: rcEndpoint,
		binding: orgStatusBinding(key, endpoint+"\x00"+rcEndpoint), jitter: func(d time.Duration) time.Duration { return time.Duration(rand.Int64N(int64(d) + 1)) },
		client: &http.Client{Timeout: 10 * time.Second, Transport: httputils.CreateHTTPTransport(cfg), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	p.next = clk.Now().Add(p.jitter(time.Minute))
	p.observation = PARScopeObservation{Reason: "par_scope_unchecked", NextCheck: p.next}
	return p
}

func parDiscoveryConfig(cfg model.Reader) (key, endpoint, rcEndpoint string, compatible bool) {
	key = cfg.GetString("api_key")
	rcEndpoint = configutils.GetMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url")
	main := configutils.GetMainEndpoint(cfg, "https://api.", "dd_url")
	site := configutils.ExtractSiteFromURL(main)
	if site == "" {
		site = "datadoghq.com"
	}
	endpoint = "https://api." + site
	if os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS") == "true" {
		endpoint = strings.TrimSuffix(main, "/")
		compatible = parLoopbackURL(endpoint) && parLoopbackURL(rcEndpoint)
	} else {
		compatible = rcEndpoint == "https://config."+site
	}
	return
}

func parLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func (p *parScopePoller) gate(now time.Time) (OrgStatus, bool) {
	if !p.demand.Load() || os.Getenv("DD_PAR_PHONE_HOME_POC") != "true" || !p.cfg.GetBool("private_action_runner.enabled") || !configutils.IsRemoteConfigEnabled(p.cfg) {
		return OrgStatus{}, false
	}
	key, endpoint, rcEndpoint, compatible := parDiscoveryConfig(p.cfg)
	if !compatible || orgStatusBinding(key, endpoint+"\x00"+rcEndpoint) != p.binding {
		return OrgStatus{}, false
	}
	rc := p.status()
	return rc, rc.Ready(now) && rc.Matches(p.key, p.rcEndpoint)
}

func (p *parScopePoller) start() {
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

func (p *parScopePoller) stop() {
	p.demand.Store(false)
	p.demandChanged()
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
}

func (p *parScopePoller) demandChanged() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.epoch++
	p.observation = PARScopeObservation{Reason: "par_scope_unchecked", NextCheck: p.next}
	if p.cancelRequest != nil {
		p.cancelRequest()
	}
}

func (p *parScopePoller) allowed(epoch, generation uint64) bool {
	rc, ready := p.gate(p.clock.Now())
	p.mu.Lock()
	defer p.mu.Unlock()
	return ready && p.epoch == epoch && rc.Generation == generation
}

// Dispatch is checked again at RoundTrip, not only before the async request is
// built. The normal Agent transport still owns proxy/TLS behavior.
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

func (p *parScopePoller) step(ctx context.Context) {
	now := p.clock.Now()
	rc, ready := p.gate(now)
	p.mu.Lock()
	if !ready || ctx.Err() != nil {
		p.observation = PARScopeObservation{Reason: "par_scope_waiting_for_rc_or_demand", NextCheck: p.next}
		p.mu.Unlock()
		return
	}
	if p.observation.RCGeneration != rc.Generation || !now.Before(p.observation.ValidUntil) {
		p.observation.Reason = "par_scope_unchecked"
	}
	if now.Before(p.next) || p.cancelRequest != nil {
		p.mu.Unlock()
		return
	}
	epoch := p.epoch
	requestCtx, cancel := context.WithCancel(ctx)
	p.cancelRequest = cancel
	// Reserve a cadence before dispatch; a canceled or superseded response cannot
	// turn a closed/open RC transition into an immediate retry burst.
	p.next = now.Add(max(pollInterval, p.backoff))
	p.observation.NextCheck = p.next
	p.mu.Unlock()

	allowed := func() bool { return p.allowed(epoch, rc.Generation) }
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := p.clock.Ticker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-requestCtx.Done():
				return
			case <-ticker.C:
				if !allowed() {
					cancel()
					return
				}
			}
		}
	}()
	client := *p.client
	client.Transport = parGatedTransport{base: p.client.Transport, allowed: allowed}
	result := validate(requestCtx, &client, p.endpoint, p.key, p.clock.Now())
	canceled := requestCtx.Err() != nil
	valid := allowed() && ctx.Err() == nil && !canceled
	cancel()
	<-watchDone
	finished := p.clock.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelRequest = nil
	// A server's timing hint remains a lower bound even if its grant is obsolete.
	if result.transient && !canceled {
		p.backoff = min(slowRetry, max(pollInterval, 2*p.backoff))
		result.delay = p.backoff
	} else if valid {
		p.backoff = 0
	}
	if result.delay > 0 {
		delay := result.delay
		extra := p.jitter(delay / 5)
		// Do not overflow an already saturated, untrusted Retry-After duration.
		if extra <= time.Duration(1<<63-1)-delay {
			delay += extra
		}
		p.next = maxTime(p.next, finished.Add(delay))
	}
	if valid && p.epoch == epoch {
		p.observation = PARScopeObservation{Reason: result.reason, CheckedAt: now, ValidUntil: now.Add(2 * pollInterval), RCGeneration: rc.Generation, binding: orgStatusBinding(p.key, p.endpoint)}
	} else {
		p.observation = PARScopeObservation{Reason: "par_scope_unchecked"}
	}
	p.observation.NextCheck = p.next
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
