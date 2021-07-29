// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/config"
)

/*
type TestCheck struct {
	check.StubCheck
	sync.Mutex
	done   chan interface{}
}

func newTestCheck(doErr bool, id string) *TestCheck {
	return &TestCheck{
		doErr: doErr,
		id:    id,
		done:  make(chan interface{}, 1),
	}
}

func (c *TestCheck) Run() error {
	c.Lock()
	defer c.Unlock()
	defer func() { close(c.done) }()
}

func (c *TestCheck) GetWarnings() []error                 { return nil }
func (c *TestCheck) GetStats() (check.SenderStats, error) { return check.NewSenderStats(), nil }
*/

// Fixtures

type testCheck struct {
	check.StubCheck
	RunLock  sync.Mutex
	StopLock sync.Mutex

	doErr       bool
	doWarn      bool
	id          string
	t           *testing.T
	runFunc     func(id check.ID)
	runCount    uint64
	startedChan chan struct{}
	stopped     bool
}

func (c *testCheck) ID() check.ID   { return check.ID(c.id) }
func (c *testCheck) String() string { return check.IDToCheckName(c.ID()) }
func (c *testCheck) RunCount() int  { return int(c.runCount) }
func (c *testCheck) Stop() {
	c.StopLock.Lock()
	defer c.StopLock.Unlock()

	c.stopped = true
}
func (c *testCheck) IsStopped() bool { return c.stopped }
func (c *testCheck) StartedChan() chan struct{} {
	if c.startedChan == nil {
		c.startedChan = make(chan struct{}, 1)
	}

	return c.startedChan
}

func (c *testCheck) GetWarnings() []error {
	if c.doWarn {
		return []error{fmt.Errorf("Warning")}
	}

	return []error{}
}

