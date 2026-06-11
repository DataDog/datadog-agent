// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestAutoConfigShadowAdapterReturnsBeforeShadowScheduleCompletes(t *testing.T) {
	source := newTestShadowConfig().SourceConfig
	shadowController := &recordingShadowController{
		scheduleEntered: make(chan struct{}),
		releaseSchedule: make(chan struct{}),
	}
	scheduler := NewAutoConfigShadowAdapter(Options{ShadowChecksEnabled: true}, shadowController)
	t.Cleanup(scheduler.Stop)

	start := time.Now()
	scheduler.Schedule([]integration.Config{source})

	assert.Less(t, time.Since(start), 100*time.Millisecond)
	select {
	case <-shadowController.scheduleEntered:
	case <-time.After(time.Second):
		t.Fatal("expected queued shadow schedule to start")
	}
	close(shadowController.releaseSchedule)
	require.Eventually(t, func() bool {
		return len(shadowController.Scheduled()) == 1
	}, time.Second, 10*time.Millisecond)
}

func TestAutoConfigShadowAdapterSerializesLifecycleOperations(t *testing.T) {
	source := newTestShadowConfig().SourceConfig
	expected := DeriveShadowConfigs([]integration.Config{source}, Options{ShadowChecksEnabled: true})
	shadowController := &recordingShadowController{}
	scheduler := NewAutoConfigShadowAdapter(Options{ShadowChecksEnabled: true}, shadowController)
	t.Cleanup(scheduler.Stop)

	scheduler.Schedule([]integration.Config{source})
	scheduler.Unschedule([]integration.Config{source})

	require.Eventually(t, func() bool {
		return shadowController.OperationNamesEqual([]string{"schedule", "unschedule"})
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, expected, shadowController.Scheduled()[0])
	assert.Equal(t, expected, shadowController.Unscheduled()[0])
}

func TestAutoConfigShadowAdapterStopDropsQueuedOperationsAndStopsShadowScheduler(t *testing.T) {
	source := newTestShadowConfig().SourceConfig
	shadowController := &recordingShadowController{
		scheduleEntered: make(chan struct{}),
		releaseSchedule: make(chan struct{}),
	}
	scheduler := NewAutoConfigShadowAdapter(Options{ShadowChecksEnabled: true}, shadowController)

	scheduler.Schedule([]integration.Config{source})
	select {
	case <-shadowController.scheduleEntered:
	case <-time.After(time.Second):
		t.Fatal("expected first schedule to start")
	}
	scheduler.Unschedule([]integration.Config{source})

	start := time.Now()
	scheduler.Stop()

	assert.Less(t, time.Since(start), 100*time.Millisecond)
	assert.True(t, shadowController.Stopped())
	close(shadowController.releaseSchedule)
	require.Eventually(t, func() bool {
		return shadowController.OperationNamesEqual([]string{"schedule"})
	}, time.Second, 10*time.Millisecond)
}

type recordingShadowController struct {
	mu sync.Mutex

	scheduleEntered chan struct{}
	releaseSchedule chan struct{}

	operations  []string
	scheduled   [][]ShadowConfig
	unscheduled [][]ShadowConfig
	stopped     bool
}

func (s *recordingShadowController) Schedule(configs []ShadowConfig) error {
	if s.scheduleEntered != nil {
		close(s.scheduleEntered)
	}
	if s.releaseSchedule != nil {
		<-s.releaseSchedule
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.operations = append(s.operations, "schedule")
	s.scheduled = append(s.scheduled, configs)
	return nil
}

func (s *recordingShadowController) Unschedule(configs []ShadowConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operations = append(s.operations, "unschedule")
	s.unscheduled = append(s.unscheduled, configs)
	return nil
}

func (s *recordingShadowController) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	return nil
}

func (s *recordingShadowController) Scheduled() [][]ShadowConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]ShadowConfig(nil), s.scheduled...)
}

func (s *recordingShadowController) Unscheduled() [][]ShadowConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]ShadowConfig(nil), s.unscheduled...)
}

func (s *recordingShadowController) Stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *recordingShadowController) OperationNamesEqual(expected []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return assert.ObjectsAreEqual(expected, s.operations)
}
