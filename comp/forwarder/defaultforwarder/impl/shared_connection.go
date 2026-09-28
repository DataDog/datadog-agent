// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// SharedConnection holds a shared http.Client that is used by each worker.
// Access to the client is protected by an RWMutex.
type SharedConnection struct {
	client          *http.Client
	lock            *sync.RWMutex
	log             log.Component
	isLocal         bool
	numberOfWorkers int
	config          config.Component
	transport       http.RoundTripper
	fallback        *model.ConfigFallback
	active          bool
}

// NewSharedConnection creates a new shared connection with the given
// http.Client.
func NewSharedConnection(
	log log.Component,
	isLocal bool,
	numberOfWorkers int,
	config config.Component,
	transport http.RoundTripper,
) *SharedConnection {
	sc := &SharedConnection{
		lock:            &sync.RWMutex{},
		log:             log,
		isLocal:         isLocal,
		numberOfWorkers: numberOfWorkers,
		config:          config,
		transport:       transport,
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
	sc.recordFallback()
}

func (sc *SharedConnection) setActive(active bool) {
	sc.lock.Lock()
	defer sc.lock.Unlock()
	sc.active = active
	sc.recordFallback()
}

// Each connection owns its observation, so stopping one cannot clear another's fallback.
func (sc *SharedConnection) recordFallback() {
	consumer := fmt.Sprintf("forwarder:%p", sc)
	if !sc.active || sc.fallback == nil {
		sc.config.ClearConfigFallback("min_tls_version", consumer)
		return
	}
	fallback := *sc.fallback
	fallback.Consumer = consumer
	sc.config.RecordConfigFallback(fallback)
}

func (sc *SharedConnection) newClient() *http.Client {
	var c *http.Client
	sc.fallback = nil
	if sc.isLocal {
		c = newBearerAuthHTTPClient(sc.numberOfWorkers)
	} else {
		transport, fallback := newHTTPTransport(sc.config, sc.numberOfWorkers, sc.log)
		sc.fallback = fallback
		c = &http.Client{Transport: transport, Timeout: sc.config.GetDuration("forwarder_timeout") * time.Second}
	}
	if sc.transport != nil {
		c.Transport = sc.transport
		sc.fallback = nil
	}
	return c
}
