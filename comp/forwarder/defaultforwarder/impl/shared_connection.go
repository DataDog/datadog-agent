// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// SharedConnection holds a shared http.Client that is used by each worker.
// Access to the client is protected by an RWMutex.
type SharedConnection struct {
	client    *http.Client
	lock      *sync.RWMutex
	log       log.Component
	isLocal   bool
	config    config.Component
	transport http.RoundTripper
	semaphore *resizableSemaphore
}

// NewSharedConnection creates a new shared connection. The concurrency limit
// is read from forwarder_max_concurrent_requests.
func NewSharedConnection(
	log log.Component,
	isLocal bool,
	config config.Component,
	transport http.RoundTripper,
) *SharedConnection {
	sc := &SharedConnection{
		lock:      &sync.RWMutex{},
		log:       log,
		isLocal:   isLocal,
		config:    config,
		transport: transport,
		semaphore: newResizableSemaphore(maxConcurrentRequests(config, log)),
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
