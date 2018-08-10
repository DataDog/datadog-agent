// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package runner

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

// FIXTURE
type TestCheck struct {
	doErr  bool
	hasRun bool
}

func (c *TestCheck) String() string                                     { return "TestCheck" }
func (c *TestCheck) Version() string                                    { return "" }
func (c *TestCheck) Stop()                                              {}
func (c *TestCheck) Configure(integration.Data, integration.Data) error { return nil }
func (c *TestCheck) Interval() time.Duration                            { return 1 }
func (c *TestCheck) Run() error {
	if c.doErr {
		msg := "A tremendous error occurred."
		return errors.New(msg)
	}

	c.hasRun = true
	return nil
}
func (c *TestCheck) ID() check.ID                              { return check.ID(c.String()) }
func (c *TestCheck) GetWarnings() []error                      { return nil }
func (c *TestCheck) GetMetricStats() (map[string]int64, error) { return make(map[string]int64), nil }

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	assert.NotNil(t, r.pending)
	assert.NotNil(t, r.runningChecks)
}

func TestStop(t *testing.T) {
	r := NewRunner()
	r.Stop()
	_, ok := <-r.pending
	assert.False(t, ok)

	// calling Stop on a stopped runner should be a noop
	r.Stop()
}

func TestGetChan(t *testing.T) {
	r := NewRunner()
	assert.NotNil(t, r.GetChan())
}

func TestWork(t *testing.T) {
	defaultNumWorkers = 1
	r := NewRunner()
	c1 := TestCheck{}
	c2 := TestCheck{doErr: true}

	r.pending <- &c1
	r.pending <- &c2
	assert.True(t, c1.hasRun)
	r.Stop()

	// fake a check is already running
	r = NewRunner()
	c3 := new(TestCheck)
	r.runningChecks[c3.ID()] = c3
	r.pending <- c3
	// wait to be sure the worker tried to run the check
	time.Sleep(100 * time.Millisecond)
	assert.False(t, c3.hasRun)
}

func TestLogging(t *testing.T) {
	r := NewRunner()
	c := TestCheck{}
	s := &check.Stats{
		CheckID:   c.ID(),
		CheckName: c.String(),
	}
	s.TotalRuns = 0
	checkStats.Stats[c.String()] = make(map[check.ID]*check.Stats)
	checkStats.Stats[c.String()][c.ID()] = s

	doLog, lastLog := shouldLog(c.ID())
	assert.True(t, doLog)
	assert.False(t, lastLog)

	s.TotalRuns = 5
	doLog, lastLog = shouldLog(c.ID())
	assert.True(t, doLog)
	assert.True(t, lastLog)

	s.TotalRuns = 6
	doLog, lastLog = shouldLog(c.ID())
	assert.False(t, doLog)
	assert.False(t, lastLog)

	s.TotalRuns = 20
	doLog, lastLog = shouldLog(c.ID())
	assert.True(t, doLog)
	assert.False(t, lastLog)

	r.Stop()
}

type TimingoutCheck struct {
	TestCheck
}

func (tc *TimingoutCheck) Stop() {
	for {
		runtime.Gosched()
	}
}
func (tc *TimingoutCheck) String() string { return "TimeoutTestCheck" }

func TestStopCheck(t *testing.T) {
	r := NewRunner()
	err := r.StopCheck("foo")
	assert.Nil(t, err)

	c1 := &TestCheck{}
	r.runningChecks[c1.ID()] = c1
	err = r.StopCheck(c1.ID())
	assert.Nil(t, err)

	c2 := &TimingoutCheck{}
	r.runningChecks[c2.ID()] = c2
	err = r.StopCheck(c2.ID())
	assert.Equal(t, "timeout during stop operation on check id TestCheck", err.Error())
}
