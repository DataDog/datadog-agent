// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerCache is a thread-safe, ttl based, auto-cleaning cache for tracer infos.
type TracerCache struct {
	tracerInfos map[string]*entry
	maxEntries  int
	ttl         time.Duration
	mutex       sync.Mutex
	closed      chan struct{}
	interval    time.Duration
}

type entry struct {
	tracerInfo *pbgo.TracerInfo
	lastSeen   time.Time
}

// NewTracerCache returns a new TracerCache.
func NewTracerCache(maxEntries int, ttl time.Duration, cleanupInterval time.Duration) *TracerCache {
	t := &TracerCache{
		tracerInfos: make(map[string]*entry),
		maxEntries:  maxEntries,
		ttl:         ttl,
		interval:    cleanupInterval,
		closed:      make(chan struct{}, 1),
	}
	go t.startLoop()
	return t
}

func (tc *TracerCache) startLoop() {
	ticker := time.NewTicker(tc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tc.mutex.Lock()
			tc.cleanup()
			tc.mutex.Unlock()
		case <-tc.closed:
			return

		}
	}
}

// cleanup cleans the tracer cache. Caller is required to handle locking.
func (tc *TracerCache) cleanup() int {
	log.Debugf("Running tracer cleanup, currently %d entries", len(tc.tracerInfos))
	now := time.Now()
	count := 0
	for runtimeID, tracer := range tc.tracerInfos {
		if now.Sub(tracer.lastSeen) >= tc.ttl {
			log.Debugf("Expiring tracer %s, last seen: %s", tracer.tracerInfo.RuntimeId, tracer.lastSeen.UTC())
			delete(tc.tracerInfos, runtimeID)
			count++
		}
	}
	return count
}

// Stop stops the cleanup loop. Not idempotent, can only be called once.
func (tc *TracerCache) Stop() {
	close(tc.closed)
}

// TrackTracer tracks a tracer. If it is already contained in the cache, the lastSeen will be updated. If it is new, it
// will be added to the cache if there is space. If the cache is full, a cleanup will be triggered. If it is still full
// after the cleanup, an error is returned.
func (tc *TracerCache) TrackTracer(tracer *pbgo.TracerInfo) error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	if _, ok := tc.tracerInfos[tracer.RuntimeId]; ok {
		log.Debugf("Refreshing last seen for tracer %s", tracer)
		tc.tracerInfos[tracer.RuntimeId] = &entry{tracer, time.Now()}
		return nil
	}

	if len(tc.tracerInfos) >= tc.maxEntries {
		if tc.cleanup() == 0 {
			return fmt.Errorf("TracerCache maxCapacity reached. Refusing to add tracer %s", tracer)
		}
	}

	log.Debugf("Adding tracer %s", tracer)
	tc.tracerInfos[tracer.RuntimeId] = &entry{tracer, time.Now()}
	return nil
}

// Tracers get all tracers currently considered alive in the cache.
func (tc *TracerCache) Tracers() []*pbgo.TracerInfo {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	tc.cleanup()
	tracers := make([]*pbgo.TracerInfo, 0, len(tc.tracerInfos))
	for _, tracer := range tc.tracerInfos {
		tracers = append(tracers, tracer.tracerInfo)
	}
	return tracers
}
