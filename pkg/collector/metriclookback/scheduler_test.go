// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	aggregatorsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestShadowSchedulerLoadsCheckWithMappedSenderIdentityAndRunsOnTick(t *testing.T) {
	expvars.Reset()
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	senderManager := &recordingSenderManager{}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return senderManager },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      time.Second,
		NewTicker:        tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))

	assert.Equal(t, sourceConfig.SourceConfig, loader.loadedConfig)
	assert.Equal(t, sourceConfig.Instance, loader.loadedInstance)
	assert.Equal(t, sourceConfig.InstanceIndex, loader.loadedInstanceIndex)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.GetSenderIDs())

	tickers.TickAndWait(t, 0)
	require.Eventually(t, func() bool {
		return loader.check.RunCount() == 1
	}, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		stats, found := expvars.CheckStats(sourceConfig.ShadowCheckID)
		return found && stats.TotalRuns == 1
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, int(expvars.GetRunsCount()))
}

func TestShadowSchedulerDoesNotRescheduleExistingShadowCheck(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      time.Second,
		NewTicker:        tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))

	assert.Equal(t, 1, loader.LoadCount())
	assert.Len(t, tickers.Tickers(), 1)
}

func TestShadowSchedulerSharesOneRunnerAcrossShadowChecks(t *testing.T) {
	first := newTestShadowConfig()
	second := newTestShadowConfig()
	second.InstanceIndex = 1
	second.Instance = integration.Data("name: second\n")
	second.SourceCheckID = checkid.ID("cpu:second")
	second.ShadowCheckID = checkid.ID("cpu:second:shadow")

	loader := &sequencedShadowLoader{checks: []*testShadowCheck{
		newTestShadowCheck(first.SourceCheckID),
		newTestShadowCheck(second.SourceCheckID),
	}}
	tickers := &manualTickerFactory{}
	newRunner := newTestShadowRunnerFactory(t)
	runnerCalls := 0
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner: func(scheduled runner.ScheduledChecks) ShadowRunner {
			runnerCalls++
			return newRunner(scheduled)
		},
		Interval:    time.Second,
		StopTimeout: time.Second,
		NewTicker:   tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{first, second}))

	assert.Equal(t, 1, runnerCalls)
}

func TestShadowSchedulerConcurrentScheduleKeepsOneShadowCheck(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &blockingShadowLoader{
		checks: []*testShadowCheck{
			newTestShadowCheck(sourceConfig.SourceCheckID),
			newTestShadowCheck(sourceConfig.SourceCheckID),
		},
		ready:   make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	senderManager := &recordingSenderManager{}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return senderManager },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      5 * time.Second,
		NewTicker:        tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	var scheduleWG sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		scheduleWG.Add(1)
		go func() {
			defer scheduleWG.Done()
			errs <- scheduler.Schedule([]ShadowConfig{sourceConfig})
		}()
	}

	for i := 0; i < 2; i++ {
		select {
		case <-loader.ready:
		case <-time.After(time.Second):
			t.Fatal("expected both schedule calls to reach load")
		}
	}
	start := time.Now()
	close(loader.release)
	scheduleWG.Wait()
	elapsed := time.Since(start)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Len(t, tickers.Tickers(), 2)
	assert.Len(t, senderManager.DestroySenderIDs(), 1)
	assert.Less(t, elapsed, 500*time.Millisecond)
}

func TestShadowSchedulerDelaysOverlappingTicksOnDedicatedRunner(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	shadowCheck := newTestShadowCheck(sourceConfig.SourceCheckID)
	shadowCheck.blockRuns = true
	loader := &testShadowLoader{check: shadowCheck}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      time.Second,
		NewTicker:        tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))

	tickers.TickAndWait(t, 0)
	require.Eventually(t, func() bool {
		return shadowCheck.RunCount() == 1
	}, time.Second, 10*time.Millisecond)

	tickers.TickAndWait(t, 0)
	assert.Equal(t, 1, shadowCheck.RunCount())

	shadowCheck.UnblockRun()
	require.Eventually(t, func() bool {
		return shadowCheck.RunCount() == 2
	}, time.Second, 10*time.Millisecond)
}

func TestShadowSchedulerRecoversRunPanics(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	shadowCheck := newTestShadowCheck(sourceConfig.SourceCheckID)
	shadowCheck.runFunc = func() error {
		panic("boom")
	}
	loader := &testShadowLoader{check: shadowCheck}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      time.Second,
		NewTicker:        tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	tickers.TickAndWait(t, 0)

	require.Eventually(t, func() bool {
		stats, found := expvars.CheckStats(sourceConfig.ShadowCheckID)
		return found && stats.TotalErrors == 1
	}, time.Second, 10*time.Millisecond)
	runStats, found := expvars.CheckStats(sourceConfig.ShadowCheckID)
	require.True(t, found)
	assert.Contains(t, runStats.LastError, "check panicked: boom")
	assert.False(t, shadowCheck.IsRunning())
}

