// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package utils

import (
	"errors"
	"sync"
	"time"
)

// Expire implements a simple last-seen-time-based expiry logic for watching for disappearing entities (for triggering events or just cache housekeeping).
// User classes define an expiry delay, then call Update every time they encounter a given entity.
// ComputeExpires() returns the entity names that have not been seen for longer than the configured delay.
//As Expire keeps an internal state of entity names, Update will return true if a name is new, false otherwise.
type Expire struct {
	sync.Mutex
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
}

// NewExpire creates a new Expire object. Called when a Collector is started.
func NewExpire(expiryDuration time.Duration) (*Expire, error) {
	if expiryDuration.Seconds() <= 0.0 {
		return nil, errors.New("expiryDuration must be above 0")
	}
	return &Expire{
		expiryDuration: expiryDuration,
		lastSeen:       make(map[string]time.Time),
	}, nil
}

// Update the map of the Expire obect.
func (e *Expire) Update(container string, ts time.Time) bool {
	e.Lock()
	_, found := e.lastSeen[container]
	e.lastSeen[container] = ts
	e.Unlock()
	return !found
}

// ComputeExpires should be called right after an Update.
func (e *Expire) ComputeExpires() ([]string, error) {
	now := time.Now()
	var expiredContainers []string
	e.Lock()
	for id, seen := range e.lastSeen {
		if now.Sub(seen) > e.expiryDuration {
			expiredContainers = append(expiredContainers, id)
			delete(e.lastSeen, id)
		}
	}
	e.Unlock()
	return expiredContainers, nil
}
