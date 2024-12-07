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

type Throttler interface {
	// RequestToken returns a token
	RequestToken() token
	// ReleaseToken returns token back to the throttler
	Release(t token)
}

type limiter struct {
	mutex          sync.RWMutex
	tokensChan     chan struct{}
	activeRequests map[token]struct{}
}

func NewSyncThrottler(maxConcurrentSync uint32) Throttler {
	return limiter{
		mutex:          sync.RWMutex{},
		tokensChan:     make(chan struct{}, maxConcurrentSync),
		activeRequests: make(map[token]struct{}),
	}
}

func (l limiter) RequestToken() token {
	tk := token(uuid.New().String())
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.tokensChan <- struct{}{}
	l.activeRequests[tk] = struct{}{}
	return tk
}

func (l limiter) Release(t token) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if _, found := l.activeRequests[t]; found {
		<-l.tokensChan
		delete(l.activeRequests, t)
	}
}
