// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"errors"
	"sync"
	"time"
)

// expire implements a simple last-seen-time-based expiry logic for watching for disappearing entities (for triggering events or just cache housekeeping).
// User classes define an expiry delay, then call Update every time they encounter a given entity.
// ComputeExpires() returns the entity names that have not been seen for longer than the configured delay.
//As expire keeps an internal state of entity names, Update will return true if a name is new, false otherwise.
type expire struct {
	sync.Mutex
	source         string
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
	lastExpire     time.Time
}

// newExpire creates a new Expire object. Called when a Collector is started.
func newExpire(source string, expiryDuration time.Duration) (*expire, error) {
	if expiryDuration.Seconds() <= 0.0 {
		return nil, errors.New("expiryDuration must be above 0")
	}

	return &expire{
		source:         source,
		expiryDuration: expiryDuration,
		lastSeen:       make(map[string]time.Time),
		lastExpire:     time.Now(),
	}, nil
}

// Update the map of the Expire obect.
func (e *expire) Update(entityID string, ts time.Time) bool {
	e.Lock()
	defer e.Unlock()

	_, found := e.lastSeen[entityID]
	e.lastSeen[entityID] = ts

	return !found
}

// ComputeExpires should be called right after an Update.
func (e *expire) ComputeExpires() []*TagInfo {
	var expiredEntities []*TagInfo
	now := time.Now()

	e.Lock()
	defer e.Unlock()

	if now.Sub(e.lastExpire) < e.expiryDuration {
		return nil
	}

	for id, seen := range e.lastSeen {
		if now.Sub(seen) < e.expiryDuration {
			continue
		}

		expiredEntities = append(expiredEntities, &TagInfo{
			Source:       e.source,
			Entity:       id,
			DeleteEntity: true,
		})

		delete(e.lastSeen, id)
	}

	e.lastExpire = now

	return expiredEntities
}
