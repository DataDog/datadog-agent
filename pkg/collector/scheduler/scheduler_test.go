package scheduler

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

// FIXTURE
type TestCheck struct{ intl time.Duration }

func (c *TestCheck) String() string             { return "TestCheck" }
func (c *TestCheck) Configure(check.ConfigData) {}
func (c *TestCheck) InitSender()                {}
func (c *TestCheck) Interval() time.Duration    { return c.intl }
func (c *TestCheck) Run() error                 { return nil }

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
	c := make(chan<- check.Check)
	return NewScheduler(c)
}

func TestNewScheduler(t *testing.T) {
	c := make(chan<- check.Check)
	s := NewScheduler(c)
	assert.Equal(t, s.checksPipe, c)
	assert.Equal(t, len(s.jobQueues), 0)
}

func TestEnter(t *testing.T) {
	c := []check.Check{&TestCheck{intl: 1}}
	s := getScheduler()

	// schedule one check, one interval
	s.Enter(c)
	assert.Len(t, s.jobQueues, 1)
	assert.Len(t, s.jobQueues[1].jobs, 1)

	// schedule another, same interval
	c = []check.Check{&TestCheck{intl: 1}}
	s.Enter(c)
	assert.Len(t, s.jobQueues, 1)
	assert.Len(t, s.jobQueues[1].jobs, 2)

	// schedule again the previous plus another with different interval
	c = append(c, &TestCheck{intl: 20})
	s.Enter(c)
	assert.Len(t, s.jobQueues, 2)
	assert.Len(t, s.jobQueues[1].jobs, 3)
	assert.Len(t, s.jobQueues[20].jobs, 1)
}

func TestRun(t *testing.T) {
	s := getScheduler()
	defer s.Stop()

	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run()
	assert.True(t, consistently(func() bool { return s.jobQueues[10].started }))
}

func TestStop(t *testing.T) {
	s := getScheduler()
	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run()

	err := s.Stop()
	assert.Nil(t, err)
	assert.True(t, consistently(func() bool { return s.jobQueues[10].started == false }))
}

func TestStopTimeout(t *testing.T) {
	s := getScheduler()
	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run()
	s.Stop()
	// stopping a second time triggers the timeout, set it at 1ms
	err := s.Stop(1)
	assert.NotNil(t, err)
}

func TestReload(t *testing.T) {
	s := getScheduler()
	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run()

	// check the scheduler has booted
	assert.True(t, consistently(func() bool { return s.jobQueues[10].started }))

	// add a queue to check the reload picks it up
	s.Enter([]check.Check{&TestCheck{intl: 1}})

	err := s.Reload()
	assert.Nil(t, err)

	// check the scheduler is up again with the new queue running
	assert.True(t, consistently(func() bool { return s.jobQueues[1].started }))
}
