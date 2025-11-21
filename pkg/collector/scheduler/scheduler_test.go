// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

// FIXTURE
type TestCheck struct {
	stub.StubCheck
	intl time.Duration
}

func (c *TestCheck) Interval() time.Duration { return c.intl }

func (c *TestCheck) RunOnce() bool { return false }

var initialMinAllowedInterval = minAllowedInterval

func consume(c chan check.Check, stop chan bool) {
	for {
		select {
		case <-c:
			continue
		case <-stop:
			return
		}
	}
}

func resetMinAllowedInterval() {
	minAllowedInterval = initialMinAllowedInterval
}

func getScheduler() *Scheduler {
	return NewScheduler(make(chan<- check.Check))
}

func TestNewScheduler(t *testing.T) {
	c := make(chan<- check.Check)
	s := NewScheduler(c)

	assert.Equal(t, c, s.checksPipe)
	assert.Equal(t, len(s.jobQueues), 0)
	assert.False(t, s.running.Load())
}

func TestEnter(t *testing.T) {
	c := &TestCheck{}
	ch := make(chan check.Check)
	stop := make(chan bool)
	s := NewScheduler(ch)

	// consume the enqueued checks
	go consume(ch, stop)

	// schedule passing a wrong interval value
	c.intl = -1
	err := s.Enter(c)
	assert.Len(t, s.jobQueues, 0)
	assert.NotNil(t, err)

	// schedule a one-time check
	c.intl = 0
	err = s.Enter(c)
	assert.Nil(t, err)

	// schedule one check, one interval
	c.intl = 1 * time.Second
	s.Enter(c)
	assert.Len(t, s.jobQueues, 1)
	assert.Len(t, s.jobQueues[c.intl].buckets, 1)
	assert.Len(t, s.jobQueues[c.intl].buckets[0].jobs, 1)

	// schedule another, same interval
	c = &TestCheck{intl: c.intl}
	s.Enter(c)
	assert.Len(t, s.jobQueues, 1)
	assert.Len(t, s.jobQueues[c.intl].buckets, 1)
	assert.Len(t, s.jobQueues[c.intl].buckets[0].jobs, 2)

	// schedule again the previous plus another with different interval
	s.Enter(c)
	c = &TestCheck{intl: 20 * time.Second}
	s.Enter(c)
	assert.Len(t, s.jobQueues, 2)
	assert.Len(t, s.jobQueues[1*time.Second].buckets[0].jobs, 3)
	assert.Len(t, s.jobQueues[c.intl].buckets, 20)
	assert.Len(t, s.jobQueues[c.intl].buckets[0].jobs, 1)

	stop <- true
}

func TestCancel(t *testing.T) {
	c := make(chan check.Check)
	stop := make(chan bool)
	chk := &TestCheck{intl: 1 * time.Second}

	// consume the enqueued checks
	go consume(c, stop)
	defer func() {
		stop <- true
	}()

	s := NewScheduler(c)
	defer s.Stop()

	s.Enter(chk)
	s.Run()
	s.Cancel(chk.ID())
	assert.Len(t, s.jobQueues[chk.intl].buckets[0].jobs, 0)
}

func TestRun(t *testing.T) {
	s := getScheduler()
	defer s.Stop()

	intl := 1 * time.Second
	s.Enter(&TestCheck{intl: intl})
	s.Run()
	assert.True(t, s.running.Load())
	assert.True(t, s.jobQueues[intl].running)

	// Calling Run again should be a non blocking, noop procedure
	s.Run()
}

func TestStop(t *testing.T) {
	s := getScheduler()
	s.Enter(&TestCheck{intl: 10 * time.Second})
	s.Run()

	err := s.Stop()
	assert.Nil(t, err)
	assert.False(t, s.running.Load())
	assert.False(t, s.jobQueues[10*time.Second].running)

	// stopping again should be non blocking, noop and return nil
	assert.Nil(t, s.Stop())
}

func TestStopCancelsProducers(_ *testing.T) {
	ch := make(chan check.Check)
	stop := make(chan bool)
	s := NewScheduler(ch)

	// consume the enqueued checks
	go consume(ch, stop)

	minAllowedInterval = time.Millisecond // for the purpose of this test, so that the scheduler actually schedules the checks
	defer resetMinAllowedInterval()

	s.Enter(&TestCheck{intl: time.Millisecond})
	s.Run()

	time.Sleep(2 * time.Millisecond) // wait for the scheduler to actually schedule the check

	s.Stop()
	// stop check consumer routine
	stop <- true
	// once the scheduler is stopped, it should be safe to close this channel. Otherwise, this should panic
	close(s.checksPipe)
	// sleep to make the runtime schedule the hanging producer goroutines, if there are any left
	time.Sleep(time.Millisecond)

}

func TestTinyInterval(t *testing.T) {
	s := getScheduler()
	err := s.Enter(&TestCheck{intl: 1 * time.Millisecond})
	assert.NotNil(t, err)
}

