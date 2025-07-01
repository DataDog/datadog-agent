// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && !serverless

package providers

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isRootUser checks if the current user is root
func isRootUser() bool {
	return os.Geteuid() == 0
}

func (p *ProcessLogConfigProvider) processEventsNoVerifyReadable(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	return p.processEventsInner(evBundle, false)
}

func TestProcessLogProviderEvents(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	// Test scheduling a config
	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}
	changes := p.processEventsNoVerifyReadable(setBundle)
	t.Logf("Changes: %+v", changes)
	t.Logf("Schedule count: %d", len(changes.Schedule))
	t.Logf("Unschedule count: %d", len(changes.Unschedule))
	assert.Len(t, changes.Schedule, 1)
	assert.Len(t, changes.Unschedule, 0)
	config := changes.Schedule[0]
	assert.Equal(t, "process-test-service-gen-_var_log_test.log", config.Name)
	assert.Equal(t, names.ProcessLog, config.Provider)
	assert.Contains(t, string(config.LogsConfig), "/var/log/test.log")

	// check that scheduling the same config again doesn't do anything
	changes = p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)

	// Test unscheduling a config
	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 1)
	config = changes.Unschedule[0]
	assert.Equal(t, "process-test-service-gen-_var_log_test.log", config.Name)

	// check that unscheduling the same config again doesn't do anything
	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

// TestProcessNoLogFile tests that a process without a log file doesn't generate a config
func TestProcessNoLogFile(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

func TestMultipleLogSources(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log", "/var/log/test2.log"},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)

	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 2)
}

func TestGetConfigErrors(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	errors := p.GetConfigErrors()
	assert.Empty(t, errors)
}

func TestProcessLogProviderString(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	assert.Equal(t, names.ProcessLog, p.String())
}

func TestStream(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	_, ok := provider.(types.StreamingConfigProvider)
	assert.True(t, ok)
}

// TestMultipleProcesses creates multiple processes and checks that they are all scheduled and unscheduled correctly.
func TestMultipleProcesses(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}
	process2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "456",
		},
		Pid: 456,
		Service: &workloadmeta.Service{
			DDService:     "test-service-2",
			GeneratedName: "test-service-2-gen",
			LogFiles:      []string{"/var/log/test2.log"},
		},
	}

	setEvents := []workloadmeta.Event{
		{
			Type:   workloadmeta.EventTypeSet,
			Entity: process1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Entity: process2,
		},
	}
	setBundle := workloadmeta.EventBundle{
		Events: setEvents,
	}

	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)
	var found1, found2 bool
	for _, config := range changes.Schedule {
		if config.Name == "process-test-service-gen-_var_log_test.log" {
			found1 = true
			assert.Contains(t, string(config.LogsConfig), "/var/log/test.log")
		} else if config.Name == "process-test-service-2-gen-_var_log_test2.log" {
			found2 = true
			assert.Contains(t, string(config.LogsConfig), "/var/log/test2.log")
		}
	}
	assert.True(t, found1)
	assert.True(t, found2)

	unsetEvents := []workloadmeta.Event{
		{
			Type:   workloadmeta.EventTypeUnset,
			Entity: process1,
		},
		{
			Type:   workloadmeta.EventTypeUnset,
			Entity: process2,
		},
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: unsetEvents,
	}

	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 2)
	found1, found2 = false, false
	for _, config := range changes.Unschedule {
		if config.Name == "process-test-service-gen-_var_log_test.log" {
			found1 = true
		} else if config.Name == "process-test-service-2-gen-_var_log_test2.log" {
			found2 = true
		}
	}
	assert.True(t, found1)
	assert.True(t, found2)
}

