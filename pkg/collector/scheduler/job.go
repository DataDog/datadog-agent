// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type jobBucket struct {
	jobs []check.Check
	mu   sync.RWMutex // to protect critical sections in struct's fields
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

// removeJob removes the check from the bucket, and returns
// whether the check was indeed in the bucket (and therefore actually removed)
func (jb *jobBucket) removeJob(id checkid.ID) bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	for i, c := range jb.jobs {
		if c.ID() == id {
			// delete the check from the jobs slice, making sure the backing array
			// doesn't keep a reference to the check, so that it can be GC'ed.
			// Logic from https://github.com/golang/go/wiki/SliceTricks
			copy(jb.jobs[i:], jb.jobs[i+1:])
			jb.jobs[len(jb.jobs)-1] = nil
			jb.jobs = jb.jobs[:len(jb.jobs)-1]
			return true
		}
	}
	return false
}

// jobQueue contains a list of checks (called jobs) that need to be
// scheduled at a certain interval.
type jobQueue struct {
	interval            time.Duration
	stop                chan bool // to stop this queue
	stopped             chan bool // signals that this queue has stopped
	buckets             []*jobBucket
	bucketTicker        *time.Ticker
	lastTick            time.Time
	sparseStep          uint
	currentBucketIdx    uint
	schedulingBucketIdx uint
	running             bool
	health              *health.Handle
	mu                  sync.RWMutex // to protect critical sections in struct's fields
}

// newJobQueue creates a new jobQueue instance
func newJobQueue(interval time.Duration) *jobQueue {
	jq := &jobQueue{
		interval:     interval,
		stop:         make(chan bool),
		stopped:      make(chan bool),
		health:       health.RegisterLiveness(fmt.Sprintf("collector-queue-%vs", interval.Seconds())),
		bucketTicker: time.NewTicker(time.Second),
	}

	var nb int
	if interval <= time.Second {
		nb = 1
	} else {
		nb = int(interval.Truncate(time.Second).Seconds())
	}
	for i := 0; i < nb; i++ {
		bucket := &jobBucket{}
		jq.buckets = append(jq.buckets, bucket)
	}

	// compute step for sparse scheduling
	if nb <= 2 {
		jq.sparseStep = uint(1)
	} else {
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
	}

	return jq
}

// addJob is a convenience method to add a check to a queue
func (jq *jobQueue) addJob(c check.Check) {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	// Checks scheduled to buckets scheduled with sparse round-robin
	jq.buckets[jq.schedulingBucketIdx].addJob(c)
	jq.schedulingBucketIdx = (jq.schedulingBucketIdx + jq.sparseStep) % uint(len(jq.buckets))
}

func (jq *jobQueue) removeJob(id checkid.ID) error {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	for _, bucket := range jq.buckets {
		if found := bucket.removeJob(id); found {
			return nil
		}
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
func (jq *jobQueue) run(s *Scheduler) {

	go func() {
		log.Debugf("Job queue is running...")
		for jq.process(s) {
			// empty
		}
		jq.stopped <- true
	}()
}

// process  enqueues the checks at a tick, and returns whether the queue
// should listen to the following tick (or stop)
func (jq *jobQueue) process(s *Scheduler) bool {

	select {
	case <-jq.stop:
		jq.health.Deregister() //nolint:errcheck
		return false
	case t := <-jq.bucketTicker.C:
		log.Tracef("Bucket ticked... current index: %v", jq.currentBucketIdx)
		jq.mu.Lock()
		if !jq.lastTick.Equal(time.Time{}) && t.After(jq.lastTick.Add(2*time.Second)) {
			log.Debugf("Previous bucket took over %v to schedule. Next checks will be running behind the schedule.", t.Sub(jq.lastTick))
		}
		jq.lastTick = t
		bucket := jq.buckets[jq.currentBucketIdx]
		jq.mu.Unlock()

		bucket.mu.RLock()
		// we have to copy to avoid blocking the bucket :(
		// blocking could interfere with scheduling new jobs
		jobs := []check.Check{}
		jobs = append(jobs, bucket.jobs...)
		bucket.mu.RUnlock()

		log.Tracef("Jobs in bucket: %v", jobs)

		for _, check := range jobs {
			if !s.IsCheckScheduled(check.ID()) {
				continue
			}

			select {
			// blocking, we'll be here as long as it takes
			case s.checksPipe <- check:
			case <-jq.stop:
				jq.health.Deregister() //nolint:errcheck
				return false
			}

			select {
			// we were able to schedule a check so we're not stuck, therefore poll the health chan
			case <-jq.health.C:
			default:
			}
		}
		jq.mu.Lock()
		jq.currentBucketIdx = (jq.currentBucketIdx + 1) % uint(len(jq.buckets))
		jq.mu.Unlock()
	case <-jq.health.C:
		// nothing
	}

	return true
}
