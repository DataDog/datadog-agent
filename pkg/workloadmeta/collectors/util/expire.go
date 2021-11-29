// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Expire implements a simple last-seen-time-based expiry logic for watching
// for disappearing entities (for triggering events or just cache
// housekeeping). User classes define an expiry delay, then call Update every
// time they encounter a given entity. ComputeExpires() returns the entity
// names that have not been seen for longer than the configured delay.
type Expire struct {
	mu         sync.Mutex
	duration   time.Duration
	lastSeen   map[workloadmeta.EntityID]time.Time
	lastExpire time.Time
}

// NewExpire creates a new Expire object.
func NewExpire(duration time.Duration) *Expire {
	return &Expire{
		duration:   duration,
		lastSeen:   make(map[workloadmeta.EntityID]time.Time),
		lastExpire: time.Now(),
	}
}

// Update marks the time an ID was last seen. Returns true if the ID is new,
// false if it was already present in the store.
func (e *Expire) Update(id workloadmeta.EntityID, ts time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, found := e.lastSeen[id]
	e.lastSeen[id] = ts

	return !found
}

// Remove forgets an ID.
func (e *Expire) Remove(id workloadmeta.EntityID) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.lastSeen, id)
}

// ComputeExpires returns a list of IDs that have not been seen in the Expire's
// duration.
func (e *Expire) ComputeExpires() []workloadmeta.EntityID {
	var expired []workloadmeta.EntityID

	now := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()

	if now.Sub(e.lastExpire) < e.duration {
		return nil
	}

	for id, seen := range e.lastSeen {
		if now.Sub(seen) < e.duration {
			continue
		}

		expired = append(expired, id)

		delete(e.lastSeen, id)
	}

	e.lastExpire = now

	return expired
}