func TestShadowSchedulerUnscheduleCancelsActiveRunAndDestroysSender(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	var runCtx context.Context
	shadowCheck := newTestShadowCheck(sourceConfig.SourceCheckID)
	shadowCheck.runFunc = func() error {
		<-runCtx.Done()
		return runCtx.Err()
	}
	loader := &testShadowLoader{check: shadowCheck}
	senderManager := &recordingSenderManager{}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(ctx context.Context) aggregatorsender.SenderManager {
			runCtx = ctx
			return senderManager
		},
		NewRunner:   newTestShadowRunnerFactory(t),
		Interval:    time.Second,
		StopTimeout: 200 * time.Millisecond,
		NewTicker:   tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	tickers.TickAndWait(t, 0)
	require.Eventually(t, func() bool {
		return shadowCheck.RunCount() == 1
	}, time.Second, 10*time.Millisecond)

	start := time.Now()
	require.NoError(t, scheduler.Unschedule([]ShadowConfig{sourceConfig}))

	assert.Less(t, time.Since(start), time.Second)
	assert.True(t, shadowCheck.StopCalled())
	assert.True(t, shadowCheck.CancelCalled())
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
}

func TestShadowSchedulerUnscheduleIsBoundedWhenRunDoesNotExit(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	shadowCheck := newTestShadowCheck(sourceConfig.SourceCheckID)
	unblock := make(chan struct{})
	shadowCheck.runFunc = func() error {
		<-unblock
		return nil
	}
	loader := &testShadowLoader{check: shadowCheck}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      50 * time.Millisecond,
		NewTicker:        tickers.NewTicker,
	})

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	tickers.TickAndWait(t, 0)
	require.Eventually(t, func() bool {
		return shadowCheck.RunCount() == 1
	}, time.Second, 10*time.Millisecond)

	start := time.Now()
	err := scheduler.Unschedule([]ShadowConfig{sourceConfig})
	elapsed := time.Since(start)
	close(unblock)

	require.NoError(t, err)
	assert.Less(t, elapsed, 500*time.Millisecond)
}

func TestShadowSchedulerUnscheduleCallsCancelWhenStopTimesOut(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	shadowCheck := newTestShadowCheck(sourceConfig.SourceCheckID)
	unblockStop := make(chan struct{})
	shadowCheck.stopFunc = func() {
		<-unblockStop
	}
	defer close(unblockStop)

	loader := &testShadowLoader{check: shadowCheck}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      50 * time.Millisecond,
		NewTicker:        tickers.NewTicker,
	})

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	err := scheduler.Unschedule([]ShadowConfig{sourceConfig})

	require.Error(t, err)
	assert.True(t, shadowCheck.StopCalled())
	assert.True(t, shadowCheck.CancelCalled())
}

func TestShadowSchedulerUnscheduleUsesConfigDigestAndInstanceIndex(t *testing.T) {
	first := newTestShadowConfig()
	second := newTestShadowConfig()
	second.InstanceIndex = 1
	second.Instance = integration.Data("name: second\n")
	second.SourceCheckID = checkid.ID("cpu:second")
	second.ShadowCheckID = checkid.ID("cpu:second:shadow")

	loader := &sequencedShadowLoader{checks: []*testShadowCheck{
		newTestShadowCheck(first.SourceCheckID),
		newTestShadowCheck(second.SourceCheckID),
	}}
	tickers := &manualTickerFactory{}
	firstSenderManager := &recordingSenderManager{}
	secondSenderManager := &recordingSenderManager{}
	senderManagers := []*recordingSenderManager{firstSenderManager, secondSenderManager}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager {
			sm := senderManagers[0]
			senderManagers = senderManagers[1:]
			return sm
		},
		NewRunner:   newTestShadowRunnerFactory(t),
		Interval:    time.Second,
		StopTimeout: time.Second,
		NewTicker:   tickers.NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{first, second}))
	require.NoError(t, scheduler.Unschedule([]ShadowConfig{first}))

	tickers.TickAndWait(t, 1)
	require.Eventually(t, func() bool {
		return loader.checks[1].RunCount() == 1
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, []checkid.ID{first.ShadowCheckID}, firstSenderManager.DestroySenderIDs())
	assert.Empty(t, secondSenderManager.DestroySenderIDs())
}

