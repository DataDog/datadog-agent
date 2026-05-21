// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"sync"
	"time"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// trialRetiredTTL bounds how long AD remembers retired trial check IDs after it
// enqueues an unschedule. The scheduler controller applies schedule changes
// asynchronously and declares an operation unhealthy after schedulersTimeout
// (currently 5 minutes), so keeping retired IDs for the same window covers the
// expected drain period without retaining high-churn discovery IDs forever.
// If a stale queued run reports after this TTL, it is treated like a fresh
// trial result; that is preferable to an unbounded retired-ID map, and should
// only happen if the scheduler controller is already unhealthy or badly stuck.
const trialRetiredTTL = 5 * time.Minute

// trialRegistry tracks consecutive failures of trial-mode (discovery)
// checks. It also tracks recently retired trial checks: AutoConfig unschedule
// requests are applied asynchronously by the scheduler controller, so a check
// can still have stale scheduler/runner work in flight after AD has decided to
// remove it. Retired IDs keep those late trial results suppressed and prevent a
// late success from promoting a check that AD is already unscheduling.
type trialRegistry struct {
	threshold int
	mu        sync.Mutex
	counts    map[checkid.ID]int
	retired   map[checkid.ID]time.Time
}

func newTrialRegistry(threshold int) *trialRegistry {
	return &trialRegistry{
		threshold: threshold,
		counts:    map[checkid.ID]int{},
		retired:   map[checkid.ID]time.Time{},
	}
}

// recordResult records a check run outcome. It returns whether the worker
// should suppress the result from normal integration reporting and whether
// this result newly crossed the failure threshold, which means AutoConfig
// should enqueue an unschedule.
func (r *trialRegistry) recordResult(id checkid.ID, ok bool) (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pruneRetiredLocked(time.Now())
	if _, found := r.retired[id]; found {
		return true, false
	}

	if ok {
		delete(r.counts, id)
		return false, false
	}

	r.counts[id]++
	if r.counts[id] < r.threshold {
		return true, false
	}

	r.retireLocked(id, time.Now())
	return true, true
}

// retire marks id as retired because AD requested unscheduling. The scheduler
// controller applies that request asynchronously, so already-queued runs may
// still report trial results after the config left AD's scheduled set.
func (r *trialRegistry) retire(id checkid.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retireLocked(id, time.Now())
}

// reset drops all tracked state for id. Called when a discovery config is
// scheduled again so a fresh trial check is not affected by an earlier retire.
func (r *trialRegistry) reset(id checkid.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.counts, id)
	delete(r.retired, id)
}

func (r *trialRegistry) retireLocked(id checkid.ID, now time.Time) {
	delete(r.counts, id)
	r.retired[id] = now
	r.pruneRetiredLocked(now)
}

// pruneRetiredLocked removes retired IDs whose async-unschedule guard window
// has expired. It runs opportunistically while the registry lock is already
// held instead of using a background goroutine: trial results and retire events
// are the only times this state matters, and this keeps lifecycle management
// local to the registry.
func (r *trialRegistry) pruneRetiredLocked(now time.Time) {
	for id, retiredAt := range r.retired {
		if now.Sub(retiredAt) > trialRetiredTTL {
			delete(r.retired, id)
		}
	}
}
