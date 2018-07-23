// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

const (
	stopChannel   = 0
	healthChannel = 1
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

func (jb *jobBucket) size() int {
	jb.mu.RLock()
	defer jb.mu.RUnlock()

	return len(jb.jobs)
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
	sparseStep      uint
	nextBucket      uint
	bucketScheduled uint
	running         bool
	health          *health.Handle
	rand            *rand.Rand
	mu              sync.RWMutex // to protect critical sections in struct's fields
}

// newJobQueue creates a new jobQueue instance
func newJobQueue(interval time.Duration) *jobQueue {
	jq := &jobQueue{
		interval: interval,
		stop:     make(chan bool),
		stopped:  make(chan bool),
		health:   health.Register("collector-queue"),
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
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

	// compute step for sparse scheduling
	switch nb % 2 {
	case 0:
		step := nb / 2
		switch step % 2 {
		case 0:
			jq.sparseStep = uint(step - 1)
		case 1:
			jq.sparseStep = uint(step - 2)
		}
	case 1:
		jq.sparseStep = uint(nb / 2)
	}

	return jq
}

// addJob is a convenience method to add a check to a queue
func (jq *jobQueue) addJob(c check.Check) {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	// Checks scheduled to buckets scheduled in round-robin
	jq.buckets[jq.nextBucket].addJob(c)
	jq.nextBucket = (jq.nextBucket + jq.sparseStep) % uint(len(jq.buckets))
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

func (jq *jobQueue) stats() map[string]interface{} {
	jq.mu.RLock()
	defer jq.mu.RUnlock()

	nJobs := 0
	nBuckets := 0
	for _, bucket := range jq.buckets {
		nJobs += bucket.size()
		nBuckets++
	}

	return map[string]interface{}{
		"Interval": jq.interval / time.Second,
		"Buckets":  nBuckets,
		"Size":     nJobs,
	}
}

// run schedules the checks in the queue by posting them to the
// execution pipeline.
// Not blocking, runs in a new goroutine.
func (jq *jobQueue) run(out chan<- check.Check) {

	time.Sleep(time.Second) // wait for one ticker

	go func() {
		cases := make([]reflect.SelectCase, 2+len(jq.buckets))
		cases[stopChannel] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(jq.stop),
		}
		cases[healthChannel] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(jq.health.C),
		}

		ready := true
		for i, bucket := range jq.buckets {

			bucket.mu.RLock()
			if bucket.ticker != nil {
				cases[2+i] = reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(bucket.ticker.C),
				}
			} else {
				ready = false
				cases[2+i] = reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(nil),
				}
			}
			bucket.mu.RUnlock()
		}

		for jq.waitForTick(cases, out) {
			if ready {
				continue
			}

			log.Debugf("Not all buckets are ticking")

			ready = true
			for i, bucket := range jq.buckets {
				bucket.mu.RLock()
				if bucket.ticker == nil {
					ready = false
				} else if bucket.ticker != nil && (!cases[2+i].Chan.IsValid() || cases[2+i].Chan.IsNil()) {
					log.Debugf("Adding ticker to select case")
					cases[2+i].Chan = reflect.ValueOf(bucket.ticker.C)
				}
				bucket.mu.RUnlock()
			}
		}
		jq.stopped <- true
	}()
}

// stopBuckets Stop all buckets in a job queue. Should be thread-safe in the context of the jobQueue.
func (jq *jobQueue) stopBuckets() {
	jq.mu.RLock()
	defer jq.mu.RUnlock()

	for _, bucket := range jq.buckets {
		bucket.stop()
	}
	jq.health.Deregister()
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

	deadline := time.After(time.Second)

	switch chosen {
	case stopChannel:
		// someone asked to stop this queue
		jq.stopBuckets()
		return false
	case healthChannel:
	default:
		// normal case, (re)schedule the queue
		idx := chosen - 2
		log.Debugf("Processing checks in queue %s and bucket %d", jq.interval, idx)

		bucket := jq.buckets[idx]
		bucket.mu.RLock()

		// randomize job scheduling to avoid job starvation
		nJobs := len(bucket.jobs)
		if nJobs > 0 {
			jIdx := jq.rand.Intn(nJobs)
			jobs := append(bucket.jobs[jIdx:nJobs], bucket.jobs[0:jIdx]...)

		jobloop:
			for _, check := range jobs {
				// sending to `out` is blocking, we need to constantly check that someone
				// isn't asking to stop this queue
				select {
				case <-jq.stop:
					bucket.mu.RUnlock()

					jq.stopBuckets()
					return false
				case out <- check:
					log.Debugf("Enqueuing check %s for queue %s", check, jq.interval)
				case <-deadline:
					log.Infof("Bucket[%d] deadline reached not enough runners were available - skipping runs", idx)
					break jobloop
				}
			}
		}
		bucket.mu.RUnlock()
	}

	return true
}
