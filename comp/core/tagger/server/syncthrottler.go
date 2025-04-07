// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"sync"

	"github.com/google/uuid"
)

type token string

// Throttler provides tokens with throttling logic that limits the number of active tokens at the same time
// When a component is done with a token, it should release the token by calling the Release method
type Throttler interface {
	// RequestToken returns a token
	RequestToken() token
	// ReleaseToken returns token back to the throttler
	// This method is idempotent (i.e. invoking it on the same token multiple times will have the same effect)
	Release(t token)
}

// limiter implements the Throttler interface
type limiter struct {
	mutex          sync.RWMutex
	tokensChan     chan struct{}
	activeRequests map[token]struct{}
}

// NewSyncThrottler creates and returns a new Throttler
func NewSyncThrottler(maxConcurrentSync uint32) Throttler {
	return &limiter{
		mutex:          sync.RWMutex{},
		tokensChan:     make(chan struct{}, maxConcurrentSync),
		activeRequests: make(map[token]struct{}),
	}
}

// RequestToken implements Throttler#RequestToken
func (l *limiter) RequestToken() token {
	tk := token(uuid.New().String())
	l.tokensChan <- struct{}{}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.activeRequests[tk] = struct{}{}
	return tk
}

// Release implements Throttler#Release
func (l *limiter) Release(t token) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if _, found := l.activeRequests[t]; found {
		<-l.tokensChan
		delete(l.activeRequests, t)
	}
}