func TestShadowSchedulerDoesNotRunOrRecordStatsAfterUnschedule(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	shadowCheck := newTestShadowCheck(sourceConfig.SourceCheckID)
	unblock := make(chan struct{})
	shadowCheck.runFunc = func() error {
		<-unblock
		return nil
	}
	loader := &testShadowLoader{check: shadowCheck}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      50 * time.Millisecond,
		NewTicker:        tickers.NewTicker,
	})

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	tickers.TickAndWait(t, 0)
	require.Eventually(t, func() bool {
		return shadowCheck.RunCount() == 1
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, scheduler.Unschedule([]ShadowConfig{sourceConfig}))
	_, found := expvars.CheckStats(sourceConfig.ShadowCheckID)
	assert.False(t, found)

	close(unblock)
	require.Eventually(t, func() bool {
		return !shadowCheck.IsRunning()
	}, time.Second, 10*time.Millisecond)

	tickers.Tick(0)
	assert.Equal(t, 1, shadowCheck.RunCount())
	_, found = expvars.CheckStats(sourceConfig.ShadowCheckID)
	assert.False(t, found)
}

func TestShadowSchedulerLoadErrorCleansUpShadowSenderAndCancelsContext(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	senderManager := &recordingSenderManager{}
	var senderCtx context.Context
	loader := &failingShadowLoader{sourceID: sourceConfig.SourceCheckID}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(ctx context.Context) aggregatorsender.SenderManager {
			senderCtx = ctx
			return senderManager
		},
		NewRunner:   newTestShadowRunnerFactory(t),
		Interval:    time.Second,
		StopTimeout: time.Second,
		NewTicker:   (&manualTickerFactory{}).NewTicker,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	err := scheduler.Schedule([]ShadowConfig{sourceConfig})

	require.Error(t, err)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.GetSenderIDs())
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
	select {
	case <-senderCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected load failure to cancel sender context")
	}
}

func TestShadowSchedulerStopPreventsInFlightLoadFromStarting(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &blockingShadowLoader{
		checks:  []*testShadowCheck{newTestShadowCheck(sourceConfig.SourceCheckID)},
		ready:   make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	senderManager := &recordingSenderManager{}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return senderManager },
		NewRunner:        newTestShadowRunnerFactory(t),
		Interval:         time.Second,
		StopTimeout:      time.Second,
		NewTicker:        tickers.NewTicker,
	})

	errs := make(chan error, 1)
	go func() {
		errs <- scheduler.Schedule([]ShadowConfig{sourceConfig})
	}()

	select {
	case <-loader.ready:
	case <-time.After(time.Second):
		t.Fatal("expected schedule to reach load")
	}

	require.NoError(t, scheduler.Stop())
	close(loader.release)

	require.Eventually(t, func() bool {
		return len(errs) == 1
	}, time.Second, 10*time.Millisecond)
	require.ErrorIs(t, <-errs, errShadowSchedulerStopped)
	assert.Len(t, tickers.Tickers(), 1)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())

	tickers.Tick(0)
	assert.Equal(t, 0, loader.checks[0].RunCount())
}

func TestShadowSchedulerStopStopsAllShadowChecks(t *testing.T) {
	first := newTestShadowConfig()
	second := newTestShadowConfig()
	second.InstanceIndex = 1
	second.Instance = integration.Data("name: second\n")
	second.SourceCheckID = checkid.ID("cpu:second")
	second.ShadowCheckID = checkid.ID("cpu:second:shadow")

	loader := &sequencedShadowLoader{checks: []*testShadowCheck{
		newTestShadowCheck(first.SourceCheckID),
		newTestShadowCheck(second.SourceCheckID),
	}}
	firstSenderManager := &recordingSenderManager{}
	secondSenderManager := &recordingSenderManager{}
	senderManagers := []*recordingSenderManager{firstSenderManager, secondSenderManager}
	tickers := &manualTickerFactory{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager {
			sm := senderManagers[0]
			senderManagers = senderManagers[1:]
			return sm
		},
		NewRunner:   newTestShadowRunnerFactory(t),
		Interval:    time.Second,
		StopTimeout: time.Second,
		NewTicker:   tickers.NewTicker,
	})

	require.NoError(t, scheduler.Schedule([]ShadowConfig{first, second}))
	require.NoError(t, scheduler.Stop())
	require.NoError(t, scheduler.Stop())

	assert.Equal(t, []checkid.ID{first.ShadowCheckID}, firstSenderManager.DestroySenderIDs())
	assert.Equal(t, []checkid.ID{second.ShadowCheckID}, secondSenderManager.DestroySenderIDs())
	for _, c := range loader.checks {
		assert.True(t, c.StopCalled())
		assert.True(t, c.CancelCalled())
	}
}

