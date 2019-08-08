// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package runner

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// FIXTURE
type TestCheck struct {
	sync.Mutex
	doErr  bool
	hasRun bool
	id     string
	done   chan interface{}
}

func newTestCheck(doErr bool, id string) *TestCheck {
	return &TestCheck{
		doErr: doErr,
		id:    id,
		done:  make(chan interface{}, 1),
	}
}

func (c *TestCheck) String() string                                             { return "TestCheck" }
func (c *TestCheck) Version() string                                            { return "" }
func (c *TestCheck) ConfigSource() string                                       { return "" }
func (c *TestCheck) Stop()                                                      {}
func (c *TestCheck) Configure(integration.Data, integration.Data, string) error { return nil }
func (c *TestCheck) Interval() time.Duration                                    { return 1 }
func (c *TestCheck) Run() error {
	c.Lock()
	defer c.Unlock()
	defer func() { close(c.done) }()
	if c.doErr {
		msg := "A tremendous error occurred."
		return errors.New(msg)
	}

	c.hasRun = true
	return nil
}
func (c *TestCheck) ID() check.ID {
	c.Lock()
	defer c.Unlock()
	return check.ID(fmt.Sprintf("%s:%s", c.String(), c.id))
}
func (c *TestCheck) GetWarnings() []error                      { return nil }
func (c *TestCheck) GetMetricStats() (map[string]int64, error) { return make(map[string]int64), nil }
func (c *TestCheck) HasRun() bool {
	c.Lock()
	defer c.Unlock()
	return c.hasRun
}

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
	r := NewRunner()
	c1 := newTestCheck(false, "1")
	c2 := newTestCheck(true, "2")

	r.pending <- c1
	r.pending <- c2

	select {
	case <-c1.done:
	case <-time.After(1 * time.Second):
		require.Fail(t, "Check hasn't run 1 second after being scheduled")
	}
	assert.True(t, c1.HasRun())
	r.Stop()

	// fake a check is already running
	r = NewRunner()
	c3 := newTestCheck(false, "3")
	r.runningChecks[c3.ID()] = c3
	r.pending <- c3
	// wait to be sure the worker tried to run the check
	time.Sleep(100 * time.Millisecond)
	assert.False(t, c3.HasRun())
}

func TestLogging(t *testing.T) {
	defaultFrequency := config.Datadog.GetInt64("logging_frequency")
	config.Datadog.SetDefault("logging_frequency", int64(20))
	defer config.Datadog.SetDefault("logging_frequency", defaultFrequency)

	r := NewRunner()
	c := newTestCheck(false, "1")
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

	c1 := newTestCheck(false, "1")
	r.runningChecks[c1.ID()] = c1
	err = r.StopCheck(c1.ID())
	assert.Nil(t, err)

	c2 := &TimingoutCheck{TestCheck: *newTestCheck(false, "2")}
	r.runningChecks[c2.ID()] = c2
	err = r.StopCheck(c2.ID())
	assert.Equal(t, "timeout during stop operation on check id TestCheck:2", err.Error())
}