// TestReferenceCounting tests the reference counting behavior for multiple processes using the same log file
func TestReferenceCounting(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Create two processes with the same service name and log file
	process1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	process2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "456",
		},
		Pid: 456,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	// Schedule first process - should create a new config
	setEvent1 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process1,
	}
	setBundle1 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent1},
	}
	changes := p.processEventsNoVerifyReadable(setBundle1)
	assert.Len(t, changes.Schedule, 1)
	assert.Len(t, changes.Unschedule, 0)
	config := changes.Schedule[0]
	assert.Equal(t, "process-test-service-gen-_var_log_test.log", config.Name)
	assert.Equal(t, fmt.Sprintf("%s://test-service-gen:_var_log_test.log", names.ProcessLog), config.ServiceID)

	// Verify reference count is 1
	serviceLogKey := p.generateServiceLogKey("test-service-gen", "/var/log/test.log")
	ref, exists := p.serviceLogRefs[serviceLogKey]
	assert.True(t, exists)
	assert.Equal(t, 1, ref.refCount)

	// Schedule second process with same service and log - should only increment reference count
	setEvent2 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process2,
	}
	setBundle2 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent2},
	}
	changes = p.processEventsNoVerifyReadable(setBundle2)
	assert.Len(t, changes.Schedule, 0) // No new config scheduled
	assert.Len(t, changes.Unschedule, 0)

	// Verify reference count is now 2
	ref, exists = p.serviceLogRefs[serviceLogKey]
	assert.True(t, exists)
	assert.Equal(t, 2, ref.refCount)

	// Unschedule first process - should only decrement reference count
	unsetEvent1 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process1,
	}
	unsetBundle1 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent1},
	}
	changes = p.processEventsNoVerifyReadable(unsetBundle1)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0) // Config not unscheduled yet

	// Verify reference count is now 1
	ref, exists = p.serviceLogRefs[serviceLogKey]
	assert.True(t, exists)
	assert.Equal(t, 1, ref.refCount)

	// Unschedule second process - should unschedule config and cleanup
	unsetEvent2 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process2,
	}
	unsetBundle2 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent2},
	}
	changes = p.processEventsNoVerifyReadable(unsetBundle2)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 1) // Config now unscheduled

	// Verify cleanup
	_, exists = p.serviceLogRefs[serviceLogKey]
	assert.False(t, exists)
}

// TestReferenceCountingDifferentServices tests that different services with same log path get separate configs
func TestReferenceCountingDifferentServices(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Create two processes with different service names but same log file
	process1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "service1",
			GeneratedName: "service1-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	process2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "456",
		},
		Pid: 456,
		Service: &workloadmeta.Service{
			DDService:     "service2",
			GeneratedName: "service2-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	// Schedule both processes
	setEvents := []workloadmeta.Event{
		{Type: workloadmeta.EventTypeSet, Entity: process1},
		{Type: workloadmeta.EventTypeSet, Entity: process2},
	}
	setBundle := workloadmeta.EventBundle{Events: setEvents}
	changes := p.processEventsNoVerifyReadable(setBundle)

	// Should create two separate configs
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)

	// Verify both configs exist with different service IDs
	var config1, config2 integration.Config
	for _, config := range changes.Schedule {
		if config.Name == "process-service1-gen-_var_log_test.log" {
			config1 = config
		} else if config.Name == "process-service2-gen-_var_log_test.log" {
			config2 = config
		}
	}
	assert.NotEmpty(t, config1.Name)
	assert.NotEmpty(t, config2.Name)
	assert.NotEqual(t, config1.ServiceID, config2.ServiceID)

	// Verify both reference entries exist
	key1 := p.generateServiceLogKey("service1-gen", "/var/log/test.log")
	key2 := p.generateServiceLogKey("service2-gen", "/var/log/test.log")
	ref1, exists1 := p.serviceLogRefs[key1]
	ref2, exists2 := p.serviceLogRefs[key2]
	assert.True(t, exists1)
	assert.True(t, exists2)
	assert.Equal(t, 1, ref1.refCount)
	assert.Equal(t, 1, ref2.refCount)

	// Unschedule both processes
	unsetEvents := []workloadmeta.Event{
		{Type: workloadmeta.EventTypeUnset, Entity: process1},
		{Type: workloadmeta.EventTypeUnset, Entity: process2},
	}
	unsetBundle := workloadmeta.EventBundle{Events: unsetEvents}
	changes = p.processEventsNoVerifyReadable(unsetBundle)

	// Should unschedule both configs
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 2)

	// Verify cleanup
	_, exists1 = p.serviceLogRefs[key1]
	_, exists2 = p.serviceLogRefs[key2]
	assert.False(t, exists1)
	assert.False(t, exists2)
}

