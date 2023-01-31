// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeCheck struct {
	id       int
	period   time.Duration
	runCount int
}

var checkID = 0

func newCheck(period time.Duration) Check {
	checkID++
	return &fakeCheck{id: checkID, period: period}
}

func (c *fakeCheck) ID() string {
	return strconv.Itoa(c.id)
}

func (c *fakeCheck) String() string {
	return "fakecheck:" + strconv.Itoa(c.id)
}

func (c *fakeCheck) Version() string {
	return "0"
}

func (c *fakeCheck) Period() time.Duration {
	return c.period
}

func (c *fakeCheck) Run() error {
	c.runCount++
	return nil
}

func TestScheduler(t *testing.T) {
	checks := []Check{
		newCheck(20 * time.Millisecond),
		newCheck(20 * time.Millisecond),
		newCheck(20 * time.Millisecond),
		newCheck(20 * time.Millisecond),

		newCheck(200 * time.Millisecond),
		newCheck(200 * time.Millisecond),
	}
	s := NewPeriodicScheduler()
	s.StartScheduling(checks)
	time.Sleep(500 * time.Millisecond)
	s.StopScheduling(context.Background())
	checksCopy := make([]fakeCheck, len(checks))
	for i, check := range checks {
		checksCopy[i] = *(check.(*fakeCheck))
		switch check.Period() {
		case 20 * time.Millisecond:
			assert.Greater(t, check.(*fakeCheck).runCount, 20)
		case 200 * time.Millisecond:
			assert.Greater(t, check.(*fakeCheck).runCount, 1)
		default:
			t.Fail()
		}
	}
	time.Sleep(500 * time.Millisecond)
	for i, check := range checks {
		assert.Equal(t, checksCopy[i].runCount, check.(*fakeCheck).runCount)
	}
}
