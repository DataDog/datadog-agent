// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// DebugInfo contains a snapshot of the actuator's internal state for
// debugging purposes, exposed via the flare mechanism.
type DebugInfo struct {
	Processes        []ProcessDebugInfo  `json:"processes"`
	Programs         []ProgramDebugInfo  `json:"programs"`
	DiscoveredTypes  map[string][]string `json:"discovered_types"`
	CurrentlyLoading *ir.ProgramID       `json:"currently_loading"`
	QueuedLoading    []ir.ProgramID      `json:"queued_loading"`
	Counters         CountersDebugInfo   `json:"counters"`
	CircuitBreaker   CircuitBreakerInfo  `json:"circuit_breaker"`
}

// ProcessDebugInfo contains debug information about a single process in the
// actuator's state machine.
type ProcessDebugInfo struct {
	PID            int32        `json:"pid"`
	State          string       `json:"state"`
	Service        string       `json:"service"`
	CurrentProgram ir.ProgramID `json:"current_program"`
	ProbeCount     int          `json:"probe_count"`
}

// ProgramDebugInfo contains debug information about a single program in the
// actuator's state machine.
type ProgramDebugInfo struct {
	ProgramID          ir.ProgramID `json:"program_id"`
	State              string       `json:"state"`
	ProcessPID         int32        `json:"process_pid"`
	ProbeCount         int          `json:"probe_count"`
	NeedsRecompilation bool         `json:"needs_recompilation"`
}

// CountersDebugInfo contains cumulative counters from the actuator.
type CountersDebugInfo struct {
	Loaded                      uint64 `json:"loaded"`
	LoadFailed                  uint64 `json:"load_failed"`
	Attached                    uint64 `json:"attached"`
	AttachFailed                uint64 `json:"attach_failed"`
	Detached                    uint64 `json:"detached"`
	Unloaded                    uint64 `json:"unloaded"`
	TypeRecompilationsTriggered uint64 `json:"type_recompilations_triggered"`
}

// CircuitBreakerInfo contains the circuit breaker configuration.
type CircuitBreakerInfo struct {
	Interval          string  `json:"interval"`
	PerProbeCPULimit  float64 `json:"per_probe_cpu_limit"`
	AllProbesCPULimit float64 `json:"all_probes_cpu_limit"`
	InterruptOverhead string  `json:"interrupt_overhead"`
}

// debugInfo returns a snapshot of the state machine for debugging.
func (s *state) debugInfo() DebugInfo {
	processes := make([]ProcessDebugInfo, 0, len(s.processes))
	for _, p := range s.processes {
		processes = append(processes, ProcessDebugInfo{
			PID:            p.processID.PID,
			State:          p.state.String(),
			Service:        p.service,
			CurrentProgram: p.currentProgram,
			ProbeCount:     len(p.probes),
		})
	}

	programs := make([]ProgramDebugInfo, 0, len(s.programs))
	for _, p := range s.programs {
		programs = append(programs, ProgramDebugInfo{
			ProgramID:          p.id,
			State:              p.state.String(),
			ProcessPID:         p.processID.PID,
			ProbeCount:         len(p.config),
			NeedsRecompilation: p.needsRecompilation,
		})
	}

	discoveredTypes := make(map[string][]string, len(s.discoveredTypes))
	for svc, types := range s.discoveredTypes {
		discoveredTypes[svc] = slices.Clone(types)
	}

	var currentlyLoading *ir.ProgramID
	if s.currentlyLoading != nil {
		id := s.currentlyLoading.id
		currentlyLoading = &id
	}

	queuedLoading := make([]ir.ProgramID, 0, s.queuedLoading.len())
	for _, item := range s.queuedLoading.m {
		queuedLoading = append(queuedLoading, item.value.id)
	}

	return DebugInfo{
		Processes:        processes,
		Programs:         programs,
		DiscoveredTypes:  discoveredTypes,
		CurrentlyLoading: currentlyLoading,
		QueuedLoading:    queuedLoading,
		Counters: CountersDebugInfo{
			Loaded:                      s.counters.loaded,
			LoadFailed:                  s.counters.loadFailed,
			Attached:                    s.counters.attached,
			AttachFailed:                s.counters.attachFailed,
			Detached:                    s.counters.detached,
			Unloaded:                    s.counters.unloaded,
			TypeRecompilationsTriggered: s.counters.typeRecompilationsTriggered,
		},
		CircuitBreaker: CircuitBreakerInfo{
			Interval:          s.breakerCfg.Interval.String(),
			PerProbeCPULimit:  s.breakerCfg.PerProbeCPULimit,
			AllProbesCPULimit: s.breakerCfg.AllProbesCPULimit,
			InterruptOverhead: s.breakerCfg.InterruptOverhead.String(),
		},
	}
}
