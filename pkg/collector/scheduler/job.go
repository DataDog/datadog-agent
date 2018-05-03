// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

type jobBucket struct {
	idx     uint
	starter *time.Timer
	ticker  *time.Ticker
	cue     chan uint
	halt    chan bool // to stop this bucket
	jobs    []check.Check
}

func newJobBucket(idx uint, interval, start time.Duration) *jobBucket {
	bucket := &jobBucket{idx: idx}

	// ticker starts after start duration
	bucket.starter = time.AfterFunc(start, func() {
		bucket.ticker = time.NewTicker(interval)
	})

	// start ticker bucket cue processor
	go func() {
		for {
			select {
			case <-bucket.ticker.C:
				bucket.cue <- bucket.idx
			case <-bucket.halt:
				return
			}
		}
	}()

	return bucket
}

func (jb *jobBucket) stop() {
	jb.ticker.Stop()
	jb.halt <- true
}

// jobQueue contains a list of checks (called jobs) that need to be
// scheduled at a certain interval.
type jobQueue struct {
	interval        time.Duration
	stop            chan bool // to stop this queue
	stopped         chan bool // signals that this queue has stopped
	buckets         []*jobBucket
	nextBucket      uint
	bucketScheduled uint
	running         bool
	health          *health.Handle
	mu              sync.RWMutex // to protect critical sections in struct's fields
}

// newJobQueue creates a new jobQueue instance
func newJobQueue(interval time.Duration) *jobQueue {
	jq := &jobQueue{
		interval: interval,
		stop:     make(chan bool),
		stopped:  make(chan bool),
		health:   health.Register("collector-queue"),
	}

	nBuckets := int(interval.Truncate(time.Second).Seconds())
	for i := 0; i < nBuckets; i++ {
		bucket := newJobBucket(uint(i), interval, time.Duration(time.Duration(i)*time.Second))
		jq.buckets = append(jq.buckets, bucket)
	}

	return jq
}

// addJob is a convenience method to add a check to a queue
func (jq *jobQueue) addJob(c check.Check) {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	// Checks scheduled to buckets scheduled in round-robin
	jq.buckets[jq.nextBucket].jobs = append(jq.buckets[jq.nextBucket].jobs, c)
	jq.nextBucket = (jq.nextBucket + 1) % uint(len(jq.buckets))
}

func (jq *jobQueue) removeJob(id check.ID) error {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	for i, bucket := range jq.buckets {
		for j, c := range bucket.jobs {
			if c.ID() == id {
				jq.buckets[i].jobs = append(bucket.jobs[:j], bucket.jobs[j+1:]...)
				return nil
			}
		}
	}

	return fmt.Errorf("check with id %s is not in this Job Queue", id)
}

// run schedules the checks in the queue by posting them to the
// execution pipeline.
// Not blocking, runs in a new goroutine.
func (jq *jobQueue) run(out chan<- check.Check) {
	go func() {
		cases := make([]reflect.SelectCase, len(jq.buckets))
		for i, bucket := range jq.buckets {
			cases[i] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(bucket.cue),
			}
		}

		for jq.waitForTick(cases, out) {
		}
		jq.stopped <- true
	}()
}

// waitForTicks enqueues the checks at a tick, and returns whether the queue
// should listen to the following tick (or stop)
func (jq *jobQueue) waitForTick(cases []reflect.SelectCase, out chan<- check.Check) bool {

	select {
	case <-jq.stop:
		// someone asked to stop this queue
		for _, bucket := range jq.buckets {
			bucket.stop()
		}
		jq.health.Deregister()
		return false
	case <-jq.health.C:
	default:
		// normal case, (re)schedule the queue
		jq.mu.RLock()
		chosen, idx, ok := reflect.Select(cases)
		if !ok {
			// The chosen channel has been closed, so zero out the channel to disable the case
			// should never really happen: we use the stop channel.
			cases[chosen].Chan = reflect.ValueOf(nil)
			jq.mu.RUnlock()
			return true
		}

		for _, check := range jq.buckets[idx.Uint()].jobs {
			// sending to `out` is blocking, we need to constantly check that someone
			// isn't asking to stop this queue
			select {
			case <-jq.stop:
				for _, bucket := range jq.buckets {
					bucket.stop()
				}
				jq.health.Deregister()
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
