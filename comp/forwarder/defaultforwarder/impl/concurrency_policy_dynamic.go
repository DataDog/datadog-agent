// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// dynamicConcurrencySettings holds the forwarder.dynamic_concurrency.* settings.
type dynamicConcurrencySettings struct {
	workers int
	// maxLimit is the upper bound of the concurrency limit.
	maxLimit int64
	// initialLimit is the concurrency the policy starts at.
	initialLimit int64
	// evalInterval is how often the policy evaluates whether to grow the
	// limit. It is sized to the payload arrival cadence so that a single batch
	// arriving between ticks does not look like sustained backlog.
	evalInterval time.Duration
	// increaseThreshold is the fraction of an evaluation window the semaphore
	// must spend fully saturated before the limit grows.
	increaseThreshold float64
	// decreaseThreshold is the saturation below which the limit decays by one per
	// evaluation window, so that capacity grown for a past backlog is released
	// once it is no longer needed. Zero disables decay.
	decreaseThreshold float64
	// increaseStep is the additive growth applied per evaluation window.
	increaseStep int64
	// minDecreaseInterval debounces decreases: a burst of backoff signals (e.g.
	// every transaction in a batch getting a 429) collapses into a single
	// halving, while sustained throttling keeps stepping the limit down.
	minDecreaseInterval time.Duration
	// increaseCooldown suppresses growth for this long after a decrease, so the
	// limit does not immediately climb back into the throttle it just yielded to.
	increaseCooldown time.Duration
}

func dynamicConcurrencySettingsFromConfig(config config.Component, log log.Component) dynamicConcurrencySettings {
	const prefix = "forwarder.dynamic_concurrency."

	s := dynamicConcurrencySettings{
		workers:             config.GetInt(prefix + "num_workers"),
		maxLimit:            maxConcurrentRequestsFromConfig(config, log, prefix+"max_concurrent_requests"),
		initialLimit:        config.GetInt64(prefix + "initial_limit"),
		evalInterval:        config.GetDuration(prefix + "eval_interval"),
		increaseThreshold:   config.GetFloat64(prefix + "increase_threshold"),
		decreaseThreshold:   config.GetFloat64(prefix + "decrease_threshold"),
		increaseStep:        config.GetInt64(prefix + "increase_step"),
		minDecreaseInterval: config.GetDuration(prefix + "min_decrease_interval"),
		increaseCooldown:    config.GetDuration(prefix + "increase_cooldown"),
	}

	if s.initialLimit < 1 || s.initialLimit > s.maxLimit {
		clamped := min(max(s.initialLimit, 1), s.maxLimit)
		log.Warnf("Invalid %sinitial_limit '%d', setting to %d", prefix, s.initialLimit, clamped)
		s.initialLimit = clamped
	}
	if s.evalInterval <= 0 {
		log.Warnf("Invalid %seval_interval '%s', setting to 1s", prefix, s.evalInterval)
		s.evalInterval = time.Second
	}
	if s.increaseThreshold < 0 || s.increaseThreshold > 1 {
		clamped := min(max(s.increaseThreshold, 0), 1)
		log.Warnf("Invalid %sincrease_threshold '%v', setting to %v", prefix, s.increaseThreshold, clamped)
		s.increaseThreshold = clamped
	}
	if s.decreaseThreshold < 0 || s.decreaseThreshold > s.increaseThreshold {
		clamped := min(max(s.decreaseThreshold, 0), s.increaseThreshold)
		log.Warnf("Invalid %sdecrease_threshold '%v', setting to %v", prefix, s.decreaseThreshold, clamped)
		s.decreaseThreshold = clamped
	}
	if s.increaseStep < 1 {
		log.Warnf("Invalid %sincrease_step '%d', setting to 1", prefix, s.increaseStep)
		s.increaseStep = 1
	}
	if s.minDecreaseInterval < 0 {
		log.Warnf("Invalid %smin_decrease_interval '%s', setting to 0s", prefix, s.minDecreaseInterval)
		s.minDecreaseInterval = 0
	}
	if s.increaseCooldown < 0 {
		log.Warnf("Invalid %sincrease_cooldown '%s', setting to 0s", prefix, s.increaseCooldown)
		s.increaseCooldown = 0
	}

	return s
}

type dynamicConcurrencyPolicy struct {
	log      log.Component
	settings dynamicConcurrencySettings
}

func (p dynamicConcurrencyPolicy) numWorkers() int {
	return p.settings.workers
}

// maxConns returns 0, which means unlimited connections per host: the limit is
// enforced by the shared semaphore, not the transport.
func (p dynamicConcurrencyPolicy) maxConns() int {
	return 0
}

