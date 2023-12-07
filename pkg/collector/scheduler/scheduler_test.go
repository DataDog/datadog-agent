// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

// FIXTURE
type TestCheck struct {
	stub.StubCheck
	intl time.Duration
}

func (c *TestCheck) Interval() time.Duration { return c.intl }

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

//nolint:revive // TODO(AML) Fix revive linter
func TestStopCancelsProducers(t *testing.T) {
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