// TestReferenceCountingPathSanitization tests that log paths are properly sanitized
func TestReferenceCountingPathSanitization(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Test that slashes are replaced with underscores in service log keys
	key1 := p.generateServiceLogKey("test-service", "/var/log/test.log")
	key2 := p.generateServiceLogKey("test-service", "/var/log/test.log")
	key3 := p.generateServiceLogKey("test-service", "/var/log/different.log")

	assert.Equal(t, "test-service:_var_log_test.log", key1)
	assert.Equal(t, key1, key2)    // Same key for same path
	assert.NotEqual(t, key1, key3) // Different key for different path

	// Test that the sanitized path is used in config names
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}
	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 1)

	config := changes.Schedule[0]
	assert.Equal(t, "process-test-service-gen-_var_log_test.log", config.Name)
	assert.Equal(t, fmt.Sprintf("%s://test-service-gen:_var_log_test.log", names.ProcessLog), config.ServiceID)
}

func TestInvalidEvent(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	invalidEvent := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "invalid",
			},
		},
	}

	invalidBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{invalidEvent},
	}

	changes := p.processEventsNoVerifyReadable(invalidBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

func TestProcessWithoutService(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid:     123,
		Service: nil,
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

func TestProcessWithContainerID(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid:         123,
		ContainerID: "container-123",
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

// TestProcessWithNilServiceComprehensive tests that processes with nil Service are handled correctly
// for both Set and Unset events, including edge cases like PID tracking
func TestProcessWithNilServiceComprehensive(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Test 1: Set event with nil Service
	processWithNilService := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid:     123,
		Service: nil,
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: processWithNilService,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
	// Verify that no PID tracking was added
	assert.Empty(t, p.pidToServiceIDs[123])

	// Test 2: Unset event with nil Service (should not panic or cause issues)
	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: processWithNilService,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}

	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)

	// Test 3: Set event with nil Service after having a process with Service
	// This should clean up any existing PID tracking
	processWithService := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	// First set a process with service
	setEventWithService := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: processWithService,
	}
	setBundleWithService := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEventWithService},
	}
	changes = p.processEventsNoVerifyReadable(setBundleWithService)
	assert.Len(t, changes.Schedule, 1)
	assert.Len(t, changes.Unschedule, 0)
	assert.NotEmpty(t, p.pidToServiceIDs[123])

	// Now set the same PID with nil Service - should clean up
	setEventNilService := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: processWithNilService,
	}
	setBundleNilService := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEventNilService},
	}
	changes = p.processEventsNoVerifyReadable(setBundleNilService)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 1)    // Should unschedule the previous config
	assert.Empty(t, p.pidToServiceIDs[123]) // Should clean up PID tracking

	// Test 4: Mixed events with nil and non-nil services
	mixedProcess1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "456",
		},
		Pid:     456,
		Service: nil,
	}

	mixedProcess2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "789",
		},
		Pid: 789,
		Service: &workloadmeta.Service{
			DDService:     "test-service-2",
			GeneratedName: "test-service-2-gen",
			LogFiles:      []string{"/var/log/test2.log"},
		},
	}

	mixedEvents := []workloadmeta.Event{
		{Type: workloadmeta.EventTypeSet, Entity: mixedProcess1},
		{Type: workloadmeta.EventTypeSet, Entity: mixedProcess2},
	}
	mixedBundle := workloadmeta.EventBundle{
		Events: mixedEvents,
	}

	changes = p.processEventsNoVerifyReadable(mixedBundle)
	assert.Len(t, changes.Schedule, 1) // Only the process with service should generate config
	assert.Len(t, changes.Unschedule, 0)
	assert.Empty(t, p.pidToServiceIDs[456])    // PID with nil service should not be tracked
	assert.NotEmpty(t, p.pidToServiceIDs[789]) // PID with service should be tracked
}

