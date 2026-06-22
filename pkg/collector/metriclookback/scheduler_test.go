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
	aggregatorsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

func TestShadowSchedulerSchedulesCollectorBackedShadowCheck(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	senderManager := &recordingSenderManager{}
	collector := &recordingCollector{}
	scheduler := newTestShadowScheduler(loader, senderManager, collector)
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))

	assert.Equal(t, sourceConfig.SourceConfig, loader.loadedConfig)
	assert.Equal(t, sourceConfig.Instance, loader.loadedInstance)
	assert.Equal(t, sourceConfig.InstanceIndex, loader.loadedInstanceIndex)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.GetSenderIDs())
	require.Len(t, collector.runChecks, 1)
	assert.Equal(t, sourceConfig.ShadowCheckID, collector.runChecks[0].ID())
	assert.Equal(t, time.Second, collector.runChecks[0].Interval())
	assert.True(t, check.IsShadow(collector.runChecks[0]))
}

func TestShadowSchedulerDoesNotRescheduleExistingShadowCheck(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	collector := &recordingCollector{}
	scheduler := newTestShadowScheduler(loader, &recordingSenderManager{}, collector)
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))

	assert.Equal(t, 1, loader.LoadCount())
	assert.Len(t, collector.runChecks, 1)
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
	collector := &recordingCollector{}
	scheduler := newTestShadowScheduler(loader, senderManager, collector)
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
	close(loader.release)
	scheduleWG.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Len(t, collector.runChecks, 1)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
}

func TestShadowSchedulerUnscheduleStopsCollectorCheckAndDestroysSender(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	senderManager := &recordingSenderManager{}
	collector := &recordingCollector{}
	scheduler := newTestShadowScheduler(loader, senderManager, collector)
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{sourceConfig}))
	require.NoError(t, scheduler.Unschedule([]ShadowConfig{sourceConfig}))

	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, collector.stoppedChecks)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
	assert.False(t, scheduler.IsCheckScheduled(sourceConfig.ShadowCheckID))
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
	firstSenderManager := &recordingSenderManager{}
	secondSenderManager := &recordingSenderManager{}
	senderManagers := []*recordingSenderManager{firstSenderManager, secondSenderManager}
	collector := &recordingCollector{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager {
			sm := senderManagers[0]
			senderManagers = senderManagers[1:]
			return sm
		},
		Collector: collector,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	require.NoError(t, scheduler.Schedule([]ShadowConfig{first, second}))
	require.NoError(t, scheduler.Unschedule([]ShadowConfig{first}))

	assert.Equal(t, []checkid.ID{first.ShadowCheckID}, collector.stoppedChecks)
	assert.Equal(t, []checkid.ID{first.ShadowCheckID}, firstSenderManager.DestroySenderIDs())
	assert.Empty(t, secondSenderManager.DestroySenderIDs())
	assert.False(t, scheduler.IsCheckScheduled(first.ShadowCheckID))
	assert.True(t, scheduler.IsCheckScheduled(second.ShadowCheckID))
}

func TestShadowSchedulerLoadErrorCleansUpShadowSenderAndCancelsContext(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	senderManager := &recordingSenderManager{}
	collector := &recordingCollector{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           &failingShadowLoader{sourceID: sourceConfig.SourceCheckID},
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return senderManager },
		Collector:        collector,
	})
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	err := scheduler.Schedule([]ShadowConfig{sourceConfig})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "load failed")
	assert.Empty(t, collector.runChecks)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.GetSenderIDs())
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
}

func TestShadowSchedulerCollectorScheduleErrorCleansUpShadowSender(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	senderManager := &recordingSenderManager{}
	collector := &recordingCollector{runErr: errors.New("collector stopped")}
	scheduler := newTestShadowScheduler(&testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}, senderManager, collector)
	t.Cleanup(func() { assert.NoError(t, scheduler.Stop()) })

	err := scheduler.Schedule([]ShadowConfig{sourceConfig})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "collector stopped")
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
	assert.False(t, scheduler.IsCheckScheduled(sourceConfig.ShadowCheckID))
}

