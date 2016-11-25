package check

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	assert.Nil(t, r.pending)
}

func TestRun(t *testing.T) {
	r := NewRunner()

	// let's fake the runner already started, this should be a noop
	r.pending = make(chan Check)
	r.Run(1)

	r = NewRunner()
	r.Run(1)
	assert.NotNil(t, r.pending)
	assert.NotNil(t, r.runningChecks)
	close(r.pending)
}

func TestStop(t *testing.T) {
	r := NewRunner()
	r.Run(1)
	r.Stop()
	_, ok := <-r.pending
	assert.False(t, ok)

	// calling Stop on a stopped runner should be a noop
	r.Stop()
}

func TestGetChan(t *testing.T) {
	r := NewRunner()
	assert.Nil(t, r.GetChan())
	r.Run(1)
	assert.NotNil(t, r.GetChan())
}

func TestWork(t *testing.T) {
	r := NewRunner()
	r.Run(1)
	c1 := TestCheck{}
	c2 := TestCheck{doErr: true}

	r.pending <- &c1
	r.pending <- &c2
	assert.True(t, c1.hasRun)
	r.Stop()

	// fake a check is already running
	r = NewRunner()
	r.Run(1)
	c3 := new(TestCheck)
	r.runningChecks[c3.String()] = c3
	r.pending <- c3
	// wait to be sure the worker tried to run the check
	time.Sleep(100 * time.Millisecond)
	assert.False(t, c3.hasRun)
}
