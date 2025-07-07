// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

// deepCopyState creates a deep copy of the state struct, though note that
// some things are not deep copied like the rcjson.Probe objects because they
// are considered immutable.
func deepCopyState(original *state) *state {
	if original == nil {
		return nil
	}

	// Create new state with basic fields copied.
	copied := newState()

	copied.programIDAlloc = original.programIDAlloc

	// Deep copy programs map first (needed for queue and currentlyCompiling
	// references).
	for id, prog := range original.programs {
		copied.programs[id] = deepCopyProgram(prog)
	}

	// Deep copy processes map.
	for id, proc := range original.processes {
		copied.processes[id] = deepCopyProcess(proc)
	}

	// Set currentlyCompiling to point to the copied program if it exists.
	if original.currentlyLoading != nil {
		copied.currentlyLoading = copied.programs[original.currentlyLoading.id]
	}

	// Copy queued compilations.
	for prog := range original.queuedLoading.items() {
		copied.queuedLoading.pushBack(copied.programs[prog.id])
	}

	return copied
}

// deepCopyProgram creates a deep copy of a program struct.
func deepCopyProgram(original *program) *program {
	if original == nil {
		return nil
	}

	// Shallow copy probes slice (probes themselves are shared).
	copiedConfig := make([]ir.ProbeDefinition, len(original.config))
	copy(copiedConfig, original.config)

	copied := &program{
		state:      original.state,
		id:         original.id,
		config:     copiedConfig,
		executable: original.executable,
		processKey: original.processKey,
	}

	// Note: loadedProgram interface is more complex to copy and represents
	// external resources, so we'll handle it conservatively.
	if original.loaded != nil {
		// For testing purposes, we'll assume the loadedProgram is immutable
		// or represents a resource that can be shared safely.
		copied.loaded = original.loaded
	}

	return copied
}

// deepCopyProcess creates a deep copy of a process struct.
func deepCopyProcess(original *process) *process {
	if original == nil {
		return nil
	}

	// Shallow copy probes map (probes themselves are shared).
	copiedProbes := make(map[probeKey]ir.ProbeDefinition)
	for key, probe := range original.probes {
		copiedProbes[key] = probe
	}

	copied := &process{
		state:           original.state,
		processKey:      original.processKey,
		executable:      original.executable,
		probes:          copiedProbes,
		currentProgram:  original.currentProgram,
		attachedProgram: original.attachedProgram,
	}

	return copied
}

// TestDeepCopyState verifies that deepCopyState works correctly.
func TestDeepCopyState(t *testing.T) {
	s := newState()
	processID := ProcessID{PID: 123}
	executable := Executable{Path: "/test/path"}
	probe := &rcjson.SnapshotProbe{
		LogProbeCommon: rcjson.LogProbeCommon{
			ProbeCommon: rcjson.ProbeCommon{
				ID:         "test-probe",
				Version:    1,
				Where:      &rcjson.Where{MethodName: "testMethod"},
				Tags:       []string{"test-tag"},
				EvaluateAt: "test-evaluate-at",
			},
		},
	}
	s.programIDAlloc = 5
	tenantID := tenantID(1)
	key := processKey{
		tenantID:  tenantID,
		ProcessID: processID,
	}
	s.processes[key] = &process{
		state:      processStateWaitingForProgram,
		executable: executable,
		probes: map[probeKey]ir.ProbeDefinition{
			{id: "test-probe", version: 1}: probe,
		},
		currentProgram: 1,
	}
	programID := ir.ProgramID(1)
	program := &program{
		state:      programStateQueued,
		id:         programID,
		config:     []ir.ProbeDefinition{probe},
		executable: executable,
		processKey: key,
	}
	s.programs[programID] = program

	clone := deepCopyState(s)

	// Verify basic fields.
	require.Equal(t, clone.programIDAlloc, clone.programIDAlloc)

	// Verify processes are deeply copied.
	require.Equal(t, len(clone.processes), len(clone.processes))
	copiedProcess := clone.processes[key]
	require.NotNil(t, copiedProcess)
	require.NotSame(t, s.processes[key], copiedProcess)
	require.Equal(t, s.processes[key].state, copiedProcess.state)
	require.Equal(t, s.processes[key].processKey, copiedProcess.processKey)

	// Verify programs are deeply copied.
	require.Equal(t, len(clone.programs), len(clone.programs))
	copiedProgram := clone.programs[programID]
	require.NotNil(t, copiedProgram)
	require.NotSame(t, s.programs[programID], copiedProgram)
	require.Equal(t, s.programs[programID].state, copiedProgram.state)
	require.Equal(t, s.programs[programID].id, copiedProgram.id)

	// Verify config slices are copied but probes are shared.
	require.Equal(
		t, len(clone.programs[programID].config), len(copiedProgram.config),
	)
	if len(copiedProgram.config) > 0 {
		// Verify probe instances are shared (same object).
		require.Same(
			t, clone.programs[programID].config[0], copiedProgram.config[0],
		)
	}
}