func TestShadowSchedulerScheduleAfterStopDoesNotCreateRunner(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	runnerCalls := 0
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return &recordingSenderManager{} },
		NewRunner: func(scheduled runner.ScheduledChecks) ShadowRunner {
			runnerCalls++
			return newTestShadowRunnerFactory(t)(scheduled)
		},
		Interval:    time.Second,
		StopTimeout: time.Second,
		NewTicker:   (&manualTickerFactory{}).NewTicker,
	})

	require.NoError(t, scheduler.Stop())

	err := scheduler.Schedule([]ShadowConfig{sourceConfig})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "shadow scheduler is stopped")
	assert.Equal(t, 0, loader.LoadCount())
	assert.Equal(t, 0, runnerCalls)
}

func newTestShadowConfig() ShadowConfig {
	source := integration.Config{
		Name:       "cpu",
		InitConfig: integration.Data("loader: core\n"),
		Instances:  []integration.Data{integration.Data("name: first\n")},
		Source:     "file:cpu",
		Provider:   "file",
	}
	sourceCheckID := checkid.ID("cpu:first")
	return ShadowConfig{
		SourceConfig:       source,
		Instance:           source.Instances[0],
		InstanceIndex:      0,
		SourceConfigDigest: source.Digest(),
		SourceCheckID:      sourceCheckID,
		ShadowCheckID:      checkid.ID(string(sourceCheckID) + ShadowIDSuffix),
	}
}

type testShadowLoader struct {
	check               *testShadowCheck
	loadedConfig        integration.Config
	loadedInstance      integration.Data
	loadedInstanceIndex int
	loadCount           int
}

func (l *testShadowLoader) LoadInstance(senderManager aggregatorsender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (check.Check, bool, error) {
	l.loadedConfig = config
	l.loadedInstance = instance
	l.loadedInstanceIndex = instanceIndex
	l.loadCount++
	if _, err := senderManager.GetSender(l.check.ID()); err != nil {
		return nil, false, err
	}
	return l.check, true, nil
}

func (l *testShadowLoader) LoadCount() int { return l.loadCount }

type failingShadowLoader struct {
	sourceID checkid.ID
}

func (l *failingShadowLoader) LoadInstance(senderManager aggregatorsender.SenderManager, _ integration.Config, _ integration.Data, _ int) (check.Check, bool, error) {
	_, _ = senderManager.GetSender(l.sourceID)
	return nil, false, errors.New("load failed")
}

type sequencedShadowLoader struct {
	checks []*testShadowCheck
	next   int
}

func (l *sequencedShadowLoader) LoadInstance(senderManager aggregatorsender.SenderManager, _ integration.Config, _ integration.Data, _ int) (check.Check, bool, error) {
	if l.next >= len(l.checks) {
		return nil, false, errors.New("unexpected load")
	}
	c := l.checks[l.next]
	l.next++
	if _, err := senderManager.GetSender(c.ID()); err != nil {
		return nil, false, err
	}
	return c, true, nil
}

type blockingShadowLoader struct {
	mu      sync.Mutex
	checks  []*testShadowCheck
	next    int
	ready   chan struct{}
	release chan struct{}
}

func (l *blockingShadowLoader) LoadInstance(senderManager aggregatorsender.SenderManager, _ integration.Config, _ integration.Data, _ int) (check.Check, bool, error) {
	l.mu.Lock()
	if l.next >= len(l.checks) {
		l.mu.Unlock()
		return nil, false, errors.New("unexpected load")
	}
	c := l.checks[l.next]
	l.next++
	l.mu.Unlock()

	if _, err := senderManager.GetSender(c.ID()); err != nil {
		return nil, false, err
	}
	l.ready <- struct{}{}
	<-l.release
	return c, true, nil
}

type testShadowCheck struct {
	id           checkid.ID
	mu           sync.Mutex
	runCount     int
	running      bool
	blockRuns    bool
	unblockRun   chan struct{}
	stopCalled   bool
	cancelCalled bool
	runFunc      func() error
	stopFunc     func()
	cancelFunc   func()
}

func newTestShadowCheck(id checkid.ID) *testShadowCheck {
	return &testShadowCheck{id: id, unblockRun: make(chan struct{})}
}

func (c *testShadowCheck) Run() error {
	c.mu.Lock()
	c.runCount++
	c.running = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	if c.runFunc != nil {
		return c.runFunc()
	}
	if c.blockRuns {
		<-c.unblockRun
	}
	return nil
}

func (c *testShadowCheck) UnblockRun() { close(c.unblockRun) }

func (c *testShadowCheck) RunCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runCount
}

