// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package utils

import (
	"errors"
	"sync"
	"time"
)

// Expire regularly pools the source keeping the elements reporting.
// Currently only used for ECSCollector and active containers.
// It keeps an internal state to only send the updated elements (only used for containers to start with).
type Expire struct {
	sync.Mutex
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
}

// NewExpire creates a new Expire object. Called when a Collector is started.
// Only used for the ECS collector to start with.
func NewExpire(expiryDuration time.Duration) (*Expire, error) {
	if expiryDuration <= 0 {
		return nil, errors.New("expiryDuration must be above 0")
	}
	return &Expire{
		expiryDuration: expiryDuration,
		lastSeen:       make(map[string]time.Time),
	}, nil
}

// Update will update the map of the Expire obect with the elements and the time they reported.
// Will return True if the element passed (container) is not found in the current lastseen map.
func (e *Expire) Update(container string, ts time.Time) bool {

	e.Lock()
	_, found := e.lastSeen[container]
	e.lastSeen[container] = ts
	e.Unlock()
	return !found
}

// ExpireContainers returns a list of container id for containers
// that are not listed in the podlist/tasklist anymore. It must be called
// immediately after a PullChanges or a Fetch.
func (e *Expire) ExpireContainers() ([]string, error) {
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