// TestUnscheduleNonExistent tests that unscheduling a non-existent config does not panic.
func TestUnscheduleNonExistent(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	changes := p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

func TestConfigDigest(t *testing.T) {
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	p := &ProcessLogConfigProvider{}
	serviceLogKey1 := p.generateServiceLogKey(process.Service.GeneratedName, "/var/log/test.log")
	config1, err := p.buildConfig(process, "/var/log/test.log", serviceLogKey1)
	require.NoError(t, err)

	config2, err := p.buildConfig(process, "/var/log/test.log", serviceLogKey1)
	require.NoError(t, err)

	assert.Equal(t, config1.Digest(), config2.Digest())
}

// Test that when a process has multiple log files, we get one config for each
func TestOneProcessMultipleLogFiles(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log", "/var/log/test2.log"},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}
	changes := p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)
	var found1, found2 bool
	for _, config := range changes.Schedule {
		if config.Name == "process-test-service-gen-_var_log_test.log" {
			if string(config.LogsConfig) == `[{"path":"/var/log/test.log","service":"test-service","source":"test-service-gen","type":"file"}]` {
				found1 = true
			}
		} else if config.Name == "process-test-service-gen-_var_log_test2.log" {
			if string(config.LogsConfig) == `[{"path":"/var/log/test2.log","service":"test-service","source":"test-service-gen","type":"file"}]` {
				found2 = true
			}
		}
	}
	assert.True(t, found1)
	assert.True(t, found2)

	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 2)
	// both configs have different names now due to the log path in the name
	assert.Equal(t, "process-test-service-gen-_var_log_test.log", changes.Unschedule[0].Name)
	assert.Equal(t, "process-test-service-gen-_var_log_test2.log", changes.Unschedule[1].Name)
}

// TestDebugProcessEvents is a simple test to debug why processEvents is not working
func TestDebugProcessEvents(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test.log"},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	t.Logf("Processing events for process: %+v", process)
	t.Logf("Service: %+v", process.Service)

	changes := p.processEventsNoVerifyReadable(setBundle)
	t.Logf("Changes: %+v", changes)
	t.Logf("Schedule count: %d", len(changes.Schedule))
	t.Logf("Unschedule count: %d", len(changes.Unschedule))

	if len(changes.Schedule) > 0 {
		t.Logf("First scheduled config: %+v", changes.Schedule[0])
	}
}

// TestProcessLogFilesChange tests that when a process's log files change in a Set event,
// the old configs are unscheduled and new ones are scheduled correctly
func TestProcessLogFilesChange(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Initial process with log file 1
	process1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test1.log"},
		},
	}

	// Schedule initial process
	setEvent1 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process1,
	}
	setBundle1 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent1},
	}
	changes := p.processEventsNoVerifyReadable(setBundle1)
	assert.Len(t, changes.Schedule, 1)
	assert.Len(t, changes.Unschedule, 0)
	config1 := changes.Schedule[0]
	assert.Equal(t, "process-test-service-gen-_var_log_test1.log", config1.Name)
	assert.Contains(t, string(config1.LogsConfig), "/var/log/test1.log")

	// Update process with different log files
	process2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123", // Same process ID
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{"/var/log/test2.log", "/var/log/test3.log"}, // Different log files
		},
	}

	// Set event with updated process should unschedule old config and schedule new ones
	setEvent2 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process2,
	}
	setBundle2 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent2},
	}
	changes = p.processEventsNoVerifyReadable(setBundle2)

	// Should unschedule the old config and schedule two new ones
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 1)

	// Check that old config was unscheduled
	unscheduledConfig := changes.Unschedule[0]
	assert.Equal(t, "process-test-service-gen-_var_log_test1.log", unscheduledConfig.Name)

	// Check that new configs were scheduled
	var found2, found3 bool
	for _, config := range changes.Schedule {
		if config.Name == "process-test-service-gen-_var_log_test2.log" {
			found2 = true
			assert.Contains(t, string(config.LogsConfig), "/var/log/test2.log")
		} else if config.Name == "process-test-service-gen-_var_log_test3.log" {
			found3 = true
			assert.Contains(t, string(config.LogsConfig), "/var/log/test3.log")
		}
	}
	assert.True(t, found2)
	assert.True(t, found3)

	// Verify reference counts are correct
	key1 := p.generateServiceLogKey("test-service-gen", "/var/log/test1.log")
	key2 := p.generateServiceLogKey("test-service-gen", "/var/log/test2.log")
	key3 := p.generateServiceLogKey("test-service-gen", "/var/log/test3.log")

	// Old key should not exist
	_, exists := p.serviceLogRefs[key1]
	assert.False(t, exists)

	// New keys should exist with ref count 1
	ref2, exists := p.serviceLogRefs[key2]
	assert.True(t, exists)
	assert.Equal(t, 1, ref2.refCount)

	ref3, exists := p.serviceLogRefs[key3]
	assert.True(t, exists)
	assert.Equal(t, 1, ref3.refCount)

	// Update process to remove all log files
	process3 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123", // Same process ID
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{}, // No log files
		},
	}

	// Set event with no log files should unschedule all configs
	setEvent3 := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process3,
	}
	setBundle3 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent3},
	}
	changes = p.processEventsNoVerifyReadable(setBundle3)

	// Should unschedule both configs and schedule none
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 2)

	// Check that both configs were unscheduled
	var foundUnschedule2, foundUnschedule3 bool
	for _, config := range changes.Unschedule {
		if config.Name == "process-test-service-gen-_var_log_test2.log" {
			foundUnschedule2 = true
		} else if config.Name == "process-test-service-gen-_var_log_test3.log" {
			foundUnschedule3 = true
		}
	}
	assert.True(t, foundUnschedule2)
	assert.True(t, foundUnschedule3)

	// Verify all reference entries are cleaned up
	_, exists = p.serviceLogRefs[key2]
	assert.False(t, exists)
	_, exists = p.serviceLogRefs[key3]
	assert.False(t, exists)
}