func (c *testShadowCheck) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *testShadowCheck) Stop() {
	c.mu.Lock()
	c.stopCalled = true
	c.mu.Unlock()
	if c.stopFunc != nil {
		c.stopFunc()
	}
}

func (c *testShadowCheck) Cancel() {
	c.mu.Lock()
	c.cancelCalled = true
	c.mu.Unlock()
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
}

func (c *testShadowCheck) StopCalled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopCalled
}

func (c *testShadowCheck) CancelCalled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancelCalled
}

func (c *testShadowCheck) String() string { return checkid.IDToCheckName(c.id) }
func (c *testShadowCheck) Loader() string { return goCheckLoaderName }
func (c *testShadowCheck) Configure(aggregatorsender.SenderManager, uint64, integration.Data, integration.Data, string, string) error {
	return nil
}
func (c *testShadowCheck) Interval() time.Duration { return 15 * time.Second }
func (c *testShadowCheck) ID() checkid.ID          { return c.id }
func (c *testShadowCheck) GetWarnings() []error    { return nil }
func (c *testShadowCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.NewSenderStats(), nil
}
func (c *testShadowCheck) Version() string          { return "" }
func (c *testShadowCheck) ConfigSource() string     { return "" }
func (c *testShadowCheck) ConfigProvider() string   { return "" }
func (c *testShadowCheck) IsTelemetryEnabled() bool { return false }
func (c *testShadowCheck) InitConfig() string       { return "" }
func (c *testShadowCheck) InstanceConfig() string   { return "" }
func (c *testShadowCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}
func (c *testShadowCheck) IsHASupported() bool { return false }

type manualTickerFactory struct {
	mu      sync.Mutex
	tickers []*manualTicker
}

func (f *manualTickerFactory) NewTicker(time.Duration) ShadowTicker {
	f.mu.Lock()
	defer f.mu.Unlock()
	ticker := &manualTicker{ch: make(chan time.Time, 10)}
	f.tickers = append(f.tickers, ticker)
	return ticker
}

func (f *manualTickerFactory) Tick(index int) {
	f.mu.Lock()
	ticker := f.tickers[index]
	f.mu.Unlock()
	ticker.ch <- time.Now()
}

func (f *manualTickerFactory) TickAndWait(t *testing.T, index int) {
	t.Helper()
	f.Tick(index)
	require.Eventually(t, func() bool {
		f.mu.Lock()
		ticker := f.tickers[index]
		f.mu.Unlock()
		return len(ticker.ch) == 0
	}, time.Second, 10*time.Millisecond)
}

func (f *manualTickerFactory) Tickers() []*manualTicker {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*manualTicker(nil), f.tickers...)
}

type manualTicker struct {
	ch chan time.Time
}

func (t *manualTicker) C() <-chan time.Time { return t.ch }
func (t *manualTicker) Stop()               {}

type recordingSenderManager struct {
	mu               sync.Mutex
	getSenderIDs     []checkid.ID
	destroySenderIDs []checkid.ID
}

func (m *recordingSenderManager) GetSender(id checkid.ID) (aggregatorsender.Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getSenderIDs = append(m.getSenderIDs, id)
	return nil, nil
}
func (m *recordingSenderManager) SetSender(aggregatorsender.Sender, checkid.ID) error { return nil }
func (m *recordingSenderManager) DestroySender(id checkid.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroySenderIDs = append(m.destroySenderIDs, id)
}
func (m *recordingSenderManager) GetDefaultSender() (aggregatorsender.Sender, error) {
	return nil, nil
}

func (m *recordingSenderManager) GetSenderIDs() []checkid.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]checkid.ID(nil), m.getSenderIDs...)
}

func (m *recordingSenderManager) DestroySenderIDs() []checkid.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]checkid.ID(nil), m.destroySenderIDs...)
}

func newTestShadowRunnerFactory(t *testing.T) ShadowRunnerFactory {
	t.Helper()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("check_runners", 1)
	mockConfig.SetInTest("hostname", "myhost")

	return func(scheduled runner.ScheduledChecks) ShadowRunner {
		r := runner.NewRunnerWithOptions(
			&recordingSenderManager{},
			haagentmock.NewMockHaAgent(),
			healthplatformmock.Mock(t),
			runner.Options{
				StatusEmitter: noopStatusEmitter{},
			},
		)
		r.SetScheduler(scheduled)
		return r
	}
}

type noopStatusEmitter struct{}

func (noopStatusEmitter) Emit(context.Context, check.Check, error, []error) {}
