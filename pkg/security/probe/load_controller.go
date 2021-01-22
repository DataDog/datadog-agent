// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventCounterLRUKey struct {
	Pid    uint32
	Cookie uint32
	Event  EventType
}

// LoadController is used to monitor and control the pressure put on the host
type LoadController struct {
	sync.RWMutex
	probe        *Probe
	statsdClient *statsd.Client

	eventsTotal    int64
	eventsCounters *simplelru.LRU
	forkCounters   *simplelru.LRU

	EventsCountThreshold int64
	ForkBombThreshold    int64
	DiscarderTimeout     time.Duration
	ControllerPeriod     time.Duration
}

// NewLoadController instantiates a new load controller
func NewLoadController(probe *Probe, statsdClient *statsd.Client) (*LoadController, error) {
	lru, err := simplelru.NewLRU(probe.config.PIDCacheSize, nil)
	if err != nil {
		return nil, err
	}

	forkLru, err := simplelru.NewLRU(probe.config.PIDCacheSize, nil)
	if err != nil {
		return nil, err
	}

	lc := &LoadController{
		probe:        probe,
		statsdClient: statsdClient,

		eventsCounters: lru,
		forkCounters:   forkLru,

		EventsCountThreshold: probe.config.LoadControllerEventsCountThreshold,
		ForkBombThreshold:    probe.config.LoadControllerForkBombThreshold,
		DiscarderTimeout:     probe.config.LoadControllerDiscarderTimeout,
		ControllerPeriod:     probe.config.LoadControllerControlPeriod,
	}
	return lc, nil
}

// Count processes the provided events and ensures the load of the provided event type is within the configured limits
func (lc *LoadController) Count(event *Event) {
	switch event.GetEventType() {
	case ExitEventType, ExecEventType, InvalidateDentryEventType:
	case ForkEventType:
		lc.CountFork(event)
	default:
		lc.GenericCount(event)
	}
}

// ResetForkCount removes the fork counter for the provided mount_id & inode. Returns true if the entry was present.
func (lc *LoadController) ResetForkCount(mountID uint32, inode uint64) bool {
	return lc.forkCounters.Remove(PathKey{MountID: mountID, Inode: inode})
}

// CountFork increments the fork counter of the provided cookie
func (lc *LoadController) CountFork(event *Event) {
	lc.Lock()
	defer lc.Unlock()
	var forkBomb bool
	var newCount uint64

	key := PathKey{
		MountID: event.Process.MountID,
		Inode:   event.Process.Inode,
	}
	entry, ok := lc.forkCounters.Get(key)
	if ok {
		counter := entry.(*uint64)
		*counter++
		if *counter >= uint64(lc.ForkBombThreshold) {
			forkBomb = true

			// reset counter to avoid spamming fork bomb alerts until the best_effort move takes effect
			*counter = 0
		}
	} else {
		newCount = uint64(1)
		lc.forkCounters.Add(key, &newCount)
	}

	if forkBomb {
		if err := lc.statsdClient.Count(MetricForkBomb, 1, []string{}, 1); err != nil {
			log.Warn(errors.Wrapf(err, "failed to send %s metric", MetricForkBomb))
		}
		lc.probe.DispatchCustomEvent(NewForkBombEvent(event))

		// drop fork events with the given cookie in kernel space
		lc.probe.resolvers.ProcessResolver.MarkCookieAsBestEffort(uint32(event.Process.ResolveCookie(event)))
	}
}

// GenericCount increments the event counter of the provided event type and pid
func (lc *LoadController) GenericCount(event *Event) {
	lc.Lock()
	defer lc.Unlock()

	entry, ok := lc.eventsCounters.Get(eventCounterLRUKey{Pid: event.Process.Pid, Cookie: event.Process.Cookie, Event: event.GetEventType()})
	if ok {
		counter := entry.(*uint64)
		atomic.AddUint64(counter, 1)
	} else {
		counter := uint64(1)
		lc.eventsCounters.Add(eventCounterLRUKey{Pid: event.Process.Pid, Cookie: event.Process.Cookie, Event: event.GetEventType()}, &counter)
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
	log.Tracef("discarding %s events from pid %d for %s seconds", maxKey.Event, maxKey.Pid, lc.DiscarderTimeout)
	if err := lc.probe.discardPIDWithTimeout(maxKey.Event, maxKey.Pid, lc.DiscarderTimeout); err != nil {
		log.Warnf("couldn't insert temporary discarder: %v", err)
		return
	}

	// update current total and remove biggest entry from cache
	oldMaxCount := atomic.SwapUint64(maxCount, 0)
	atomic.AddInt64(&lc.eventsTotal, -int64(oldMaxCount))

	if lc.statsdClient != nil {
		// send load_controller.pids_discarder metric
		tags := []string{
			fmt.Sprintf("event_type:%s", maxKey.Event),
		}
		if err := lc.statsdClient.Count(MetricLoadControllerPidDiscarder, 1, tags, 1.0); err != nil {
			log.Warnf("couldn't send load_controller.pids_discarder metric: %v", err)
			return
		}

		// fetch noisy process metadata
		process := lc.probe.resolvers.ProcessResolver.Resolve(maxKey.Pid, maxKey.Cookie)

		ts := time.Now()
		lc.probe.DispatchCustomEvent(
			NewNoisyProcessEvent(
				maxKey.Event,
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

// cleanup resets the internal counters
func (lc *LoadController) cleanup() {
	// Only a read lock is required: we're not adding / removing any entry in the LRUs, just updating their counters.
	lc.RLock()
	defer lc.RUnlock()

	// reset counts
	for _, key := range lc.eventsCounters.Keys() {
		val, ok := lc.eventsCounters.Peek(key)
		if !ok || val == nil {
			continue
		}
		counter := val.(*uint64)
		atomic.SwapUint64(counter, 0)
	}
	atomic.SwapInt64(&lc.eventsTotal, 0)

	// reset fork counters
	for _, key := range lc.forkCounters.Keys() {
		val, ok := lc.forkCounters.Peek(key)
		if !ok || val == nil {
			continue
		}
		counter := val.(*uint64)
		atomic.SwapUint64(counter, 0)
	}
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
