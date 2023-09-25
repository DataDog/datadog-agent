// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Fixtures

type testCheck struct {
	runCount *atomic.Uint64
	stopped  *atomic.Bool

	stub.StubCheck
	RunLock   sync.Mutex
	StartLock sync.Mutex
	StopLock  sync.Mutex

	doErr       bool
	doWarn      bool
	id          string
	t           *testing.T
	runFunc     func(id checkid.ID)
	startedChan chan struct{}
}

func (c *testCheck) ID() checkid.ID { return checkid.ID(c.id) }
func (c *testCheck) String() string { return checkid.IDToCheckName(c.ID()) }
func (c *testCheck) RunCount() int  { return int(c.runCount.Load()) }
func (c *testCheck) Stop() {
	c.StopLock.Lock()
	defer c.StopLock.Unlock()

	c.stopped.Store(true)
}
func (c *testCheck) IsStopped() bool { return c.stopped.Load() }
func (c *testCheck) StartedChan() chan struct{} {
	c.StartLock.Lock()
	defer c.StartLock.Unlock()

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

	c.runCount.Inc()

	if c.doErr {
		return fmt.Errorf("myerror")
	}

	return nil
}

// Helpers

func newCheck(t *testing.T, id string, doErr bool, runFunc func(checkid.ID)) *testCheck {
	return &testCheck{
		runCount: atomic.NewUint64(0),
		stopped:  atomic.NewBool(false),
		doErr:    doErr,
		t:        t,
		id:       id,
		runFunc:  runFunc,
	}
}

func newScheduler() *scheduler.Scheduler {
	return scheduler.NewScheduler(nil)
}

func assertAsyncWorkerCount(t *testing.T, count int) {
	for idx := 0; idx < 75; idx++ {
		workers := expvars.GetWorkerCount()
		if workers == count {
			// This may seem superfluous but we want to ensure that at least one
			// assertion runs in all cases
			require.Equal(t, count, workers)
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(t, count, expvars.GetWorkerCount())
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

func testSetUp(t *testing.T) {
	assertAsyncWorkerCount(t, 0)
	expvars.Reset()
	config.Datadog.Set("hostname", "myhost")
}

// Tests

func TestNewRunner(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "3")

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

	assertAsyncWorkerCount(t, 3)

	assert.NotNil(t, r.GetChan())
	r.GetChan() <- newCheck(t, "mycheck:123", false, nil)
}

func TestRunnerAddWorker(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "1")

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

	r.AddWorker()
	r.AddWorker()
	r.AddWorker()

	assertAsyncWorkerCount(t, 4)
}

func TestRunnerStaticUpdateNumWorkers(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "2")

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer func() {
		r.Stop()
		assertAsyncWorkerCount(t, 0)
	}()

	// Static check runner count should not change anything
	for checks := 0; checks < 30; checks++ {
		r.UpdateNumWorkers(int64(checks))
	}

	assertAsyncWorkerCount(t, 2)
}

func TestRunnerDynamicUpdateNumWorkers(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "0")

	testCases := [][]int{
		{0, 10, 4},
		{11, 15, 10},
		{16, 20, 15},
		{21, 25, 20},
		{26, 35, config.MaxNumWorkers},
	}

	for _, testCase := range testCases {
		assertAsyncWorkerCount(t, 0)
		min, max, expectedWorkers := testCase[0], testCase[1], testCase[2]

		r := NewRunner(aggregator.NewNoOpSenderManager())
		require.NotNil(t, r)

		for checks := min; checks <= max; checks++ {
			r.UpdateNumWorkers(int64(checks))
			require.True(t, expvars.GetWorkerCount() <= expectedWorkers)
		}

		assertAsyncWorkerCount(t, expectedWorkers)
		r.Stop()
	}
}

func TestRunner(t *testing.T) {
	testSetUp(t)
	numChecks := 10

	checks := make([]*testCheck, numChecks)
	for idx := 0; idx < numChecks; idx++ {
		checks[idx] = newCheck(t, fmt.Sprintf("mycheck_%d:123", idx), false, nil)
	}

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

	for idx := 0; idx < numChecks; idx++ {
		r.GetChan() <- checks[idx]
	}

	time.Sleep(150 * time.Millisecond)
	for idx := 0; idx < numChecks; idx++ {
		require.Equal(t, 1, checks[idx].RunCount())
	}
}

