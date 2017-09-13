// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package runner

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

// FIXTURE
type TestCheck struct {
	doErr  bool
	hasRun bool
}

func (c *TestCheck) String() string                                     { return "TestCheck" }
func (c *TestCheck) Stop()                                              {}
func (c *TestCheck) Configure(check.ConfigData, check.ConfigData) error { return nil }
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
	r := NewRunner(1)
	assert.NotNil(t, r.pending)
	assert.NotNil(t, r.runningChecks)
}

func TestStop(t *testing.T) {
	r := NewRunner(1)
	r.Stop()
	_, ok := <-r.pending
	assert.False(t, ok)

	// calling Stop on a stopped runner should be a noop
	r.Stop()
}

func TestGetChan(t *testing.T) {
	r := NewRunner(1)
	assert.NotNil(t, r.GetChan())
}

func TestWork(t *testing.T) {
	r := NewRunner(1)
	c1 := TestCheck{}
	c2 := TestCheck{doErr: true}

	r.pending <- &c1
	r.pending <- &c2
	assert.True(t, c1.hasRun)
	r.Stop()

	// fake a check is already running
	r = NewRunner(1)
	c3 := new(TestCheck)
	r.runningChecks[c3.ID()] = c3
	r.pending <- c3
	// wait to be sure the worker tried to run the check
	time.Sleep(100 * time.Millisecond)
	assert.False(t, c3.hasRun)
}

type TimingoutCheck struct {
	TestCheck
}

func (tc *TimingoutCheck) Stop() {
	for {
	}
}

func TestStopCheck(t *testing.T) {
	r := NewRunner(1)
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
