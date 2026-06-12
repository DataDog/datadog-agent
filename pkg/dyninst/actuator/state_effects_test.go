// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// effect represents a side effect that can be recorded and serialized to YAML
type effect interface {
	yamlTag() string
	yamlData() map[string]any
}

// Effect implementations

type effectSpawnBpfLoading struct {
	processID                ProcessID
	programID                ir.ProgramID
	executable               Executable
	probes                   []ir.ProbeDefinition
	additionalTypes          []string
	skipRuntimeRecoveryProbe bool
}

func (e effectSpawnBpfLoading) yamlTag() string {
	return "!spawn-bpf-loading"
}

func (e effectSpawnBpfLoading) yamlData() map[string]any {
	var probeKeys []string
	for _, probe := range e.probes {
		probeKeys = append(probeKeys, probe.GetID())
	}
	slices.Sort(probeKeys)
	data := map[string]any{
		"process_id": int(e.processID.PID),
		"program_id": int(e.programID),
		"executable": e.executable.String(),
		"probes":     probeKeys,
	}
	if len(e.additionalTypes) > 0 {
		data["additional_types"] = e.additionalTypes
	}
	if e.skipRuntimeRecoveryProbe {
		data["skip_runtime_recovery_probe"] = true
	}
	return data
}

type effectAttachToProcess struct {
	programID  ir.ProgramID
	processID  ProcessID
	executable Executable
}

func (e effectAttachToProcess) yamlTag() string {
	return "!attach-to-process"
}

func (e effectAttachToProcess) yamlData() map[string]any {
	return map[string]any{
		"program_id": int(e.programID),
		"process_id": int(e.processID.PID),
		"executable": e.executable.String(),
	}
}

type effectDetachFromProcess struct {
	programID ir.ProgramID
	processID ProcessID
	failure   error
}

func (e effectDetachFromProcess) yamlTag() string {
	return "!detach-from-process"
}

func (e effectDetachFromProcess) yamlData() map[string]any {
	failureStr := "no"
	if e.failure != nil {
		failureStr = "yes"
	}
	return map[string]any{
		"program_id": int(e.programID),
		"process_id": int(e.processID.PID),
		"failure":    failureStr,
	}
}

type effectUnloadProgram struct {
	programID ir.ProgramID
}

func (e effectUnloadProgram) yamlTag() string {
	return "!unload-program"
}

func (e effectUnloadProgram) yamlData() map[string]any {
	return map[string]any{
		"program_id": int(e.programID),
	}
}

type effectReportProbeError struct {
	programID ir.ProgramID
	processID ProcessID
	probeID   string
	hasReason bool
}

func (e effectReportProbeError) yamlTag() string {
	return "!report-probe-error"
}

func (e effectReportProbeError) yamlData() map[string]any {
	reason := "no"
	if e.hasReason {
		reason = "yes"
	}
	return map[string]any{
		"program_id": int(e.programID),
		"process_id": int(e.processID.PID),
		"probe_id":   e.probeID,
		"reason":     reason,
	}
}

// effectRecorder records effects for testing
type effectRecorder struct {
	effects []effect
}

func (er *effectRecorder) recordEffect(eff effect) {
	er.effects = append(er.effects, eff)
}

// Convert effects to YAML nodes for snapshot testing
func (er *effectRecorder) yamlNodes() ([]*yaml.Node, error) {
	var nodes []*yaml.Node
	for _, eff := range er.effects {
		var node yaml.Node
		if err := node.Encode(eff.yamlData()); err != nil {
			return nil, fmt.Errorf("failed to marshal effect to YAML: %v", err)
		}
		node.Tag = eff.yamlTag()
		node.Kind = yaml.MappingNode
		node.Style = yaml.FlowStyle
		nodes = append(nodes, &node)
	}
	return nodes, nil
}

// Implementation of effectHandler interface using the unified system

func (er *effectRecorder) loadProgram(
	programID ir.ProgramID,
	executable Executable,
	processID ProcessID,
	probes []ir.ProbeDefinition,
	opts LoadOptions,
) {
	er.recordEffect(effectSpawnBpfLoading{
		processID:                processID,
		programID:                programID,
		executable:               executable,
		probes:                   probes,
		additionalTypes:          opts.AdditionalTypes,
		skipRuntimeRecoveryProbe: opts.SkipRuntimeRecoveryProbe,
	})
}

func (er *effectRecorder) attachToProcess(
	loaded *loadedProgram,
	executable Executable,
	processID ProcessID,
) {
	er.recordEffect(effectAttachToProcess{
		programID:  loaded.programID,
		processID:  processID,
		executable: executable,
	})
}

func (er *effectRecorder) detachFromProcess(attached *attachedProgram, failure error) {
	er.recordEffect(effectDetachFromProcess{
		programID: attached.programID,
		processID: attached.processID,
		failure:   failure,
	})
}

func (er *effectRecorder) unloadProgram(lp *loadedProgram) {
	// For tests we just record that the sink and program are being closed.
	er.recordEffect(effectUnloadProgram{
		programID: lp.programID,
	})
}

func (er *effectRecorder) reportProbeError(
	ap *attachedProgram, probe ir.ProbeDefinition, reason error,
) {
	er.recordEffect(effectReportProbeError{
		programID: ap.programID,
		processID: ap.processID,
		probeID:   probe.GetID(),
		hasReason: reason != nil,
	})
}
