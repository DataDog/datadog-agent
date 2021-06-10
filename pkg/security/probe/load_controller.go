// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/hashicorp/golang-lru/simplelru"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventCounterLRUKey struct {
	Pid    uint32
	Cookie uint32
}

// LoadController is used to monitor and control the pressure put on the host
type LoadController struct {
	sync.RWMutex
	probe        *Probe
	statsdClient *statsd.Client

	eventsTotal    int64
	eventsCounters *simplelru.LRU

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
		probe:        probe,
		statsdClient: statsdClient,

		eventsCounters: lru,

		EventsCountThreshold: probe.config.LoadControllerEventsCountThreshold,
		DiscarderTimeout:     probe.config.LoadControllerDiscarderTimeout,
		ControllerPeriod:     probe.config.LoadControllerControlPeriod,
	}
	return lc, nil
}

// Count processes the provided events and ensures the load of the provided event type is within the configured limits
func (lc *LoadController) Count(event *Event) {
	switch event.GetEventType() {
	case model.ExecEventType, model.InvalidateDentryEventType, model.ForkEventType:
	case model.ExitEventType:
		lc.cleanupCounter(event.ProcessContext.Pid, event.ProcessContext.Cookie)
	default:
		lc.GenericCount(event)
	}
}

// GenericCount increments the event counter of the provided event type and pid
func (lc *LoadController) GenericCount(event *Event) {
	lc.Lock()
	defer lc.Unlock()

	entry, ok := lc.eventsCounters.Get(eventCounterLRUKey{Pid: event.ProcessContext.Pid, Cookie: event.ProcessContext.Cookie})
	if ok {
		counter := entry.(*uint64)
		atomic.AddUint64(counter, 1)
	} else {
		counter := uint64(1)
		lc.eventsCounters.Add(eventCounterLRUKey{Pid: event.ProcessContext.Pid, Cookie: event.ProcessContext.Cookie}, &counter)
	}
	newTotal := atomic.AddInt64(&lc.eventsTotal, 1)

	if newTotal >= lc.EventsCountThreshold {
		lc.discardNoisiestProcess()
	}
}

// discardNoisiestProcess determines the noisiest process and event_type tuple and pushes a temporary discarder
func (lc *LoadController) discardNoisiestProcess() {
	// iterate over the LRU map to retrieve the noisiest process & event_type tuple
	var maxKey eventCounterLRUKey
	var maxCount *uint64
	for _, key := range lc.eventsCounters.Keys() {
		entry, ok := lc.eventsCounters.Peek(key)
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
	log.Tracef("discarding events from pid %d for %s seconds", maxKey.Pid, lc.DiscarderTimeout)
	if err := lc.probe.pidDiscarders.discardWithTimeout(0xffffffffffffffff, maxKey.Pid, lc.DiscarderTimeout.Nanoseconds()); err != nil {
		log.Warnf("couldn't insert temporary discarder: %v", err)
		return
	}

	// update current total and remove biggest entry from cache
	oldMaxCount := atomic.SwapUint64(maxCount, 0)
	atomic.AddInt64(&lc.eventsTotal, -int64(oldMaxCount))

	if lc.statsdClient != nil {
		// send load_controller.pids_discarder metric
		if err := lc.statsdClient.Count(metrics.MetricLoadControllerPidDiscarder, 1, []string{}, 1.0); err != nil {
			log.Warnf("couldn't send load_controller.pids_discarder metric: %v", err)
			return
		}

		// fetch noisy process metadata
		process := lc.probe.resolvers.ProcessResolver.Resolve(maxKey.Pid, maxKey.Pid)
		if process == nil {
			log.Warnf("Unable to resolve process with pid: %d", maxKey.Pid)
			return
		}

		ts := time.Now()
		lc.probe.DispatchCustomEvent(
			NewNoisyProcessEvent(
				oldMaxCount,
				lc.EventsCountThreshold,
				lc.ControllerPeriod,
				ts.Add(lc.DiscarderTimeout),
				process,
				lc.probe.GetResolvers(),
				ts,
			),
		)
	}
}

// cleanupCounter resets the internal counter of the provided pid
func (lc *LoadController) cleanupCounter(pid uint32, cookie uint32) {
	lc.Lock()
	defer lc.Unlock()

	key := eventCounterLRUKey{Pid: pid, Cookie: cookie}
	entry, ok := lc.eventsCounters.Get(key)
	if ok {
		counter := int64(*entry.(*uint64))
		atomic.AddInt64(&lc.eventsTotal, -counter)
		lc.eventsCounters.Remove(key)
	}
}

// cleanup resets the internal counters
func (lc *LoadController) cleanup() {
	lc.Lock()
	defer lc.Unlock()

	// purge counters
	lc.eventsCounters.Purge()
	atomic.SwapInt64(&lc.eventsTotal, 0)
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
