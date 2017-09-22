// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"
)

// jobQueue contains a list of checks (called jobs) that need to be
// scheduled at a certain interval.
type jobQueue struct {
	interval time.Duration
	stop     chan bool
	ticker   *time.Ticker
	jobs     []check.Check
	running  bool
	mu       sync.RWMutex // to protect critical sections in struct's fields
}

// newJobQueue creates a new jobQueue instance
// the stop channel is buffered so the scheduler loop can send a message to stop
// without blocking
func newJobQueue(interval time.Duration) *jobQueue {
	return &jobQueue{
		interval: interval,
		ticker:   time.NewTicker(time.Duration(interval)),
		stop:     make(chan bool, 1),
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
// This doesn't block.
func (jq *jobQueue) run(out chan<- check.Check) {
	go func() {
		for {
			select {
			case <-jq.stop:
				// someone asked to stop this queue
				jq.ticker.Stop()
			case <-jq.ticker.C:
				// normal case, (re)schedule the queue
				jq.mu.RLock()
				for _, check := range jq.jobs {
					log.Debugf("Enqueuing check %s for queue %d", check, jq.interval)
					out <- check
				}
				jq.mu.RUnlock()
			}
		}
	}()
}
