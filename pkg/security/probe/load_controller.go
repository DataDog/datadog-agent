// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const (
	defaultRateLimit = 1 // per second
)

type eventCounterLRUKey struct {
	Pid    uint32
	Cookie uint32
}

// LoadController is used to monitor and control the pressure put on the host
type LoadController struct {
	sync.RWMutex
	probe *Probe

	eventsTotal        *atomic.Int64
	eventsCounters     *simplelru.LRU[eventCounterLRUKey, *atomic.Uint64]
	pidDiscardersCount *atomic.Int64

	EventsCountThreshold int64
	DiscarderTimeout     time.Duration
	ControllerPeriod     time.Duration

	NoisyProcessCustomEventRate *rate.Limiter
}

// NewLoadController instantiates a new load controller
func NewLoadController(probe *Probe) (*LoadController, error) {
	lru, err := simplelru.NewLRU[eventCounterLRUKey, *atomic.Uint64](probe.Config.Probe.PIDCacheSize, nil)
	if err != nil {
		return nil, err
	}

	lc := &LoadController{
		probe: probe,

		eventsTotal:        atomic.NewInt64(0),
		eventsCounters:     lru,
		pidDiscardersCount: atomic.NewInt64(0),

		EventsCountThreshold: probe.Config.Probe.LoadControllerEventsCountThreshold,
		DiscarderTimeout:     probe.Config.Probe.LoadControllerDiscarderTimeout,
		ControllerPeriod:     probe.Config.Probe.LoadControllerControlPeriod,

		NoisyProcessCustomEventRate: rate.NewLimiter(rate.Every(time.Second), defaultRateLimit),
	}
	return lc, nil
}

// SendStats sends load controller stats
func (lc *LoadController) SendStats() error {
	// send load_controller.pids_discarder metric
	if count := lc.pidDiscardersCount.Swap(0); count > 0 {
		if err := lc.probe.StatsdClient.Count(metrics.MetricLoadControllerPidDiscarder, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send load_controller.pids_discarder metric: %w", err)
		}
	}
	return nil
}

// Count processes the provided events and ensures the load of the provided event type is within the configured limits
func (lc *LoadController) Count(event *model.Event) {
	switch event.GetEventType() {
	case model.ExecEventType, model.InvalidateDentryEventType, model.ForkEventType:
	case model.ExitEventType:
		lc.cleanupCounter(event.ProcessContext.Pid, event.ProcessContext.Cookie)
	default:
		lc.GenericCount(event)
	}
}

// GenericCount increments the event counter of the provided event type and pid
func (lc *LoadController) GenericCount(event *model.Event) {
	lc.Lock()
	defer lc.Unlock()

	entry, ok := lc.eventsCounters.Get(eventCounterLRUKey{Pid: event.ProcessContext.Pid, Cookie: event.ProcessContext.Cookie})
	if ok {
		entry.Inc()
	} else {
		lc.eventsCounters.Add(eventCounterLRUKey{Pid: event.ProcessContext.Pid, Cookie: event.ProcessContext.Cookie}, atomic.NewUint64(1))
	}
	newTotal := lc.eventsTotal.Inc()

	if newTotal >= lc.EventsCountThreshold {
		lc.discardNoisiestProcess()
	}
}

// discardNoisiestProcess determines the noisiest process and event_type tuple and pushes a temporary discarder
func (lc *LoadController) discardNoisiestProcess() {
	// iterate over the LRU map to retrieve the noisiest process & event_type tuple
	var maxKey eventCounterLRUKey
	var maxCount *atomic.Uint64
	for _, key := range lc.eventsCounters.Keys() {
		entry, ok := lc.eventsCounters.Peek(key)
		if !ok || entry == nil {
			continue
		}

		// update max if necessary
		if maxCount == nil || maxCount.Load() < entry.Load() {
			maxCount = entry
			maxKey = key
		}
	}
	if maxCount == nil {
		// LRU is empty nothing to discard
		return
	}

	var erpcRequest erpc.ERPCRequest

	// push a temporary discarder on the noisiest process & event type tuple
	seclog.Tracef("discarding events from pid %d for %s seconds", maxKey.Pid, lc.DiscarderTimeout)
	if err := lc.probe.pidDiscarders.discardWithTimeout(&erpcRequest, allEventTypes, maxKey.Pid, lc.DiscarderTimeout.Nanoseconds()); err != nil {
		seclog.Warnf("couldn't insert temporary discarder: %v", err)
		return
	}

	// update current total and remove biggest entry from cache.  Note that there
	// is a chance of the maxCount value being incremented between these two atomic
	// operations, but that's OK -- the event is still counted.
	oldMaxCount := maxCount.Swap(0)
	lc.eventsTotal.Sub(int64(oldMaxCount))

	lc.pidDiscardersCount.Inc()

	if lc.NoisyProcessCustomEventRate.Allow() {
		process := lc.probe.resolvers.ProcessResolver.Resolve(maxKey.Pid, maxKey.Pid, 0)
		if process == nil {
			seclog.Warnf("Unable to resolve process with pid: %d", maxKey.Pid)
			return
		}

		ts := time.Now()
		lc.probe.DispatchCustomEvent(
			NewNoisyProcessEvent(
				oldMaxCount,
				lc.EventsCountThreshold,
				lc.ControllerPeriod,
				ts.Add(lc.DiscarderTimeout),
				maxKey.Pid,
				process.Comm,
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
	counter, ok := lc.eventsCounters.Get(key)
	if ok {
		lc.eventsTotal.Sub(int64(counter.Load()))
		lc.eventsCounters.Remove(key)
	}
}

// cleanup resets the internal counters
func (lc *LoadController) cleanup() {
	lc.Lock()
	defer lc.Unlock()

	// purge counters
	lc.eventsCounters.Purge()
	lc.eventsTotal.Store(0)
}

// Start resets the internal counters periodically
func (lc *LoadController) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

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