func (c *testCheck) Run() error {
	c.StartedChan() <- struct{}{}

	// Block if we have a lock set (for testing delayed processing)
	c.RunLock.Lock()
	defer c.RunLock.Unlock()

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

func assertAsyncWorkerCount(t *testing.T, count int) {
	for idx := 0; idx < 20; idx++ {
		workers := int(expvars.GetWorkerCount())
		if workers == count {
			// This may seem superfluous but we want to ensure that at least one
			// assertion runs in all cases
			require.Equal(t, count, workers)
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(t, count, int(expvars.GetWorkerCount()))
}

func assertAsyncBool(t *testing.T, actualValueFunc func() bool, expectedValue bool) {
	for idx := 0; idx < 20; idx++ {
		actualValue := actualValueFunc()
		if actualValue == expectedValue {
			// This may seem superfluous but we want to ensure that at least one
			// assertion runs in all cases
			require.Equal(t, expectedValue, actualValue)
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(t, expectedValue, actualValueFunc())
}

func testSetUp() {
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")
}

// Tests

func TestNewRunner(t *testing.T) {
	testSetUp()
	config.Datadog.Set("check_runners", "3")

	r := NewRunner()
	require.NotNil(t, r)

	assertAsyncWorkerCount(t, 3)

	assert.NotNil(t, r.GetChan())
	r.GetChan() <- newCheck(t, "mycheck:123", false, nil)
}

func TestRunnerAddWorker(t *testing.T) {
	testSetUp()
	config.Datadog.Set("check_runners", "1")

	r := NewRunner()
	require.NotNil(t, r)

	r.AddWorker()
	r.AddWorker()
	r.AddWorker()

	assertAsyncWorkerCount(t, 4)
}

func TestRunnerStaticUpdateNumWorkers(t *testing.T) {
	testSetUp()
	config.Datadog.Set("check_runners", "2")

	r := NewRunner()
	require.NotNil(t, r)

	// Static check runner count should not change anything
	for checks := 0; checks < 30; checks++ {
		r.UpdateNumWorkers(int64(checks))
	}

	assertAsyncWorkerCount(t, 2)
}

func TestRunnerDynamicUpdateNumWorkers(t *testing.T) {
	testSetUp()
	config.Datadog.Set("check_runners", "0")

	testCases := [][]int{
		{0, 10, 4},
		{11, 15, 10},
		{16, 20, 15},
		{21, 25, 20},
		{26, 35, config.MaxNumWorkers},
	}

	for _, testCase := range testCases {
		min, max, expectedWorkers := testCase[0], testCase[1], testCase[2]

		expvars.Reset()

		r := NewRunner()
		require.NotNil(t, r)

		for checks := min; checks <= max; checks++ {
			r.UpdateNumWorkers(int64(checks))
			require.True(t, int(expvars.GetWorkerCount()) <= expectedWorkers)
		}

		assertAsyncWorkerCount(t, expectedWorkers)
	}
}

func TestRunner(t *testing.T) {
	testSetUp()
	numChecks := 10

	checks := make([]*testCheck, numChecks)
	for idx := 0; idx < numChecks; idx++ {
		checks[idx] = newCheck(t, fmt.Sprintf("mycheck_%d:123", idx), false, nil)
	}

	r := NewRunner()
	for idx := 0; idx < numChecks; idx++ {
		r.GetChan() <- checks[idx]
	}

	time.Sleep(150 * time.Millisecond)
	for idx := 0; idx < numChecks; idx++ {
		require.Equal(t, 1, checks[idx].RunCount())
	}
}

func TestRunnerStop(t *testing.T) {
	testSetUp()

	config.Datadog.Set("check_runners", "10")
	numChecks := 8

	checks := make([]*testCheck, numChecks)
	for idx := 0; idx < numChecks; idx++ {
		checks[idx] = newCheck(t, fmt.Sprintf("mycheck_%d:123", idx), false, nil)

		// Make sure they aren't cleared from the running list
		checks[idx].RunLock.Lock()
	}

	r := NewRunner()
	require.NotNil(t, r)
	assertAsyncWorkerCount(t, 10)

	// Queue the checks up
	for idx := 0; idx < numChecks; idx++ {
		r.GetChan() <- checks[idx]
	}

	// Wait until all are "running"
	for idx := 0; idx < numChecks; idx++ {
		<-checks[idx].StartedChan()
	}

	go r.Stop()

	for _, c := range checks {
		assertAsyncBool(t, c.IsStopped, true)
		c.RunLock.Unlock()
	}

	_, ok := <-r.pendingChecksChan
	assert.False(t, ok)

	// Calling Stop on a stopped runner should be a noop
	r.Stop()

	// Ensure that the channel can't be written to anymore
	defer func() {
		require.NotNil(t, recover())
	}()
	r.GetChan() <- newCheck(t, "mycheck:123", false, nil)

	// Ensure that the worker counts are updated
	assertAsyncWorkerCount(t, 0)
}

func TestRunnerStopCheck(t *testing.T) {
	testSetUp()
	config.Datadog.Set("check_runners", "3")

	testCheck := newCheck(t, "mycheck:123", false, nil)
	blockedCheck := newCheck(t, "mycheck2:123", false, nil)

	testCheck.RunLock.Lock()
	blockedCheck.RunLock.Lock()
	blockedCheck.StopLock.Lock()

	r := NewRunner()
	require.NotNil(t, r)

	r.GetChan() <- testCheck
	r.GetChan() <- blockedCheck

	<-testCheck.StartedChan()
	<-blockedCheck.StartedChan()

	require.False(t, testCheck.IsStopped())
	require.False(t, blockedCheck.IsStopped())

	err := r.StopCheck(check.ID("missingid"))
	require.Nil(t, err)

	err = r.StopCheck(testCheck.ID())
	require.Nil(t, err)
	testCheck.RunLock.Unlock()
	assertAsyncBool(t, testCheck.IsStopped, true)

	err = r.StopCheck(blockedCheck.ID())
	require.NotNil(t, err)
	assertAsyncBool(t, blockedCheck.IsStopped, false)

	blockedCheck.RunLock.Unlock()
	blockedCheck.StopLock.Unlock()
	assertAsyncBool(t, blockedCheck.IsStopped, true)
}

// Stop()
// ShouldAddCheckStats
// SetScheduler
// Already running check

/*
func TestWork(t *testing.T) {
	r := NewRunner()
	c1 := newTestCheck(false, "1")
	c2 := newTestCheck(true, "2")

	r.pendingChecksChan <- c1
	r.pendingChecksChan <- c2

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
	r.checksTracker.AddCheck(c3)
	r.pendingChecksChan <- c3
	// wait to be sure the worker tried to run the check
	time.Sleep(100 * time.Millisecond)
	assert.False(t, c3.HasRun())
}

type TimingoutCheck struct {
	testCheck
}

func (tc *TimingoutCheck) Stop() {
	for {
		runtime.Gosched()
	}
}
func (tc *TimingoutCheck) String() string { return "TimeoutTestCheck" }

func TestStopCheck(t *testing.T) {
	config.Datadog.Set("hostname", "myhost")

	r := NewRunner()
	err := r.StopCheck("foo")
	assert.Nil(t, err)

	c1 := newTestCheck(false, "1")
	r.checksTracker.AddCheck(c1)
	err = r.StopCheck(c1.ID())
	assert.Nil(t, err)

	c2 := &TimingoutCheck{TestCheck: *newTestCheck(false, "2")}
	r.checksTracker.AddCheck(c2)
	err = r.StopCheck(c2.ID())
	assert.Equal(t, "timeout during stop operation on check id TestCheck:2", err.Error())
}
*/
