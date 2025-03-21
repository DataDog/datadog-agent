// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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
}

// NewSharedConnection creates a new shared connection with the given
// http.Client.
func NewSharedConnection(
	log log.Component,
	isLocal bool,
	numberOfWorkers int,
	config config.Component,
) *SharedConnection {
	sc := &SharedConnection{
		lock:            &sync.RWMutex{},
		log:             log,
		isLocal:         isLocal,
		numberOfWorkers: numberOfWorkers,
		config:          config,
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

func (sc *SharedConnection) newClient() *http.Client {
	if sc.isLocal {
		return newBearerAuthHTTPClient(sc.numberOfWorkers)
	}

	return NewHTTPClient(sc.config, sc.numberOfWorkers, sc.log)
}
