// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 1)
	assert.Len(t, changes.Unschedule, 0)
	config := changes.Schedule[0]
	assert.Equal(t, "process-123-test-service-gen", config.Name)
	assert.Equal(t, "process_log", config.Provider)
	assert.Contains(t, string(config.LogsConfig), "/var/log/test.log")

	// check that scheduling the same config again doesn't do anything
	changes = p.processEvents(setBundle)
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
	changes = p.processEvents(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 1)
	config = changes.Unschedule[0]
	assert.Equal(t, "process-123-test-service-gen", config.Name)

	// check that unscheduling the same config again doesn't do anything
	changes = p.processEvents(unsetBundle)
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

	changes := p.processEvents(setBundle)
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

	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)

	unsetEvent := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: process,
	}
	unsetBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent},
	}
	changes = p.processEvents(unsetBundle)
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

	assert.Equal(t, "process_log", p.String())
}

func TestStream(t *testing.T) {
	provider, err := NewProcessLogConfigProvider(nil, nil, nil)
	require.NoError(t, err)

	_, ok := provider.(StreamingConfigProvider)
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

	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)
	var found1, found2 bool
	for _, config := range changes.Schedule {
		if config.Name == "process-123-test-service-gen" {
			found1 = true
			assert.Contains(t, string(config.LogsConfig), "/var/log/test.log")
		} else if config.Name == "process-456-test-service-2-gen" {
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

	changes = p.processEvents(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 2)
	found1, found2 = false, false
	for _, config := range changes.Unschedule {
		if config.Name == "process-123-test-service-gen" {
			found1 = true
		} else if config.Name == "process-456-test-service-2-gen" {
			found2 = true
		}
	}
	assert.True(t, found1)
	assert.True(t, found2)
	assert.Len(t, p.configCache, 0)
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

	changes := p.processEvents(invalidBundle)
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

	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
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
	changes := p.processEvents(unsetBundle)
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
	config1, err := p.buildConfig(process, "/var/log/test.log")
	require.NoError(t, err)

	config2, err := p.buildConfig(process, "/var/log/test.log")
	require.NoError(t, err)

	assert.Equal(t, config1.Digest(), config2.Digest())

	process.Pid = 456
	config3, err := p.buildConfig(process, "/var/log/test.log")
	require.NoError(t, err)

	assert.NotEqual(t, config1.Digest(), config3.Digest())
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
	changes := p.processEvents(setBundle)
	assert.Len(t, changes.Schedule, 2)
	assert.Len(t, changes.Unschedule, 0)
	var found1, found2 bool
	for _, config := range changes.Schedule {
		if config.Name == "process-123-test-service-gen" {
			if string(config.LogsConfig) == `[{"path":"/var/log/test.log","service":"test-service","source":"test-service-gen","type":"file"}]` {
				found1 = true
			} else if string(config.LogsConfig) == `[{"path":"/var/log/test2.log","service":"test-service","source":"test-service-gen","type":"file"}]` {
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
	changes = p.processEvents(unsetBundle)
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 2)
	// both configs have the same name, but different digests. The unschedule logic uses ServiceID,
	// so both configs should be unscheduled.
	assert.Equal(t, "process-123-test-service-gen", changes.Unschedule[0].Name)
	assert.Equal(t, "process-123-test-service-gen", changes.Unschedule[1].Name)
	assert.Len(t, p.configCache, 0)
}
