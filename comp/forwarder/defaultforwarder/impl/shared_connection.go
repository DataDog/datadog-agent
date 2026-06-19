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
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// SharedConnection holds a shared http.Client that is used by each worker.
// Access to the client is protected by an RWMutex.
//
// It also owns the per-domain concurrency throttle: a resizableSemaphore that
// every worker acquires before sending, and a concurrencyController that scales
// that semaphore's limit at runtime based on saturation and backoff signals.
type SharedConnection struct {
	client    *http.Client
	lock      *sync.RWMutex
	log       log.Component
	isLocal   bool
	domain    string
	config    config.Component
	transport http.RoundTripper

	semaphore  *resizableSemaphore
	backoffCh  chan struct{}
	controller *concurrencyController
}

// NewSharedConnection creates a new shared connection. The concurrency limit
// starts at 1 and is scaled at runtime by the controller up to
// forwarder_max_concurrent_requests.
func NewSharedConnection(
	log log.Component,
	isLocal bool,
	domain string,
	config config.Component,
	transport http.RoundTripper,
) *SharedConnection {
	semaphore := newResizableSemaphore(initialLimit)
	backoffCh := make(chan struct{}, 1)
	sc := &SharedConnection{
		lock:       &sync.RWMutex{},
		log:        log,
		isLocal:    isLocal,
		domain:     domain,
		config:     config,
		transport:  transport,
		semaphore:  semaphore,
		backoffCh:  backoffCh,
		controller: newConcurrencyController(log, semaphore, domain, maxConcurrentRequests(config, log), backoffCh),
	}

	sc.client = sc.newClient()

	return sc
}

// GetClient returns the http.Client.
func (sc *SharedConnection) GetClient() *http.Client {
	sc.lock.RLock()
	defer sc.lock.RUnlock()

	return sc.client
}

// ResetClient replaces the client with a newly created one.
func (sc *SharedConnection) ResetClient() {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	sc.client.CloseIdleConnections()
	sc.client = sc.newClient()
}

// Acquire blocks until the domain is allowed to start another concurrent
// request or ctx is done.
func (sc *SharedConnection) Acquire(ctx context.Context) error {
	return sc.semaphore.Acquire(ctx)
}

// Release returns a concurrency token taken by Acquire.
func (sc *SharedConnection) Release() {
	sc.semaphore.Release()
}

// StartScaling starts the controller that dynamically scales the concurrency
// limit. It resets the limit to its initial value.
func (sc *SharedConnection) StartScaling() {
	sc.controller.start()
}

// StopScaling stops the dynamic scaling controller.
func (sc *SharedConnection) StopScaling() {
	sc.controller.stopController()
}

// signalBackoff notifies the controller that the domain pushed back (HTTP
// 408/429 or a timeout). The send is non-blocking onto a size-1 channel, so a
// burst of backoffs coalesces into a single pending wake-up.
func (sc *SharedConnection) signalBackoff() {
	select {
	case sc.backoffCh <- struct{}{}:
	default:
	}
}

func (sc *SharedConnection) newClient() *http.Client {
	var c *http.Client
	if sc.isLocal {
		c = newBearerAuthHTTPClient()
	} else {
		// 0 means unlimited connections per host: the concurrency limit is
		// enforced by the shared semaphore, not the transport.
		c = NewHTTPClient(sc.config, 0, sc.log)
	}
	if sc.transport != nil {
		c.Transport = sc.transport
	}
	// Wrap the transport so the controller observes backoff signals. Re-applied
	// here so the signal survives ResetClient.
	c.Transport = &backoffSignalTransport{base: c.Transport, onBackoff: sc.signalBackoff}
	return c
}

func maxConcurrentRequests(config config.Component, log log.Component) int64 {
	maxConcurrentRequests := config.GetInt64("forwarder_max_concurrent_requests")
	if maxConcurrentRequests <= 0 {
		log.Warnf("Invalid forwarder_max_concurrent_requests '%d', setting to 1", maxConcurrentRequests)
		maxConcurrentRequests = 1
	}
	return maxConcurrentRequests
}

// backoffSignalTransport wraps a RoundTripper and reports the backoff signals
// the concurrency controller scales down on: HTTP 408, HTTP 429, HTTP 503, and
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
