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

type Expire struct {
	sync.Mutex
	expiryDuration time.Duration
	lastSeen       map[string]time.Time
}

func NewExpire(expiryDuration time.Duration) (*Expire, error) {
	if expiryDuration <= 0 {
		return nil, errors.New("expiryDuration must be above 0")
	}
	return &Expire{
		expiryDuration: expiryDuration,
		lastSeen:       make(map[string]time.Time),
	}, nil
}

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
