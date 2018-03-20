// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"math"
	"sync"
	"time"
)

const (
	// This controls how many retry interval ranges to step down for an endpoint
	// upon success. Increasing this should only be considered when maxBackoffTime
	// is particularly high or if our intake team is particularly confident.
	recoveryInterval = 1
)

// This is the number of errors it will take to reach the maxBackoffTime. Our
// blockedEndpoints circuit breaker uses this value as the maximum number of errors.
var maxErrors = int(math.Floor(math.Log2(float64(maxBackoffTime)/float64(baseBackoffTime)))) + 1

type block struct {
	nbError int
	until   time.Time
}

type blockedEndpoints struct {
	errorPerEndpoint map[string]*block
	m                sync.RWMutex
}

func newBlockedEndpoints() *blockedEndpoints {
	return &blockedEndpoints{errorPerEndpoint: make(map[string]*block)}
}

func (e *blockedEndpoints) block(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError = int(math.Min(float64(maxErrors), float64(b.nbError+1)))
	b.until = time.Now().Add(GetBackoffDuration(b.nbError))

	e.errorPerEndpoint[endpoint] = b
}

func (e *blockedEndpoints) unblock(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpoint]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}

	b.nbError = int(math.Max(0, float64(b.nbError-recoveryInterval)))
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