func TestShadowSchedulerStopPreventsInFlightLoadFromStarting(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &blockingShadowLoader{
		checks:  []*testShadowCheck{newTestShadowCheck(sourceConfig.SourceCheckID)},
		ready:   make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	senderManager := &recordingSenderManager{}
	collector := &recordingCollector{}
	scheduler := newTestShadowScheduler(loader, senderManager, collector)

	errCh := make(chan error, 1)
	go func() {
		errCh <- scheduler.Schedule([]ShadowConfig{sourceConfig})
	}()

	select {
	case <-loader.ready:
	case <-time.After(time.Second):
		t.Fatal("expected schedule to reach load")
	}
	require.NoError(t, scheduler.Stop())
	close(loader.release)

	err := <-errCh
	require.Error(t, err)
	assert.ErrorIs(t, err, errShadowSchedulerStopped)
	assert.Empty(t, collector.runChecks)
	assert.Equal(t, []checkid.ID{sourceConfig.ShadowCheckID}, senderManager.DestroySenderIDs())
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
	collector := &recordingCollector{}
	scheduler := NewShadowScheduler(ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager {
			sm := senderManagers[0]
			senderManagers = senderManagers[1:]
			return sm
		},
		Collector: collector,
	})

	require.NoError(t, scheduler.Schedule([]ShadowConfig{first, second}))
	require.NoError(t, scheduler.Stop())
	require.NoError(t, scheduler.Stop())

	assert.ElementsMatch(t, []checkid.ID{first.ShadowCheckID, second.ShadowCheckID}, collector.stoppedChecks)
	assert.Equal(t, []checkid.ID{first.ShadowCheckID}, firstSenderManager.DestroySenderIDs())
	assert.Equal(t, []checkid.ID{second.ShadowCheckID}, secondSenderManager.DestroySenderIDs())
}

func TestShadowSchedulerScheduleAfterStopDoesNotLoad(t *testing.T) {
	sourceConfig := newTestShadowConfig()
	loader := &testShadowLoader{check: newTestShadowCheck(sourceConfig.SourceCheckID)}
	scheduler := newTestShadowScheduler(loader, &recordingSenderManager{}, &recordingCollector{})

	require.NoError(t, scheduler.Stop())

	err := scheduler.Schedule([]ShadowConfig{sourceConfig})

	require.Error(t, err)
	assert.ErrorIs(t, err, errShadowSchedulerStopped)
	assert.Equal(t, 0, loader.LoadCount())
}

func newTestShadowScheduler(loader CheckInstanceLoader, senderManager aggregatorsender.SenderManager, collector CheckScheduler) *ShadowScheduler {
	return NewShadowScheduler(ShadowSchedulerOptions{
		Loader:           loader,
		NewSenderManager: func(context.Context) aggregatorsender.SenderManager { return senderManager },
		Collector:        collector,
		Interval:         time.Second,
	})
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
	id checkid.ID
}

func newTestShadowCheck(id checkid.ID) *testShadowCheck {
	return &testShadowCheck{id: id}
}

func (c *testShadowCheck) Run() error              { return nil }
func (c *testShadowCheck) Stop()                   {}
func (c *testShadowCheck) Cancel()                 {}
func (c *testShadowCheck) String() string          { return checkid.IDToCheckName(c.id) }
func (c *testShadowCheck) Loader() string          { return goCheckLoaderName }
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
func (c *testShadowCheck) Configure(aggregatorsender.SenderManager, uint64, integration.Data, integration.Data, string, string) error {
	return nil
}

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

type recordingCollector struct {
	runChecks     []check.Check
	stoppedChecks []checkid.ID
	runErr        error
	stopErr       error
}

func (c *recordingCollector) RunCheck(check check.Check) (checkid.ID, error) {
	if c.runErr != nil {
		return "", c.runErr
	}
	c.runChecks = append(c.runChecks, check)
	return check.ID(), nil
}

func (c *recordingCollector) StopCheck(id checkid.ID) error {
	c.stoppedChecks = append(c.stoppedChecks, id)
	return c.stopErr
}
