// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	"fmt"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// jobQueue contains a list of checks (called jobs) that need to be
// scheduled at a certain interval.
type jobQueue struct {
	interval     time.Duration
	stop         chan bool // to stop this queue
	stopped      chan bool // signals that this queue has stopped
	ticker       *time.Ticker
	jobs         []check.Check
	running      bool
	healthTicker *time.Ticker
	healthToken  health.ID
	mu           sync.RWMutex // to protect critical sections in struct's fields
}

// newJobQueue creates a new jobQueue instance
func newJobQueue(interval time.Duration) *jobQueue {
	return &jobQueue{
		interval:     interval,
		ticker:       time.NewTicker(time.Duration(interval)),
		stop:         make(chan bool),
		stopped:      make(chan bool),
		healthTicker: time.NewTicker(15 * time.Second),
		healthToken:  health.Register("collector-queue"),
	}
}

// addJob is a convenience method to add a check to a queue
func (jq *jobQueue) addJob(c check.Check) {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	jq.jobs = append(jq.jobs, c)
}

func (jq *jobQueue) removeJob(id check.ID) error {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	for i, c := range jq.jobs {
		if c.ID() == id {
			jq.jobs = append(jq.jobs[:i], jq.jobs[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("check with id %s is not in this Job Queue", id)
}

// run schedules the checks in the queue by posting them to the
// execution pipeline.
// Not blocking, runs in a new goroutine.
func (jq *jobQueue) run(out chan<- check.Check) {
	go func() {
		for jq.waitForTick(out) {
		}
		jq.stopped <- true
	}()
}

// waitForTicks enqueues the checks at a tick, and returns whether the queue
// should listen to the following tick (or stop)
func (jq *jobQueue) waitForTick(out chan<- check.Check) bool {
	select {
	case <-jq.stop:
		// someone asked to stop this queue
		jq.ticker.Stop()
		jq.healthTicker.Stop()
		health.Deregister(jq.healthToken)
		return false
	case <-jq.healthTicker.C:
		health.Ping(jq.healthToken)
	case <-jq.ticker.C:
		// normal case, (re)schedule the queue
		jq.mu.RLock()
		for _, check := range jq.jobs {
			// sending to `out` is blocking, we need to constantly check that someone
			// isn't asking to stop this queue
			select {
			case <-jq.stop:
				jq.ticker.Stop()
				jq.mu.RUnlock()
				return false
			case out <- check:
				log.Debugf("Enqueuing check %s for queue %d", check, jq.interval)
			}
		}
		jq.mu.RUnlock()
	}

	return true
}
