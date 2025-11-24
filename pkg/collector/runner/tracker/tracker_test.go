// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tracker

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

type testCheck struct {
	stub.StubCheck
	id string
}

func (c *testCheck) ID() checkid.ID { return checkid.ID(c.id) }
func (c *testCheck) String() string { return checkid.IDToCheckName(c.ID()) }

func newTestCheck(id string) *testCheck {
	return &testCheck{id: id}
}

func TestRunningChecksTracker(t *testing.T) {
	tracker := NewRunningChecksTracker()

	// Test simple addition of checks

	_, found := tracker.Check("mycheck")
	require.False(t, found)

	tracker.AddCheck(newTestCheck("somecheck"))
	tracker.AddCheck(newTestCheck("someothercheck"))
	_, found = tracker.Check("mycheck")
	require.False(t, found)

	expectedCheck := newTestCheck("mycheck")
	tracker.AddCheck(expectedCheck)
	tracker.AddCheck(newTestCheck("yetanothercheck"))

	actualCheck, found := tracker.Check(expectedCheck.ID())
	assert.True(t, found)
	assert.Equal(t, actualCheck, expectedCheck)

	// Test simple deletion of checks

	tracker.DeleteCheck("somecheck")
	actualCheck, found = tracker.Check(expectedCheck.ID())
	require.True(t, found)
	assert.Equal(t, actualCheck, expectedCheck)

	tracker.DeleteCheck(actualCheck.ID())
	_, found = tracker.Check(expectedCheck.ID())
	assert.False(t, found)
}

func TestRunningChecksTrackerAddAndDeleteLocking(t *testing.T) {
	tracker := NewRunningChecksTracker()

	var wg sync.WaitGroup

	canaryCheck := newTestCheck("canary")
	tracker.AddCheck(canaryCheck)

	start := make(chan struct{})

	for i := 0; i < 500; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))
			<-start
			tracker.AddCheck(testCheck)
			tracker.DeleteCheck(testCheck.ID())
		}(i)
	}

	close(start)

	wg.Wait()

	assert.Equal(t, len(tracker.RunningChecks()), 1)

	actualCheck, found := tracker.Check(canaryCheck.ID())
	require.True(t, found)
	assert.Equal(t, actualCheck, canaryCheck)
}

func TestRunningChecksTrackerWithRunningChecks(t *testing.T) {
	tracker := NewRunningChecksTracker()

	checks := make(map[checkid.ID]check.Check)

	for idx := 0; idx < 50; idx++ {
		testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))
		tracker.AddCheck(testCheck)
		checks[testCheck.ID()] = testCheck
	}

	loopFunc := func(actualChecks map[checkid.ID]check.Check) {
		for _, actualCheck := range actualChecks {
			expectedCheck, found := checks[actualCheck.ID()]

			assert.True(t, found)
			assert.Equal(t, expectedCheck, actualCheck)

			delete(checks, expectedCheck.ID())

			// This is to check that we don't use the internal map as a param
			delete(actualChecks, actualCheck.ID())
		}
	}

	tracker.WithRunningChecks(loopFunc)

	assert.Equal(t, 0, len(checks))
	assert.Equal(t, 50, len(tracker.RunningChecks()))
}

func TestRunningChecksTrackerRunningChecks(t *testing.T) {
	tracker := NewRunningChecksTracker()
	checks := make([]check.Check, 0)

	for idx := 0; idx < 50; idx++ {
		testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))
		tracker.AddCheck(testCheck)

		checks = append(checks, testCheck)
	}

	runningChecks := tracker.RunningChecks()
	assert.Equal(t, len(runningChecks), 50)

	for _, expectedCheck := range checks {
		actualCheck, found := runningChecks[expectedCheck.ID()]
		assert.True(t, found)
		assert.Equal(t, expectedCheck, actualCheck)
	}
}

func TestRunningChecksTrackerRunningChecksValueClone(t *testing.T) {
	tracker := NewRunningChecksTracker()

	for idx := 0; idx < 50; idx++ {
		testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))
		tracker.AddCheck(testCheck)
	}

	runningChecks := tracker.RunningChecks()
	assert.Equal(t, len(runningChecks), 50)

	canaryCheck := newTestCheck("canary")
	runningChecks[canaryCheck.ID()] = canaryCheck

	newRunningchecks := tracker.RunningChecks()
	assert.Equal(t, len(newRunningchecks), 50)

	_, found := newRunningchecks[canaryCheck.ID()]
	assert.False(t, found)
}

func TestRunningChecksTrackerWithCheck(t *testing.T) {
	tracker := NewRunningChecksTracker()

	var wg sync.WaitGroup

	start := make(chan struct{})

	for i := 0; i < 500; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			runCount := 0

			testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))
			tracker.AddCheck(testCheck)

			closureFunc := func(c check.Check) {
				assert.Equal(t, c, testCheck)
				runCount++
			}

			<-start

			// Ensure closure called when check found
			found := tracker.WithCheck(testCheck.ID(), closureFunc)
			assert.True(t, found)
			assert.Equal(t, 1, runCount)

			tracker.DeleteCheck(testCheck.ID())

			// Ensure closure not called when check not found
			found = tracker.WithCheck(testCheck.ID(), closureFunc)
			assert.False(t, found)
			assert.Equal(t, 1, runCount)
		}(i)
	}

	close(start)

	wg.Wait()
}
