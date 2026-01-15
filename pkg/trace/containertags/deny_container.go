// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containertagsbuffer contains the logic to buffer payloads for container tags
// enrichment
package containertagsbuffer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// denyEviction is how long a container is kept without being seen
	denyEviction = 2 * time.Minute

	// denyRefresh is the interval at which a container's last seen time is refreshed
	denyRefresh = 10 * time.Second

	metricDenyListSize = "datadog.trace_agent.tag_buffer.denylist.size"
	metricExpires      = "datadog.trace_agent.tag_buffer.denylist.expires"
)

// deniedContainers tracks which containerIDs should not wait for container tags as we have already waited for them
type deniedContainers struct {
	mu           sync.RWMutex
	containers   map[string]time.Time
	lastEviction time.Time

	denied          atomic.Int64
	expired         atomic.Int64
	totalInDenyList atomic.Int64
}

func newDeniedContainers() *deniedContainers {
	return &deniedContainers{
		containers: make(map[string]time.Time),
	}
}

// shouldDeny returns if we should skip waiting for containers tags and updates the lastSeen time of containerID
func (d *deniedContainers) shouldDeny(now time.Time, containerID string) bool {
	d.mu.RLock()
	lastSeen, ok := d.containers[containerID]
	d.mu.RUnlock()
	if !ok {
		return false
	}
	if now.Sub(lastSeen) > denyRefresh {
		d.mu.Lock()
		d.containers[containerID] = now
		d.mu.Unlock()
	}
	d.denied.Add(1)
	return true
}

// deny adds containerID to the cache, rotating the cache if the last eviction is older than denyEviction
func (d *deniedContainers) deny(now time.Time, containerID string) {
	d.mu.Lock()
	d.containers[containerID] = now
	lastEviction := d.lastEviction
	d.mu.Unlock()
	if now.Sub(lastEviction) > denyEviction {
		d.rotate(now)
	}
}

func (d *deniedContainers) rotate(now time.Time) {
	newContainers, expired := d.populateNewContainers(now)
	d.mu.Lock()
	d.containers = newContainers
	d.lastEviction = now
	totalContainers := len(newContainers)
	d.mu.Unlock()

	if expired > 0 {
		d.expired.Add(int64(expired))
	}
	d.totalInDenyList.Store(int64(totalContainers))
}

func (d *deniedContainers) populateNewContainers(now time.Time) (map[string]time.Time, int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var expired int
	newContainers := make(map[string]time.Time, 100) // default to reasonable 100 size for efficiency
	for containerID, lastSeen := range d.containers {
		if now.Sub(lastSeen) > denyEviction {
			expired++
			continue
		}
		newContainers[containerID] = lastSeen
	}
	return newContainers, expired
}

func (d *deniedContainers) report(client statsd.ClientInterface) {
	if denied := d.denied.Swap(0); denied > 0 {
		_ = client.Count(metricDenied, denied, []string{"reason:denylist"}, 1)
	}
	if expired := d.expired.Swap(0); expired > 0 {
		_ = client.Count(metricDenied, expired, []string{"reason:expired"}, 1)
	}
	if listSize := d.totalInDenyList.Load(); listSize > 0 {
		_ = client.Gauge(metricDenyListSize, float64(listSize), nil, 1)
	}
}