// TestFileReadabilityVerification tests that only readable log files are configured
// when using processEvents (with verification) vs processEventsNoVerifyReadable
func TestFileReadabilityVerification(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Create a temporary readable file
	readableFile, err := os.CreateTemp("", "readable_test_*.log")
	require.NoError(t, err)
	defer os.Remove(readableFile.Name())
	defer readableFile.Close()

	// Write some content to make it a real file
	_, err = readableFile.WriteString("test log content")
	require.NoError(t, err)

	// Create a non-readable file path (directory that doesn't exist)
	nonReadableFile := "/non/existent/path/test.log"

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{readableFile.Name(), nonReadableFile},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	// Test with verification enabled (processEvents) - should only schedule readable file
	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 1, "Should only schedule the readable file")
	assert.Len(t, changes.Unschedule, 0)

	// Verify the scheduled config is for the readable file
	config := changes.Schedule[0]
	assert.Contains(t, string(config.LogsConfig), readableFile.Name())
	assert.NotContains(t, string(config.LogsConfig), nonReadableFile)

	// Clean up for next test
	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	p.processEvents(unsetBundle)

	// Test with verification disabled (processEventsNoVerifyReadable) - should schedule both files
	changes = p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 2, "Should schedule both files when verification is disabled")
	assert.Len(t, changes.Unschedule, 0)

	// Verify both configs were scheduled
	var foundReadable, foundNonReadable bool
	for _, config := range changes.Schedule {
		if strings.Contains(string(config.LogsConfig), readableFile.Name()) {
			foundReadable = true
		}
		if strings.Contains(string(config.LogsConfig), nonReadableFile) {
			foundNonReadable = true
		}
	}
	assert.True(t, foundReadable, "Readable file should be scheduled")
	assert.True(t, foundNonReadable, "Non-readable file should be scheduled when verification is disabled")

	// Clean up
	p.processEventsNoVerifyReadable(unsetBundle)
}

// TestFileReadabilityWithPermissionDenied tests the case where a file exists but is not readable
func TestFileReadabilityWithPermissionDenied(t *testing.T) {
	// Skip this test if running as root since root can read any file
	if isRootUser() {
		t.Skip("Skipping permission test when running as root")
	}

	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*ProcessLogConfigProvider)
	require.True(t, ok)

	// Create a temporary file
	tempFile, err := os.CreateTemp("", "permission_test_*.log")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Write some content
	_, err = tempFile.WriteString("test log content")
	require.NoError(t, err)

	// Change permissions to make it non-readable
	err = os.Chmod(tempFile.Name(), 0000)
	require.NoError(t, err)
	defer os.Chmod(tempFile.Name(), 0644) // Restore permissions for cleanup

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{tempFile.Name()},
		},
	}

	setEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: process,
	}
	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent},
	}

	// Test with verification enabled - should not schedule the non-readable file
	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 0, "Should not schedule file with permission denied")
	assert.Len(t, changes.Unschedule, 0)

	// Test with verification disabled - should schedule the file
	changes = p.processEventsNoVerifyReadable(setBundle)
	assert.Len(t, changes.Schedule, 1, "Should schedule file when verification is disabled")
	assert.Len(t, changes.Unschedule, 0)

	// Clean up
	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	p.processEventsNoVerifyReadable(unsetBundle)
}

