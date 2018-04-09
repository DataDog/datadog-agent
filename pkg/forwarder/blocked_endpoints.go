// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"sync"
	"time"
)

// Whether or not our config has been loaded by the agent yet.
var configLoaded = false

// This is the number of errors it will take to reach the maxBackoffTime. Our
// blockedEndpoints circuit breaker uses this value as the maximum number of errors.
var maxErrors int

type block struct {
	nbError int
	until   time.Time
}

type blockedEndpoints struct {
	errorPerEndpoint map[string]*block
	m                sync.RWMutex
}

func newBlockedEndpoints() *blockedEndpoints {
	// All forwarder settings are used directly or indirectly (GetBackoffDuration) by
	// this circuit breaker singleton. Therefore, it makes sense to load them here.
	if !configLoaded {
		loadConfig()
		configLoaded = true
	}

	return &blockedEndpoints{errorPerEndpoint: make(map[string]*block)}
}

func (e *blockedEndpoints) close(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError++
	if b.nbError > maxErrors {
		b.nbError = maxErrors
	}
	b.until = time.Now().Add(GetBackoffDuration(b.nbError))

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) recover(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError -= recoveryInterval
	if b.nbError < 0 {
		b.nbError = 0
	}
	b.until = time.Now().Add(GetBackoffDuration(b.nbError))

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) isBlock(endpoint string) bool {
	e.m.RLock()
	defer e.m.RUnlock()

	if b, ok := e.errorPerEndpoint[endpoint]; ok && time.Now().Before(b.until) {
		return true
	}
	return false
}
