// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"sync"
	"time"
)

const (
	blockInterval    time.Duration = 5 * time.Second
	maxBlockInterval time.Duration = 30 * time.Second
)

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

func (e *blockedEndpoints) block(endpointName string) {
	e.m.Lock()
	defer e.m.Unlock()

	var b *block
	if knownBlock, ok := e.errorPerEndpoint[endpointName]; ok {
		b = knownBlock
	} else {
		b = &block{}
	}
	b.nbError++

	newInterval := time.Duration(b.nbError) * blockInterval
	if newInterval > maxRetryInterval {
		newInterval = maxBlockInterval
	}
	b.until = time.Now().Add(newInterval)

	e.errorPerEndpoint[endpointName] = b
}

func (e *blockedEndpoints) unblock(endpoint string) {
	e.m.Lock()
	defer e.m.Unlock()

	delete(e.errorPerEndpoint, endpoint)
}

func (e *blockedEndpoints) isBlock(endpoint string) bool {
	e.m.RLock()
	defer e.m.RUnlock()

	if b, ok := e.errorPerEndpoint[endpoint]; ok && time.Now().Before(b.until) {
		return true
	}
	return false
}
