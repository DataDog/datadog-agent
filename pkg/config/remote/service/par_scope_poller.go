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
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/benbjohnson/clock"
)

// WithPARScopeDiscovery attaches the opt-in worker to core's RC service only.
func WithPARScopeDiscovery() func(*options) { return func(o *options) { o.parScopeDiscovery = true } }

type parScopePoller struct {
	cfg                       model.Reader
	clock                     clock.Clock
	status                    func() *pbgo.OrgStatusResponse
	demand                    *atomic.Bool
	client                    *http.Client
	key, endpoint, rcEndpoint string
	jitter                    func(time.Duration) time.Duration
	mu                        sync.Mutex
	observation               PARScopeObservation
	next                      time.Time
	backoff                   time.Duration
	cancelRequest             context.CancelFunc
	cancel                    context.CancelFunc
	done                      chan struct{}
}

func newPARScopePoller(cfg model.Reader, clk clock.Clock, status func() *pbgo.OrgStatusResponse, demand *atomic.Bool) *parScopePoller {
	if os.Getenv("DD_PAR_PHONE_HOME_POC") != "true" || !cfg.GetBool("private_action_runner.enabled") {
		return nil
	}
	key, endpoint, rcEndpoint, compatible := parDiscoveryConfig(cfg)
	if !compatible {
		return nil
	}
	p := &parScopePoller{cfg: cfg, clock: clk, status: status, demand: demand, key: key, endpoint: endpoint, rcEndpoint: rcEndpoint,
		jitter: func(d time.Duration) time.Duration { return time.Duration(rand.Int64N(int64(d) + 1)) },
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
	rcKey := cfg.GetString("remote_configuration.api_key")
	compatible = compatible && key != "" && (rcKey == "" || rcKey == key) && cfg.GetString("remote_configuration.key") == ""
	return
}

func parLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" &&
		(u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func (p *parScopePoller) gate() bool {
	if !p.demand.Load() || os.Getenv("DD_PAR_PHONE_HOME_POC") != "true" || !p.cfg.GetBool("private_action_runner.enabled") || !configutils.IsRemoteConfigEnabled(p.cfg) {
		return false
	}
	key, endpoint, rcEndpoint, compatible := parDiscoveryConfig(p.cfg)
	if !compatible || key != p.key || endpoint != p.endpoint || rcEndpoint != p.rcEndpoint {
		return false
	}
	rc := p.status()
	return rc != nil && rc.Enabled && rc.Authorized
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

func (p *parScopePoller) clearLocked() {
	p.observation = PARScopeObservation{Reason: "par_scope_unchecked", NextCheck: p.next}
	if p.cancelRequest != nil {
		p.cancelRequest()
	}
}

func (p *parScopePoller) demandChanged() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearLocked()
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

func (p *parScopePoller) step(ctx context.Context) {
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
	go func() {
		defer close(watchDone)
		ticker := p.clock.Ticker(time.Second)
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
