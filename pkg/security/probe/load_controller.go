// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/hashicorp/golang-lru/simplelru"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventCounterLRUKey struct {
	Pid   uint32
	Event EventType
}

// LoadController is used to monitor and control the pressure put on the host
type LoadController struct {
	sync.RWMutex
	probe        *Probe
	total        int64
	counters     *simplelru.LRU
	statsdClient *statsd.Client

	EventsCountThreshold int64
	DiscarderTimeout     time.Duration
	ControllerPeriod     time.Duration
}

// NewLoadController instantiates a new load controller
func NewLoadController(probe *Probe, statsdClient *statsd.Client) (*LoadController, error) {
	lru, err := simplelru.NewLRU(probe.config.PIDCacheSize, nil)
	if err != nil {
		return nil, err
	}

	lc := &LoadController{
		probe:                probe,
		counters:             lru,
		statsdClient:         statsdClient,
		EventsCountThreshold: probe.config.LoadControllerEventsCountThreshold,
		DiscarderTimeout:     probe.config.LoadControllerDiscarderTimeout,
		ControllerPeriod:     probe.config.LoadControllerControlPeriod,
	}
	return lc, nil
}

// Count increments the event counter of the provided event type and pid
func (lc *LoadController) Count(eventType EventType, pid uint32) {
	lc.Lock()
	defer lc.Unlock()

	entry, ok := lc.counters.Get(eventCounterLRUKey{Pid: pid, Event: eventType})
	if ok {
		counter := entry.(*uint64)
		atomic.AddUint64(counter, 1)
	} else {
		count := uint64(1)
		lc.counters.Add(eventCounterLRUKey{Pid: pid, Event: eventType}, &count)
	}
	newTotal := atomic.AddInt64(&lc.total, 1)

	if newTotal >= lc.EventsCountThreshold {
		lc.discardNoisiestProcess()
	}
}

// discardNoisiestProcess determines the noisiest process and event_type tuple and pushes a temporary discarder
func (lc *LoadController) discardNoisiestProcess() {
	// iterate over the LRU map to retrieve the noisiest process & event_type tuple
	var maxKey eventCounterLRUKey
	var maxCount *uint64
	for _, key := range lc.counters.Keys() {
		entry, ok := lc.counters.Peek(key)
		if !ok || entry == nil {
			continue
		}
		tmpCount := entry.(*uint64)
		tmpKey := key.(eventCounterLRUKey)

		// update max if necessary
		if maxCount == nil || *maxCount < *tmpCount {
			maxCount = tmpCount
			maxKey = tmpKey
		}
	}
	if maxCount == nil {
		// LRU is empty nothing to discard
		return
	}

	// push a temporary discarder on the noisiest process & event type tuple
	log.Tracef("discarding %s events from pid %d for %s seconds", maxKey.Event, maxKey.Pid, lc.DiscarderTimeout)
	if _, err := discardPIDWithTimeout(lc.probe, maxKey.Event, maxKey.Pid, lc.DiscarderTimeout); err != nil {
		log.Warnf("couldn't insert temporary discarder: %v", err)
		return
	}

	// update current total and remove biggest entry from cache
	atomic.AddInt64(&lc.total, -int64(atomic.SwapUint64(maxCount, 0)))

	if lc.statsdClient != nil {
		// send load_controller.pids_discarder metric
		tags := []string{
			fmt.Sprintf("event_type:%s", maxKey.Event),
		}
		if err := lc.statsdClient.Count(MetricPrefix+".load_controller.pids_discarder", 1, tags, 1.0); err != nil {
			log.Warnf("couldn't send load_controller.pids_discarder metric: %v", err)
			return
		}
	}
}

// cleanup resets the internal counters
func (lc *LoadController) cleanup() {
	lc.RLock()
	defer lc.RUnlock()

	// reset counts
	for _, key := range lc.counters.Keys() {
		val, ok := lc.counters.Peek(key)
		if !ok || val == nil {
			continue
		}
		counter := val.(*uint64)
		atomic.SwapUint64(counter, 0)
	}
	atomic.SwapInt64(&lc.total, 0)
}

// Start resets the internal counters periodically
func (lc *LoadController) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(lc.ControllerPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lc.cleanup()
		case <-ctx.Done():
			return
		}
	}
}
