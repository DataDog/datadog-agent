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
func (c *TestCheck) ID() string                 { return c.String() }

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
	return NewScheduler()
}

func TestNewScheduler(t *testing.T) {
	s := NewScheduler()
	assert.Nil(t, s.checksPipe)
	assert.Equal(t, len(s.jobQueues), 0)
	assert.Equal(t, s.running, uint32(0))
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

	c := make(chan<- check.Check)

	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run(c)
	assert.Equal(t, uint32(1), s.running)
	assert.True(t, s.jobQueues[10].running)

	// Calling Run again should be a non blocking, noop procedure
	s.Run(c)
}

func TestStop(t *testing.T) {
	c := make(chan<- check.Check)
	s := getScheduler()
	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run(c)

	err := s.Stop()
	assert.Nil(t, err)
	assert.Equal(t, uint32(0), s.running)
	assert.False(t, s.jobQueues[10].running)

	// stopping again should be non blocking, noop and return nil
	assert.Nil(t, s.Stop())
}

func TestStopTimeout(t *testing.T) {
	s := getScheduler()
	c := make(chan<- check.Check)
	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run(c)
	s.Stop()

	// to trigger the timeout, fake scheduler state to `running`...
	s.running = uint32(1)
	// ...now, stopping should trigger the timeout, set it at 1ms
	err := s.Stop(time.Millisecond)

	assert.NotNil(t, err)
}

func TestReload(t *testing.T) {
	s := getScheduler()
	c := make(chan<- check.Check)
	s.Enter([]check.Check{&TestCheck{intl: 10}})
	s.Run(c)

	// add a queue to check the reload picks it up
	s.Enter([]check.Check{&TestCheck{intl: 1}})

	err := s.Reload()
	assert.Nil(t, err)

	// check the scheduler is up again with the new queue running
	assert.Equal(t, uint32(1), s.running)
	assert.True(t, s.jobQueues[1].running)
}