func TestIsFileReadable(t *testing.T) {
	// Test 1: Readable text file
	readableFile, err := os.CreateTemp("", "readable_test_*.log")
	require.NoError(t, err)
	defer os.Remove(readableFile.Name())
	defer readableFile.Close()

	_, err = readableFile.WriteString("test log content\nanother line")
	require.NoError(t, err)

	assert.True(t, isFileReadable(readableFile.Name()), "Readable text file should return true")

	// Test 2: Non-existent file
	nonExistentFile := "/non/existent/path/test.log"
	assert.False(t, isFileReadable(nonExistentFile), "Non-existent file should return false")

	// Test 3: Binary file (non-UTF8 content)
	binaryFile, err := os.CreateTemp("", "binary_test_*.bin")
	require.NoError(t, err)
	defer os.Remove(binaryFile.Name())
	defer binaryFile.Close()

	// Write binary data (non-UTF8)
	binaryData := []byte{0xFF, 0xFE, 0x00, 0x01, 0x02, 0x03}
	_, err = binaryFile.Write(binaryData)
	require.NoError(t, err)

	assert.False(t, isFileReadable(binaryFile.Name()), "Binary file should return false")

	// Test 4: Empty file
	emptyFile, err := os.CreateTemp("", "empty_test_*.log")
	require.NoError(t, err)
	defer os.Remove(emptyFile.Name())
	defer emptyFile.Close()

	assert.True(t, isFileReadable(emptyFile.Name()), "Empty file should return true")

	// Test 5: File with permission denied
	if !isRootUser() {
		permissionFile, err := os.CreateTemp("", "permission_test_*.log")
		require.NoError(t, err)
		defer os.Remove(permissionFile.Name())
		defer permissionFile.Close()

		_, err = permissionFile.WriteString("test content")
		require.NoError(t, err)

		// Change permissions to make it non-readable
		err = os.Chmod(permissionFile.Name(), 0000)
		require.NoError(t, err)
		defer os.Chmod(permissionFile.Name(), 0644) // Restore permissions for cleanup

		assert.False(t, isFileReadable(permissionFile.Name()), "File with permission denied should return false")
	} else {
		t.Log("Skipping permission denied test when running as root")
	}

	// Test 6: Directory (should fail to open as file)
	tempDir, err := os.MkdirTemp("", "dir_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	assert.False(t, isFileReadable(tempDir), "Directory should return false")

	// Test 7: File with partial UTF8 content
	partialUTF8File, err := os.CreateTemp("", "partial_utf8_test_*.log")
	require.NoError(t, err)
	defer os.Remove(partialUTF8File.Name())
	defer partialUTF8File.Close()

	// Write valid UTF8 followed by invalid UTF8
	_, err = partialUTF8File.WriteString("valid text")
	require.NoError(t, err)
	_, err = partialUTF8File.Write([]byte{0xFF, 0xFE}) // Invalid UTF8
	require.NoError(t, err)

	assert.False(t, isFileReadable(partialUTF8File.Name()), "File with partial UTF8 content should return false")

	// Test 8: File with only UTF8 control characters
	controlCharFile, err := os.CreateTemp("", "control_char_test_*.log")
	require.NoError(t, err)
	defer os.Remove(controlCharFile.Name())
	defer controlCharFile.Close()

	// Write UTF8 control characters (newlines, tabs, etc.)
	_, err = controlCharFile.WriteString("\n\t\r\f\v")
	require.NoError(t, err)

	assert.True(t, isFileReadable(controlCharFile.Name()), "File with UTF8 control characters should return true")
}
