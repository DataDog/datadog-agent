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

const (
	oCHAN = 0
	hCHAN = 1
)

type jobBucket struct {
	idx     uint
	starter *time.Timer
	ticker  *time.Ticker
	halt    chan bool // to stop this bucket
	jobs    []check.Check
	mu      sync.RWMutex // to protect critical sections in struct's fields
}

func newJobBucket(idx uint, interval, start time.Duration) *jobBucket {
	bucket := &jobBucket{
		idx:  idx,
		halt: make(chan bool),
	}

	// ticker starts after start duration
	bucket.starter = time.AfterFunc(start, func() {
		bucket.mu.Lock()
		defer bucket.mu.Unlock()

		bucket.ticker = time.NewTicker(interval)
		log.Debugf("Bucket %d ticker started at interval %s", idx, interval)
	})

	return bucket
}

func (jb *jobBucket) stop() {
	select {
	case jb.halt <- true:
	default:
	}

	jb.mu.RLock()
	defer jb.mu.RUnlock()

	jb.starter.Stop()
	if jb.ticker != nil {
		jb.ticker.Stop()
	}
}

func (jb *jobBucket) addJob(c check.Check) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	jb.jobs = append(jb.jobs, c)
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

	var nb int
	if interval <= time.Second {
		nb = 1
	} else {
		nb = int(interval.Truncate(time.Second).Seconds())
	}
	for i := 0; i < nb; i++ {
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
	jq.buckets[jq.nextBucket].addJob(c)
	jq.nextBucket = (jq.nextBucket + 1) % uint(len(jq.buckets))
}

func (jq *jobQueue) removeJob(id check.ID) error {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	for i, bucket := range jq.buckets {
		bucket.mu.Lock()
		for j, c := range bucket.jobs {
			if c.ID() == id {
				jq.buckets[i].jobs = append(bucket.jobs[:j], bucket.jobs[j+1:]...)
				bucket.mu.Unlock()
				return nil
			}
		}
		bucket.mu.Unlock()
	}

	return fmt.Errorf("check with id %s is not in this Job Queue", id)
}

// run schedules the checks in the queue by posting them to the
// execution pipeline.
// Not blocking, runs in a new goroutine.
func (jq *jobQueue) run(out chan<- check.Check) {

	time.Sleep(time.Second) // wait for one ticker

	go func() {
		cases := make([]reflect.SelectCase, 2+len(jq.buckets))
		cases[oCHAN] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(jq.stop),
		}
		cases[hCHAN] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(jq.health.C),
		}

		ready := true
		for i, bucket := range jq.buckets {

			bucket.mu.RLock()
			ticker := reflect.ValueOf(nil)
			if bucket.ticker != nil {
				ticker = reflect.ValueOf(bucket.ticker.C)
			} else {
				ready = false
			}

			cases[2+i] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: ticker,
			}

			bucket.mu.RUnlock()
		}

		for jq.waitForTick(cases, out) {
			if ready {
				continue
			}

			ready = true
			for i, bucket := range jq.buckets {
				bucket.mu.RLock()
				if bucket.ticker != nil && cases[2+i].Chan == reflect.ValueOf(nil) {
					cases[2+1].Chan = reflect.ValueOf(bucket.ticker.C)
				} else {
					ready = false
				}
				bucket.mu.RUnlock()
			}
		}
		jq.stopped <- true
	}()
}

// waitForTicks enqueues the checks at a tick, and returns whether the queue
// should listen to the following tick (or stop)
func (jq *jobQueue) waitForTick(cases []reflect.SelectCase, out chan<- check.Check) bool {

	chosen, _, ok := reflect.Select(cases)
	if !ok {
		// The chosen channel has been closed, so zero out the channel to disable the case
		// should never really happen: we use the stop channel.
		cases[chosen].Chan = reflect.ValueOf(nil)
		return true
	}

	switch chosen {
	case oCHAN:
		// someone asked to stop this queue
		jq.mu.RLock()
		defer jq.mu.RUnlock()

		for _, bucket := range jq.buckets {
			bucket.stop()
		}
		jq.health.Deregister()
		return false
	case hCHAN:
	default:
		// normal case, (re)schedule the queue
		idx := chosen - 2
		log.Debugf("Processing checks in queue %s and bucket %d", jq.interval, idx)

		bucket := jq.buckets[idx]
		bucket.mu.RLock()

		for _, check := range bucket.jobs {
			// sending to `out` is blocking, we need to constantly check that someone
			// isn't asking to stop this queue
			select {
			case <-jq.stop:
				bucket.mu.RUnlock()
				jq.mu.RLock()
				defer jq.mu.RUnlock()

				for _, bucket := range jq.buckets {
					bucket.stop()
				}
				jq.health.Deregister()
				return false
			case out <- check:
				log.Debugf("Enqueuing check %s for queue %s", check, jq.interval)
			}
		}
		bucket.mu.RUnlock()
	}

	return true
}