func TestRunnerStop(t *testing.T) {
	testSetUp(t)

	config.Datadog.Set("check_runners", "10")
	numChecks := 8

	checks := make([]*testCheck, numChecks)
	for idx := 0; idx < numChecks; idx++ {
		checks[idx] = newCheck(t, fmt.Sprintf("mycheck_%d:123", idx), false, nil)

		// Make sure they aren't cleared from the running list
		checks[idx].RunLock.Lock()
	}

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

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

func TestRunnerStopWithStuckCheck(t *testing.T) {
	testSetUp(t)

	config.Datadog.Set("check_runners", "10")
	numChecks := 8

	checks := make([]*testCheck, numChecks)
	for idx := 0; idx < numChecks; idx++ {
		checks[idx] = newCheck(t, fmt.Sprintf("mycheck_%d:123", idx), false, nil)

		// Make sure they aren't cleared from the running list
		checks[idx].RunLock.Lock()
	}

	// Create a check that will block stopping
	blockedCheck := newCheck(t, "blockedcheck:123", false, nil)
	blockedCheck.RunLock.Lock()
	blockedCheck.StopLock.Lock()

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

	assertAsyncWorkerCount(t, 10)

	// Queue the checks up
	for idx := 0; idx < numChecks; idx++ {
		r.GetChan() <- checks[idx]
	}
	r.GetChan() <- blockedCheck

	// Wait until all are "running"
	for idx := 0; idx < numChecks; idx++ {
		<-checks[idx].StartedChan()
	}
	<-blockedCheck.StartedChan()

	r.Stop()

	for _, c := range checks {
		assertAsyncBool(t, c.IsStopped, true)
		c.RunLock.Unlock()
	}

	assertAsyncBool(t, blockedCheck.IsStopped, false)

	// Calling Stop on a stopped runner should be a noop
	r.Stop()

	assertAsyncWorkerCount(t, 1)

	// Release the last worker by letting the blocked check stop
	blockedCheck.StopLock.Unlock()
	blockedCheck.RunLock.Unlock()
	assertAsyncWorkerCount(t, 0)
}

func TestRunnerStopCheck(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "3")

	testCheck := newCheck(t, "mycheck:123", false, nil)
	blockedCheck := newCheck(t, "mycheck2:123", false, nil)

	testCheck.RunLock.Lock()
	blockedCheck.RunLock.Lock()
	blockedCheck.StopLock.Lock()

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer func() {
		r.Stop()
		assertAsyncWorkerCount(t, 0)
	}()

	r.GetChan() <- testCheck
	r.GetChan() <- blockedCheck

	<-testCheck.StartedChan()
	<-blockedCheck.StartedChan()

	require.False(t, testCheck.IsStopped())
	require.False(t, blockedCheck.IsStopped())

	err := r.StopCheck(checkid.ID("missingid"))
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

	// Implicit check. Ensure that we can try to stop a stopped check again
	err = r.StopCheck(testCheck.ID())
	require.Nil(t, err)
}

func TestRunnerScheduler(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "3")

	sched1 := newScheduler()
	sched2 := newScheduler()

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

	require.Nil(t, r.getScheduler())

	r.SetScheduler(sched1)
	require.Equal(t, sched1, r.getScheduler())

	r.SetScheduler(sched2)
	require.Equal(t, sched2, r.getScheduler())
}

func TestRunnerShouldAddCheckStats(t *testing.T) {
	testSetUp(t)
	config.Datadog.Set("check_runners", "3")

	testCheck := newCheck(t, "test", false, nil)
	sched := newScheduler()

	r := NewRunner(aggregator.NewNoOpSenderManager())
	require.NotNil(t, r)
	defer r.Stop()

	require.Nil(t, r.getScheduler())

	// Unconditionally, if there's no scheduler, we should add the stats
	require.True(t, r.ShouldAddCheckStats(testCheck.ID()))

	r.SetScheduler(sched)
	require.Equal(t, sched, r.getScheduler())

	// If there's a scheduler but the check isn't scheduled, don't add the stats
	require.False(t, r.ShouldAddCheckStats(testCheck.ID()))

	sched.Enter(testCheck)
	// If there's a scheduler with scheduled check, add the stats
	require.True(t, r.ShouldAddCheckStats(testCheck.ID()))
}
