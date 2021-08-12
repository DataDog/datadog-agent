// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/tracker"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type testCheck struct {
	check.StubCheck
	doErr       bool
	doWarn      bool
	id          string
	longRunning bool
	t           *testing.T
	runFunc     func(id check.ID)
	runCount    uint64
}

func (c *testCheck) ID() check.ID   { return check.ID(c.id) }
func (c *testCheck) String() string { return check.IDToCheckName(c.ID()) }
func (c *testCheck) RunCount() int  { return int(c.runCount) }

func (c *testCheck) Interval() time.Duration {
	if c.longRunning {
		return 0
	}

	return 123
}

func (c *testCheck) GetWarnings() []error {
	if c.doWarn {
		return []error{fmt.Errorf("Warning")}
	}

	return []error{}
}

func (c *testCheck) Run() error {
	if c.runFunc != nil {
		c.runFunc(c.ID())
	}

	atomic.AddUint64(&c.runCount, 1)

	if c.doErr {
		return fmt.Errorf("myerror")
	}

	return nil
}

// Helpers

func newCheck(t *testing.T, id string, doErr bool, runFunc func(check.ID)) *testCheck {
	return &testCheck{
		doErr:   doErr,
		t:       t,
		id:      id,
		runFunc: runFunc,
	}
}

func assertErrorCount(t *testing.T, c check.Check, count int) {
	stats, found := expvars.CheckStats(c.ID())
	require.True(t, found)
	assert.Equal(t, count, int(stats.TotalErrors))
}

// Tests

func TestWorkerInit(t *testing.T) {
	checksTracker := &tracker.RunningChecksTracker{}
	pendingChecksChan := make(chan check.Check, 1)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	_, err := NewWorker(1, 2, nil, checksTracker, mockShouldAddStatsFunc)
	require.NotNil(t, err)

	_, err = NewWorker(1, 2, pendingChecksChan, nil, mockShouldAddStatsFunc)
	require.NotNil(t, err)

	_, err = NewWorker(1, 2, pendingChecksChan, checksTracker, nil)
	require.NotNil(t, err)

	worker, err := NewWorker(1, 2, pendingChecksChan, checksTracker, mockShouldAddStatsFunc)
	assert.Nil(t, err)
	assert.NotNil(t, worker)
}

func TestWorker(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	var wg sync.WaitGroup

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	testCheck1 := newCheck(t, "testing:123", false, nil)
	testCheck2 := newCheck(t, "testing2:234", false, nil)

	upperTimeStatLimit := time.Now().Add(-1 * time.Second)

	// This closure ensures that the mid-run worker state is correct too
	observerAssertFunc := func(id check.ID) {
		assert.Equal(t, 2, testCheck1.RunCount())
		assert.Equal(t, 1, testCheck2.RunCount())

		assert.Equal(t, 2, len(expvars.GetCheckStats()))
		_, found := expvars.CheckStats(id)
		assert.False(t, found)

		assert.Equal(t, 1, int(expvars.GetRunningCheckCount()))
		assert.Equal(t, 1, len(checksTracker.RunningChecks()))
		assert.NotNil(t, checksTracker.RunningChecks()[id])

		assert.False(t, expvars.GetRunningStats(id).IsZero())
		assert.True(t, expvars.GetRunningStats(id).After(upperTimeStatLimit))
		assert.True(t, expvars.GetRunningStats(id).Before(time.Now().Add(1*time.Second)))
	}
	observerTestCheck := newCheck(t, "observer:123", false, observerAssertFunc)

	pendingChecksChan <- testCheck1
	pendingChecksChan <- testCheck2
	pendingChecksChan <- testCheck1
	pendingChecksChan <- observerTestCheck
	pendingChecksChan <- testCheck2
	pendingChecksChan <- testCheck1
	close(pendingChecksChan)

	worker, err := NewWorker(100, 200, pendingChecksChan, checksTracker, mockShouldAddStatsFunc)
	require.Nil(t, err)

	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run()
	}()

	wg.Wait()

	assert.Equal(t, 3, testCheck1.RunCount())
	assert.Equal(t, 2, testCheck2.RunCount())
	assert.Equal(t, 1, observerTestCheck.RunCount())

	assert.Equal(t, 3, len(expvars.GetCheckStats()))
	for _, expectedCheck := range []check.Check{
		testCheck1,
		testCheck2,
		observerTestCheck,
	} {
		_, found := expvars.CheckStats(expectedCheck.ID())
		assert.True(t, found)

		assert.True(t, expvars.GetRunningStats(expectedCheck.ID()).IsZero())
	}

	assert.Equal(t, 0, int(expvars.GetRunningCheckCount()))
	assert.Equal(t, 0, len(checksTracker.RunningChecks()))
}

