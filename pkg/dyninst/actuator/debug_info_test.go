// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	procinfo "github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

func TestStateDebugInfoEmpty(t *testing.T) {
	s := newState(Config{DiscoveredTypesLimit: 512})
	info := s.debugInfo()

	assert.Empty(t, info.Processes)
	assert.Empty(t, info.Programs)
	assert.Empty(t, info.DiscoveredTypes)
	assert.Nil(t, info.CurrentlyLoading)
	assert.Empty(t, info.QueuedLoading)
	assert.Equal(t, uint64(0), info.Counters.Loaded)
}

func TestStateDebugInfoWithProcessesAndPrograms(t *testing.T) {
	s := newState(Config{DiscoveredTypesLimit: 512})

	pid := procinfo.ID{PID: 42}
	probe := &rcjson.SnapshotProbe{
		LogProbeCommon: rcjson.LogProbeCommon{
			ProbeCommon: rcjson.ProbeCommon{
				ID:      "probe-1",
				Version: 1,
				Where:   &rcjson.Where{MethodName: "main"},
			},
		},
	}
	s.processes[pid] = &process{
		processID: pid,
		state:     processStateAttached,
		service:   "my-service",
		probes:    map[probeKey]ir.ProbeDefinition{{id: "probe-1", version: 1}: probe},
	}

	progID := ir.ProgramID(1)
	s.programs[progID] = &program{
		id:        progID,
		state:     programStateLoaded,
		processID: pid,
		config:    []ir.ProbeDefinition{probe},
	}
	s.processes[pid].currentProgram = progID

	s.discoveredTypes["my-service"] = []string{"MyType", "OtherType"}

	info := s.debugInfo()

	assert.Len(t, info.Processes, 1)
	assert.Equal(t, int32(42), info.Processes[0].PID)
	assert.Equal(t, "Attached", info.Processes[0].State)
	assert.Equal(t, "my-service", info.Processes[0].Service)
	assert.Equal(t, 1, info.Processes[0].ProbeCount)
	assert.Equal(t, progID, info.Processes[0].CurrentProgram)

	assert.Len(t, info.Programs, 1)
	assert.Equal(t, progID, info.Programs[0].ProgramID)
	assert.Equal(t, "Loaded", info.Programs[0].State)
	assert.Equal(t, int32(42), info.Programs[0].ProcessPID)
	assert.Equal(t, 1, info.Programs[0].ProbeCount)
	assert.False(t, info.Programs[0].NeedsRecompilation)

	assert.Equal(t, []string{"MyType", "OtherType"}, info.DiscoveredTypes["my-service"])
	assert.Nil(t, info.CurrentlyLoading)
	assert.Empty(t, info.QueuedLoading)
}

func TestStateDebugInfoCounters(t *testing.T) {
	s := newState(Config{DiscoveredTypesLimit: 512})
	s.counters.loaded = 5
	s.counters.loadFailed = 2
	s.counters.attached = 3
	s.counters.typeRecompilationsTriggered = 1

	info := s.debugInfo()

	assert.Equal(t, uint64(5), info.Counters.Loaded)
	assert.Equal(t, uint64(2), info.Counters.LoadFailed)
	assert.Equal(t, uint64(3), info.Counters.Attached)
	assert.Equal(t, uint64(1), info.Counters.TypeRecompilationsTriggered)
}

func TestStateDebugInfoQueuedPrograms(t *testing.T) {
	s := newState(Config{DiscoveredTypesLimit: 512})

	pid := procinfo.ID{PID: 10}
	prog := &program{
		id:        ir.ProgramID(7),
		state:     programStateQueued,
		processID: pid,
	}
	s.programs[prog.id] = prog
	s.queuedLoading.pushBack(prog)

	info := s.debugInfo()

	assert.Len(t, info.QueuedLoading, 1)
	assert.Equal(t, ir.ProgramID(7), info.QueuedLoading[0])
	assert.Nil(t, info.CurrentlyLoading)
}

func TestStateDebugInfoCurrentlyLoading(t *testing.T) {
	s := newState(Config{DiscoveredTypesLimit: 512})
	s.currentlyLoading = &program{id: ir.ProgramID(3)}

	info := s.debugInfo()

	assert.NotNil(t, info.CurrentlyLoading)
	assert.Equal(t, ir.ProgramID(3), *info.CurrentlyLoading)
}
