// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package providers

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scheduleToMap(configs []integration.Config) map[string]integration.Config {
	result := make(map[string]integration.Config, len(configs))
	for _, config := range configs {
		result[config.Name] = config
	}
	return result
}

func isRootUser() bool {
	return os.Geteuid() == 0
}

func skipOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows due to Unix-specific file operations and permissions")
	}
}

func (p *processLogConfigProvider) processEventsNoVerifyReadable(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	return p.processEventsInner(evBundle, false)
}

func TestProcessLogProviderEvents(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath := "/var/log/test.log"

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{logPath},
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
	assert.Len(t, changes.Schedule, 1)
	assert.Len(t, changes.Unschedule, 0)
	config := changes.Schedule[0]
	assert.Equal(t, getIntegrationName(logPath), config.Name)
	assert.Equal(t, names.ProcessLog, config.Provider)
	assert.Contains(t, string(config.LogsConfig), logPath)

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
	assert.Equal(t, getIntegrationName(logPath), config.Name)

	// check that unscheduling the same config again doesn't do anything
	changes = p.processEventsNoVerifyReadable(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

// TestProcessLogProviderNoLogFile tests that a process without a log file doesn't generate a config
func TestProcessLogProviderNoLogFile(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
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

func TestProcessLogProviderMultipleLogSources(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath1 := "/var/log/test.log"
	logPath2 := "/var/log/test2.log"

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{logPath1, logPath2},
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

// TestProcessLogProviderMultipleProcesses creates multiple processes and checks that they are all scheduled and unscheduled correctly.
func TestProcessLogProviderMultipleProcesses(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath1 := "/var/log/test.log"
	logPath2 := "/var/log/test2.log"

	process1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{logPath1},
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
			LogFiles:      []string{logPath2},
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

	scheduleMap := scheduleToMap(changes.Schedule)

	config1, found1 := scheduleMap[getIntegrationName(logPath1)]
	assert.True(t, found1)
	assert.Contains(t, string(config1.LogsConfig), logPath1)

	config2, found2 := scheduleMap[getIntegrationName(logPath2)]
	assert.True(t, found2)
	assert.Contains(t, string(config2.LogsConfig), logPath2)

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

	unscheduleMap := scheduleToMap(changes.Unschedule)

	assert.Contains(t, unscheduleMap, getIntegrationName(logPath1))
	assert.Contains(t, unscheduleMap, getIntegrationName(logPath2))
}

// TestProcessLogProviderReferenceCounting tests the reference counting behavior for multiple processes using the same log file
func TestProcessLogProviderReferenceCounting(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath := "/var/log/test.log"

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
			LogFiles:      []string{logPath},
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
			LogFiles:      []string{logPath},
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
	assert.Equal(t, getIntegrationName(logPath), config.Name)
	assert.Equal(t, getServiceID(logPath), config.ServiceID)

	// Verify reference count is 1
	serviceLogKey := logPath
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
	assert.NotContains(t, p.serviceLogRefs, serviceLogKey)
}

// TestProcessLogProviderUnscheduleNonExistent tests that unscheduling a non-existent config does not panic.
func TestProcessLogProviderUnscheduleNonExistent(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath := "/var/log/test.log"

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{logPath},
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

// Test that when a process has multiple log files, we get one config for each
func TestProcessLogProviderOneProcessMultipleLogFiles(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath1 := "/var/log/test.log"
	logPath2 := "/var/log/test2.log"

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Pid: 123,
		Service: &workloadmeta.Service{
			DDService:     "test-service",
			GeneratedName: "test-service-gen",
			LogFiles:      []string{logPath1, logPath2},
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

	scheduleMap := scheduleToMap(changes.Schedule)

	config1, found1 := scheduleMap[getIntegrationName(logPath1)]
	assert.True(t, found1)
	assert.Equal(t, `[{"path":"`+logPath1+`","service":"test-service","source":"test-service-gen","type":"file"}]`, string(config1.LogsConfig))

	config2, found2 := scheduleMap[getIntegrationName(logPath2)]
	assert.True(t, found2)
	assert.Equal(t, `[{"path":"`+logPath2+`","service":"test-service","source":"test-service-gen","type":"file"}]`, string(config2.LogsConfig))

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
	assert.Equal(t, getIntegrationName(logPath1), changes.Unschedule[0].Name)
	assert.Equal(t, getIntegrationName(logPath2), changes.Unschedule[1].Name)
}

// TestProcessLogProviderProcessLogFilesChange tests that when a process's log files change in a Set event,
// the old configs are unscheduled and new ones are scheduled correctly
func TestProcessLogProviderProcessLogFilesChange(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)

	logPath1 := "/var/log/test1.log"
	logPath2 := "/var/log/test2.log"
	logPath3 := "/var/log/test3.log"

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
			LogFiles:      []string{logPath1},
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
	assert.Equal(t, getIntegrationName(logPath1), config1.Name)
	assert.Contains(t, string(config1.LogsConfig), logPath1)

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
			LogFiles:      []string{logPath2, logPath3}, // Different log files
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
	assert.Equal(t, getIntegrationName(logPath1), unscheduledConfig.Name)

	// Check that new configs were scheduled
	scheduleMap := scheduleToMap(changes.Schedule)

	config2, found2 := scheduleMap[getIntegrationName(logPath2)]
	assert.True(t, found2)
	assert.Contains(t, string(config2.LogsConfig), logPath2)

	config3, found3 := scheduleMap[getIntegrationName(logPath3)]
	assert.True(t, found3)
	assert.Contains(t, string(config3.LogsConfig), logPath3)

	// Verify reference counts are correct
	key1 := logPath1
	key2 := logPath2
	key3 := logPath3

	// Old key should not exist
	assert.NotContains(t, p.serviceLogRefs, key1)

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
	unscheduleMap := scheduleToMap(changes.Unschedule)

	assert.Contains(t, unscheduleMap, getIntegrationName(logPath2))
	assert.Contains(t, unscheduleMap, getIntegrationName(logPath3))

	// Verify all reference entries are cleaned up
	assert.NotContains(t, p.serviceLogRefs, key2)
	assert.NotContains(t, p.serviceLogRefs, key3)
}

// TestProcessLogProviderFileReadabilityVerification tests that only readable log files are configured
// when using processEvents (with verification) vs processEventsNoVerifyReadable
func TestProcessLogProviderFileReadabilityVerification(t *testing.T) {
	skipOnWindows(t)

	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
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
	scheduleMap := scheduleToMap(changes.Schedule)

	var foundReadable, foundNonReadable bool
	for _, config := range scheduleMap {
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

// TestProcessLogProviderFileReadabilityWithPermissionDenied tests the case where a file exists but is not readable
func TestProcessLogProviderFileReadabilityWithPermissionDenied(t *testing.T) {
	skipOnWindows(t)

	// Skip this test if running as root since root can read any file
	if isRootUser() {
		t.Skip("Skipping permission test when running as root")
	}

	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
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

func TestProcessLogProviderIsFileReadable(t *testing.T) {
	skipOnWindows(t)

	// Test 1: Readable text file
	readableFile, err := os.CreateTemp("", "readable_test_*.log")
	require.NoError(t, err)
	defer os.Remove(readableFile.Name())
	defer readableFile.Close()

	_, err = readableFile.WriteString("test log content\nanother line")
	require.NoError(t, err)

	assert.NoError(t, checkFileReadable(readableFile.Name()), "Readable text file should return nil")

	// Test 2: Non-existent file
	nonExistentFile := "/non/existent/path/test.log"
	assert.Error(t, checkFileReadable(nonExistentFile), "Non-existent file should return error")

	// Test 3: Binary file (non-UTF8 content)
	binaryFile, err := os.CreateTemp("", "binary_test_*.bin")
	require.NoError(t, err)
	defer os.Remove(binaryFile.Name())
	defer binaryFile.Close()

	// Write binary data (non-UTF8)
	binaryData := []byte{0xFF, 0xFE, 0x00, 0x01, 0x02, 0x03}
	_, err = binaryFile.Write(binaryData)
	require.NoError(t, err)

	assert.Error(t, checkFileReadable(binaryFile.Name()), "Binary file should return error")

	// Test 4: Empty file
	emptyFile, err := os.CreateTemp("", "empty_test_*.log")
	require.NoError(t, err)
	defer os.Remove(emptyFile.Name())
	defer emptyFile.Close()

	assert.NoError(t, checkFileReadable(emptyFile.Name()), "Empty file should return nil")

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

		assert.Error(t, checkFileReadable(permissionFile.Name()), "File with permission denied should return error")
	} else {
		t.Log("Skipping permission denied test when running as root")
	}

	// Test 6: Directory (should fail to open as file)
	tempDir, err := os.MkdirTemp("", "dir_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	assert.Error(t, checkFileReadable(tempDir), "Directory should return error")

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

	assert.Error(t, checkFileReadable(partialUTF8File.Name()), "File with partial UTF8 content should return error")

	// Test 8: File with only UTF8 control characters
	controlCharFile, err := os.CreateTemp("", "control_char_test_*.log")
	require.NoError(t, err)
	defer os.Remove(controlCharFile.Name())
	defer controlCharFile.Close()

	// Write UTF8 control characters (newlines, tabs, etc.)
	_, err = controlCharFile.WriteString("\n\t\r\f\v")
	require.NoError(t, err)

	assert.NoError(t, checkFileReadable(controlCharFile.Name()), "File with UTF8 control characters should return nil")
}

func TestProcessLogProviderServiceName(t *testing.T) {
	tests := []struct {
		name    string
		service workloadmeta.Service
		want    string
	}{
		{
			name: "returns TracerMetadata ServiceName if present",
			service: workloadmeta.Service{
				GeneratedName: "foo",
				DDService:     "bar",
				TracerMetadata: []tracermetadata.TracerMetadata{
					{ServiceName: "tracer-service"},
				},
			},
			want: "bar",
		},
		{
			name: "returns DDService if TracerMetadata is empty and DDService is set",
			service: workloadmeta.Service{
				GeneratedName: "foo",
				DDService:     "bar",
			},
			want: "bar",
		},
		{
			name: "returns GeneratedName if TracerMetadata and DDService are empty",
			service: workloadmeta.Service{
				GeneratedName: "foo",
			},
			want: "foo",
		},
		{
			name:    "returns empty string if all fields are empty",
			service: workloadmeta.Service{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getServiceName(&tt.service)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProcessLogProviderAgentExclude(t *testing.T) {
	agentLogPath := "/var/log/agent.log"
	notAgentLogPath := "/var/log/not-agent.log"

	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "123",
					},
					Pid: 123,
					Exe: "/opt/datadog-agent/bin/agent",
					Service: &workloadmeta.Service{
						DDService: "agent",
						LogFiles:  []string{agentLogPath},
					},
				},
			},
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   "456",
					},
					Pid: 456,
					Exe: "/usr/bin/not-agent",
					Service: &workloadmeta.Service{
						DDService: "not-agent",
						LogFiles:  []string{notAgentLogPath},
					},
				},
			},
		},
	}

	createProvider := func(excludeAgent bool) *processLogConfigProvider {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("logs_config.process_exclude_agent", excludeAgent)

		provider, err := NewProcessLogConfigProvider(nil, nil, nil)
		require.NoError(t, err)
		p, ok := provider.(*processLogConfigProvider)
		require.True(t, ok)

		return p
	}

	p := createProvider(false)
	changes := p.processEventsNoVerifyReadable(setBundle)
	require.Len(t, changes.Schedule, 2)

	p = createProvider(true)
	changes = p.processEventsNoVerifyReadable(setBundle)
	require.Len(t, changes.Schedule, 1)
	assert.Equal(t, getIntegrationName(notAgentLogPath), changes.Schedule[0].Name)
}