func (p dynamicConcurrencyPolicy) newDomain(domain string) concurrencyDomain {
	return &dynamicConcurrencyDomain{
		dynamicConcurrencyPolicy: p,
		domain:                   domain,
		sem:                      newResizableSemaphore(p.settings.initialLimit),
		backoff:                  make(chan struct{}, 1),
		now:                      time.Now,
		limit:                    p.settings.initialLimit,
	}
}

// dynamicConcurrencyDomain shares one resizableSemaphore between all workers
// of a domain and scales its limit at runtime using AIMD: additive increase
// when the semaphore is sustainedly saturated (we cannot keep up with the
// arrival rate), and an immediate multiplicative decrease when a backoff
// signal (HTTP 408/429/503 or a timeout) is observed.
//
// All limit mutations happen in the run goroutine, so the current limit needs
// no additional locking.
type dynamicConcurrencyDomain struct {
	dynamicConcurrencyPolicy

	domain  string
	sem     *resizableSemaphore
	backoff chan struct{}
	now     func() time.Time

	stopCh chan struct{}
	done   chan struct{}

	limit        int64
	lastEval     time.Time
	lastDecrease time.Time
}

func (d *dynamicConcurrencyDomain) newRequestSemaphore() weightedSemaphore {
	return d.sem
}

func (d *dynamicConcurrencyDomain) wrapTransport(rt http.RoundTripper) http.RoundTripper {
	return &backoffSignalTransport{base: rt, onBackoff: d.signalBackoff}
}

// start launches the control loop.
func (d *dynamicConcurrencyDomain) start() {
	d.stopCh = make(chan struct{})
	d.done = make(chan struct{})
	d.lastEval = d.now()
	tlmConcurrencyLimit.Set(float64(d.limit), d.domain)
	tlmConcurrencySaturation.Set(0, d.domain)

	go d.run()
}

// stop stops the control loop and waits for it to exit.
func (d *dynamicConcurrencyDomain) stop() {
	close(d.stopCh)
	<-d.done
}

func (d *dynamicConcurrencyDomain) run() {
	defer close(d.done)

	ticker := time.NewTicker(d.settings.evalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-d.backoff:
			d.onBackoff()
		case <-ticker.C:
			d.onEval()
		}
	}
}

// signalBackoff notifies the control loop that the domain pushed back. The
// send is non-blocking onto a size-1 channel, so a burst of backoffs coalesces
// into a single pending wake-up.
func (d *dynamicConcurrencyDomain) signalBackoff() {
	select {
	case d.backoff <- struct{}{}:
	default:
	}
}

// onBackoff reacts to a backoff signal by halving the limit immediately,
// debounced so a burst collapses into one step.
func (d *dynamicConcurrencyDomain) onBackoff() {
	now := d.now()
	if !d.lastDecrease.IsZero() && now.Sub(d.lastDecrease) < d.settings.minDecreaseInterval {
		return
	}
	d.lastDecrease = now
	d.setLimit(max(1, d.limit/2))
}

// onEval grows the limit when the semaphore was saturated for most of the
// window and decays it when the semaphore was mostly idle, unless we recently
// backed off.
func (d *dynamicConcurrencyDomain) onEval() {
	now := d.now()
	window := now.Sub(d.lastEval)
	d.lastEval = now

	saturated := d.sem.takeSaturatedDuration()
	if window <= 0 {
		return
	}
	saturation := float64(saturated) / float64(window)
	tlmConcurrencySaturation.Set(saturation, d.domain)
	if !d.lastDecrease.IsZero() && now.Sub(d.lastDecrease) < d.settings.increaseCooldown {
		return
	}
	if saturation >= d.settings.increaseThreshold {
		d.setLimit(min(d.settings.maxLimit, d.limit+d.settings.increaseStep))
	} else if saturation < d.settings.decreaseThreshold {
		d.setLimit(max(1, d.limit-1))
	}
}

func (d *dynamicConcurrencyDomain) setLimit(limit int64) {
	if limit == d.limit {
		return
	}
	d.limit = limit
	d.sem.SetLimit(limit)
	tlmConcurrencyLimit.Set(float64(limit), d.domain)
	d.log.Debugf("Forwarder concurrency limit for domain %q set to %d", d.domain, limit)
}

// backoffSignalTransport wraps a RoundTripper and reports the backoff signals
// the dynamic policy scales down on: HTTP 408, HTTP 429, HTTP 503, and
// timeouts.
type backoffSignalTransport struct {
	base      http.RoundTripper
	onBackoff func()
}

func (t *backoffSignalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		if isTimeout(err) {
			t.onBackoff()
		}
		return resp, err
	}
	if resp.StatusCode == http.StatusRequestTimeout ||
		resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusServiceUnavailable {
		t.onBackoff()
	}
	return resp, err
}

// isTimeout reports whether err is a timeout we should back off on. It excludes
// context.Canceled, which is how worker shutdown cancels in-flight requests.
func isTimeout(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