func TestWorkerErrorAndWarningHandling(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	var wg sync.WaitGroup

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	config.Datadog.Set("hostname", "myhost")

	testCheck1 := newCheck(t, "testing:123", true, nil)
	testCheck2 := newCheck(t, "testing2:234", true, nil)
	testCheck3 := newCheck(t, "testing3:345", false, nil)

	for _, c := range []check.Check{
		testCheck1,
		testCheck2,
		testCheck3,
		testCheck3,
		testCheck1,
		testCheck1,
	} {
		pendingChecksChan <- c
	}
	close(pendingChecksChan)

	worker, err := NewWorker(100, 200, pendingChecksChan, checksTracker, mockShouldAddStatsFunc)
	require.Nil(t, err)

	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run()
	}()

	wg.Wait()

	assert.Equal(t, 3, testCheck1.RunCount())
	assert.Equal(t, 1, testCheck2.RunCount())
	assert.Equal(t, 2, testCheck3.RunCount())

	assertErrorCount(t, testCheck1, 3)
	assertErrorCount(t, testCheck2, 1)
	assertErrorCount(t, testCheck3, 0)

	assert.Equal(t, 6, int(expvars.GetRunsCount()))
	assert.Equal(t, 4, int(expvars.GetErrorsCount()))
	assert.Equal(t, 0, int(expvars.GetWarningsCount()))
}

func TestWorkerConcurrentCheckScheduling(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	config.Datadog.Set("hostname", "myhost")

	testCheck := newCheck(t, "testing:123", true, nil)

	// Make it appear as though the check is already running
	checksTracker.AddCheck(testCheck)

	pendingChecksChan <- testCheck
	close(pendingChecksChan)

	worker, err := NewWorker(100, 200, pendingChecksChan, checksTracker, mockShouldAddStatsFunc)
	require.Nil(t, err)

	worker.Run()

	assert.Equal(t, 0, testCheck.RunCount())
	assert.Equal(t, 0, int(expvars.GetRunsCount()))
	assert.Equal(t, 0, int(expvars.GetErrorsCount()))
	assert.Equal(t, 0, int(expvars.GetWarningsCount()))
}

func TestWorkerStatsAddition(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)

	shouldAddStatsFunc := func(id check.ID) bool {
		if string(id) == "squelched:123" {
			return false
		}
		return true
	}

	config.Datadog.Set("hostname", "myhost")

	longRunningCheckNoErrorNoWarning := &testCheck{
		t:           t,
		id:          "mycheck_noerr_nowarn",
		longRunning: true,
	}

	longRunningCheckWithError := &testCheck{
		t:           t,
		id:          "mycheck_witherr",
		longRunning: true,
		doErr:       true,
	}

	longRunningCheckWithWarnings := &testCheck{
		t:           t,
		id:          "mycheck_withwarn",
		longRunning: true,
		doWarn:      true,
	}
	squelchedStatsCheck := newCheck(t, "squelched:123", false, nil)

	pendingChecksChan <- longRunningCheckNoErrorNoWarning
	pendingChecksChan <- longRunningCheckWithError
	pendingChecksChan <- longRunningCheckWithWarnings
	pendingChecksChan <- squelchedStatsCheck
	close(pendingChecksChan)

	worker, err := NewWorker(100, 200, pendingChecksChan, checksTracker, shouldAddStatsFunc)
	require.Nil(t, err)

	worker.Run()

	for c, statsExpected := range map[check.Check]bool{
		longRunningCheckNoErrorNoWarning: false,
		longRunningCheckWithError:        true,
		longRunningCheckWithWarnings:     true,
		squelchedStatsCheck:              false,
	} {
		_, found := expvars.CheckStats(c.ID())
		assert.True(t, found == statsExpected)
	}
}

