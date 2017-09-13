// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package scheduler

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

// FIXTURE
type TestCheck struct{ intl time.Duration }

func (c *TestCheck) String() string                                     { return "TestCheck" }
func (c *TestCheck) Configure(check.ConfigData, check.ConfigData) error { return nil }
func (c *TestCheck) Interval() time.Duration                            { return c.intl }
func (c *TestCheck) Run() error                                         { return nil }
func (c *TestCheck) Stop()                                              {}
func (c *TestCheck) ID() check.ID                                       { return check.ID(c.String()) }
func (c *TestCheck) GetWarnings() []error                               { return []error{} }
func (c *TestCheck) GetMetricStats() (map[string]int64, error)          { return make(map[string]int64), nil }

// wait 1s for a predicate function to return true, use polling
// instead of a giant sleep.
// predicate f must return true if the desired condition is met
func consistently(f func() bool) bool {
	for i := 0; i < 100; i++ {
		if f() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	// condition was not met during the wait period
	return false
}

func getScheduler() *Scheduler {
	return NewScheduler(make(chan<- check.Check))
}

func TestNewScheduler(t *testing.T) {
	c := make(chan<- check.Check)
	s := NewScheduler(c)
	assert.Equal(t, c, s.checksPipe)
	assert.Equal(t, len(s.jobQueues), 0)
	assert.Equal(t, s.running, uint32(0))
}

func TestEnter(t *testing.T) {
	c := &TestCheck{}
	s := getScheduler()

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
	assert.Len(t, s.jobQueues[c.intl].jobs, 1)

	// schedule another, same interval
	c = &TestCheck{intl: c.intl}
	s.Enter(c)
	assert.Len(t, s.jobQueues, 1)
	assert.Len(t, s.jobQueues[c.intl].jobs, 2)

	// schedule again the previous plus another with different interval
	s.Enter(c)
	c = &TestCheck{intl: 20 * time.Second}
	s.Enter(c)
	assert.Len(t, s.jobQueues, 2)
	assert.Len(t, s.jobQueues[1*time.Second].jobs, 3)
	assert.Len(t, s.jobQueues[c.intl].jobs, 1)
}

func TestCancel(t *testing.T) {
	c := &TestCheck{intl: 1 * time.Second}
	s := getScheduler()
	defer s.Stop()

	s.Enter(c)
	s.Run()
	s.Cancel(c.ID())
	assert.Len(t, s.jobQueues[c.intl].jobs, 0)
}

func TestRun(t *testing.T) {
	s := getScheduler()
	defer s.Stop()

	intl := 1 * time.Second
	s.Enter(&TestCheck{intl: intl})
	s.Run()
	assert.Equal(t, uint32(1), s.running)
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
	assert.Equal(t, uint32(0), s.running)
	assert.False(t, s.jobQueues[10*time.Second].running)

	// stopping again should be non blocking, noop and return nil
	assert.Nil(t, s.Stop())
}

func TestStopTimeout(t *testing.T) {
	s := getScheduler()
	s.Enter(&TestCheck{intl: 10})
	s.Run()
	s.Stop()

	// to trigger the timeout, fake scheduler state to `running`...
	s.running = uint32(1)
	// ...now, stopping should trigger the timeout, set it at 1ms
	err := s.Stop(time.Millisecond)

	assert.NotNil(t, err)
}

func TestReload(t *testing.T) {
	s := getScheduler()
	s.Enter(&TestCheck{intl: 10 * time.Second})
	s.Run()

	// add a queue to check the reload picks it up
	s.Enter(&TestCheck{intl: 1 * time.Second})

	err := s.Reload()
	assert.Nil(t, err)

	// check the scheduler is up again with the new queue running
	assert.Equal(t, uint32(1), s.running)
	assert.True(t, s.jobQueues[1*time.Second].running)
}

func TestTinyInterval(t *testing.T) {
	s := getScheduler()
	err := s.Enter(&TestCheck{intl: 1 * time.Millisecond})
	assert.NotNil(t, err)
}