// Test that stopping the scheduler while one-time checks are still being enqueued works
func TestStopOneTimeSchedule(t *testing.T) {
	c := &TestCheck{}
	s := getScheduler()

	// schedule a one-time check
	c.intl = 0
	err := s.Enter(c)
	assert.Nil(t, err)
	s.Enter(c)

	s.Run()

	s.Stop()
	// this will panic if we didn't properly cancel all the one-time scheduling goroutines
	close(s.checksPipe)
	// sleep to make the runtime schedule the hanging goroutines, if there are any
	time.Sleep(time.Millisecond)
}

// RunOnceTestCheck is a test check that can be configured as run-once or regular
type RunOnceTestCheck struct {
	stub.StubCheck
	mu         sync.RWMutex
	id         string
	runOnce    bool
	intl       time.Duration
	runCounter int
}

func (c *RunOnceTestCheck) ID() checkid.ID { return checkid.ID(c.id) }
func (c *RunOnceTestCheck) RunOnce() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runOnce
}
func (c *RunOnceTestCheck) Interval() time.Duration { return c.intl }
func (c *RunOnceTestCheck) Run() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runCounter++
	return nil
}
func (c *RunOnceTestCheck) GetRunCounter() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runCounter
}

func TestRunOnceCheckDescheduling(t *testing.T) {
	checkChan := make(chan check.Check, 100)
	stop := make(chan bool)

	// Track what checks are scheduled
	scheduledChecks := make(map[checkid.ID]int)
	var mu sync.RWMutex
	go func() {
		for {
			select {
			case chk := <-checkChan:
				mu.Lock()
				scheduledChecks[chk.ID()]++
				mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
	defer func() {
		stop <- true
	}()

	// Helper to wait for a condition
	waitForCondition := func(condition func() bool, timeout time.Duration, msg string) {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if condition() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("Timeout waiting for: %s", msg)
	}

	// Helper to get scheduled count for a check
	getScheduledCount := func(id checkid.ID) int {
		mu.RLock()
		defer mu.RUnlock()
		return scheduledChecks[id]
	}

	s := NewScheduler(checkChan)

	// Create a run-once check and a regular check
	runOnceCheck := &RunOnceTestCheck{
		id:      "run-once-check",
		runOnce: true,
		intl:    1 * time.Second,
	}

	regularCheck := &RunOnceTestCheck{
		id:      "regular-check",
		runOnce: false,
		intl:    1 * time.Second,
	}

	// Manually create a job queue for 1 second interval and replace its ticker
	// This must be done BEFORE calling Enter to avoid race conditions
	jq := newJobQueue(1 * time.Second)
	tickerChan := make(chan time.Time, 10)
	jq.bucketTicker.Stop()
	jq.bucketTicker = &time.Ticker{C: tickerChan}
	s.jobQueues[1*time.Second] = jq

	// Schedule both checks
	err := s.Enter(runOnceCheck)
	require.NoError(t, err)
	err = s.Enter(regularCheck)
	require.NoError(t, err)

	// Start the scheduler
	s.Run()

	// Manually trigger the first tick - both checks should run
	tickerChan <- time.Now()
	waitForCondition(func() bool {
		return getScheduledCount("run-once-check") == 1 && getScheduledCount("regular-check") == 1
	}, 1*time.Second, "both checks to be scheduled once")

	// Verify both checks were scheduled once
	require.Equal(t, 1, getScheduledCount("run-once-check"), "run-once check should run exactly once")
	require.Equal(t, 1, getScheduledCount("regular-check"), "regular check should run once so far")

	// Verify run-once check was removed from the queue
	waitForCondition(func() bool {
		totalRunOnceJobs := 0
		for _, bucket := range jq.buckets {
			bucket.mu.RLock()
			for _, job := range bucket.jobs {
				if job.ID() == "run-once-check" {
					totalRunOnceJobs++
				}
			}
			bucket.mu.RUnlock()
		}
		return totalRunOnceJobs == 0
	}, 1*time.Second, "run-once check to be removed from queue")

	totalRunOnceJobs := 0
	totalRegularJobs := 0
	for _, bucket := range jq.buckets {
		bucket.mu.RLock()
		for _, job := range bucket.jobs {
			if job.ID() == "run-once-check" {
				totalRunOnceJobs++
			}
			if job.ID() == "regular-check" {
				totalRegularJobs++
			}
		}
		bucket.mu.RUnlock()
	}
	require.Equal(t, 0, totalRunOnceJobs, "run-once check should be removed from the queue")
	require.Equal(t, 1, totalRegularJobs, "regular check should remain in the queue")

	// Trigger another tick - only regular check should run
	tickerChan <- time.Now()
	waitForCondition(func() bool {
		return getScheduledCount("regular-check") == 2
	}, 1*time.Second, "regular check to run twice")

	require.Equal(t, 1, getScheduledCount("run-once-check"), "run-once check should still be 1")
	require.Equal(t, 2, getScheduledCount("regular-check"), "regular check should run twice")

	// Trigger one more tick to be sure
	tickerChan <- time.Now()
	waitForCondition(func() bool {
		return getScheduledCount("regular-check") == 3
	}, 1*time.Second, "regular check to run three times")

	require.Equal(t, 1, getScheduledCount("run-once-check"), "run-once check should still be 1")
	require.Equal(t, 3, getScheduledCount("regular-check"), "regular check should run three times")

	s.Stop()
}