func TestWorkerServiceCheckSending(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	var wg sync.WaitGroup

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	goodCheck := newCheck(t, "goodcheck:123", false, nil)
	checkWithError := newCheck(t, "check_witherr:123", true, nil)
	checkWithWarnings := &testCheck{
		t:      t,
		id:     "check_withwarn:123",
		doWarn: true,
	}

	pendingChecksChan <- goodCheck
	pendingChecksChan <- checkWithWarnings
	pendingChecksChan <- checkWithError
	close(pendingChecksChan)

	mockSender := mocksender.NewMockSender("")

	worker, err := newWorkerWithOptions(
		100,
		200,
		pendingChecksChan,
		checksTracker,
		mockShouldAddStatsFunc,
		func() (aggregator.Sender, error) {
			return mockSender, nil
		},
	)
	require.Nil(t, err)

	mockSender.On("Commit").Return().Times(3)
	mockSender.On(
		"ServiceCheck",
		serviceCheckStatusKey,
		metrics.ServiceCheckOK,
		"myhost",
		[]string{"check:goodcheck"},
		"",
	).Return().Times(1)

	mockSender.On(
		"ServiceCheck",
		serviceCheckStatusKey,
		metrics.ServiceCheckWarning,
		"myhost",
		[]string{"check:check_withwarn"},
		"",
	).Return().Times(1)

	mockSender.On(
		"ServiceCheck",
		serviceCheckStatusKey,
		metrics.ServiceCheckCritical,
		"myhost",
		[]string{"check:check_witherr"},
		"",
	).Return().Times(1)

	// Run the worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run()
	}()

	wg.Wait()

	// Quick sanity check
	assert.Equal(t, 3, int(expvars.GetRunsCount()))

	// Go through the expectations
	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Commit", 3)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 3)
}

func TestWorkerSenderNil(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	pendingChecksChan <- newCheck(t, "goodcheck:123", false, nil)
	close(pendingChecksChan)

	worker, err := newWorkerWithOptions(
		100,
		200,
		pendingChecksChan,
		checksTracker,
		mockShouldAddStatsFunc,
		func() (aggregator.Sender, error) {
			return nil, fmt.Errorf("testerr")
		},
	)
	require.Nil(t, err)

	// Implicit assertion that we don't panic
	worker.Run()

	// Quick sanity check
	assert.Equal(t, 1, int(expvars.GetRunsCount()))
}

func TestWorkerServiceCheckSendingLongRunningTasks(t *testing.T) {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")

	checksTracker := tracker.NewRunningChecksTracker()
	pendingChecksChan := make(chan check.Check, 10)
	mockShouldAddStatsFunc := func(id check.ID) bool { return true }

	longRunningCheck := &testCheck{
		t:           t,
		id:          "mycheck",
		longRunning: true,
	}

	pendingChecksChan <- longRunningCheck
	close(pendingChecksChan)

	mockSender := mocksender.NewMockSender("")

	worker, err := newWorkerWithOptions(
		100,
		200,
		pendingChecksChan,
		checksTracker,
		mockShouldAddStatsFunc,
		func() (aggregator.Sender, error) {
			return mockSender, nil
		},
	)
	require.Nil(t, err)

	worker.Run()

	// Quick sanity check
	assert.Equal(t, 1, int(expvars.GetRunsCount()))

	mockSender.AssertNumberOfCalls(t, "Commit", 0)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 0)
}
