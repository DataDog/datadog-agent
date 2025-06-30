// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
)

// effect represents a side effect that can be recorded and serialized to YAML
type effect interface {
	yamlTag() string
	yamlData() map[string]any
}

// Effect implementations

type effectSpawnEBPFCompilation struct {
	programID  ir.ProgramID
	executable Executable
	probes     []irgen.ProbeDefinition
}

func (e effectSpawnEBPFCompilation) yamlTag() string {
	return "!spawn-ebpf-compilation"
}

func (e effectSpawnEBPFCompilation) yamlData() map[string]any {
	var probeKeys []string
	for _, probe := range e.probes {
		probeKeys = append(probeKeys, probe.GetID())
	}
	slices.Sort(probeKeys)
	return map[string]any{
		"program_id": int(e.programID),
		"executable": e.executable.String(),
		"probes":     probeKeys,
	}
}

type effectSpawnBpfLoading struct {
	programID ir.ProgramID
}

func (e effectSpawnBpfLoading) yamlTag() string {
	return "!spawn-bpf-loading"
}

func (e effectSpawnBpfLoading) yamlData() map[string]any {
	return map[string]any{
		"program_id": int(e.programID),
	}
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
}

func (e effectDetachFromProcess) yamlTag() string {
	return "!detach-from-process"
}

func (e effectDetachFromProcess) yamlData() map[string]any {
	return map[string]any{
		"program_id": int(e.programID),
		"process_id": int(e.processID.PID),
	}
}

type effectRegisterProgramWithDispatcher struct {
	programID ir.ProgramID
}

func (e effectRegisterProgramWithDispatcher) yamlTag() string {
	return "!register-program-with-dispatcher"
}

func (e effectRegisterProgramWithDispatcher) yamlData() map[string]any {
	return map[string]any{
		"program_id": int(e.programID),
	}
}

type effectUnregisterProgramWithDispatcher struct {
	programID ir.ProgramID
}

func (e effectUnregisterProgramWithDispatcher) yamlTag() string {
	return "!unregister-program-with-dispatcher"
}

func (e effectUnregisterProgramWithDispatcher) yamlData() map[string]any {
	return map[string]any{
		"program_id": int(e.programID),
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

func (er *effectRecorder) compileProgram(
	programID ir.ProgramID,
	executable Executable,
	probes []irgen.ProbeDefinition,
) {
	er.recordEffect(effectSpawnEBPFCompilation{
		programID:  programID,
		executable: executable,
		probes:     probes,
	})
}

func (er *effectRecorder) loadProgram(compiled *CompiledProgram) {
	er.recordEffect(effectSpawnBpfLoading{
		programID: compiled.IR.ID,
	})
}

func (er *effectRecorder) attachToProcess(
	loaded *loadedProgram,
	executable Executable,
	processID ProcessID,
) {
	er.recordEffect(effectAttachToProcess{
		programID:  loaded.id,
		processID:  processID,
		executable: executable,
	})
}

func (er *effectRecorder) detachFromProcess(attached *attachedProgram) {
	er.recordEffect(effectDetachFromProcess{
		programID: attached.progID,
		processID: attached.procID,
	})
}

func (er *effectRecorder) registerProgramWithDispatcher(program *ir.Program) {
	er.recordEffect(effectRegisterProgramWithDispatcher{
		programID: program.ID,
	})
}

func (er *effectRecorder) unregisterProgramWithDispatcher(programID ir.ProgramID) {
	er.recordEffect(effectUnregisterProgramWithDispatcher{
		programID: programID,
	})
}
