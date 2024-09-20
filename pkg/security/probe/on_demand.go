// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

// OnDemandProbesManager is the manager for on-demand probes
type OnDemandProbesManager struct {
	sync.RWMutex

	probe *EBPFProbe

	hookPoints   []rules.OnDemandHookPoint
	manager      *manager.Manager
	probes       []*manager.Probe
	probeCounter uint16
}

func (sm *OnDemandProbesManager) disable() {
	sm.Lock()
	defer sm.Unlock()

	sm.hookPoints = nil

	for _, p := range sm.probes {
		if err := sm.manager.DetachHook(p.ProbeIdentificationPair); err != nil {
			seclog.Errorf("error disabling on-demand probe: %v", err)
		}
	}
}

func (sm *OnDemandProbesManager) setHookPoints(hps []rules.OnDemandHookPoint) {
	sm.Lock()
	defer sm.Unlock()

	sm.hookPoints = hps
}

func (sm *OnDemandProbesManager) getHookNameFromID(id int) string {
	sm.RLock()
	defer sm.RUnlock()

	if id >= len(sm.hookPoints) {
		return ""
	}

	return sm.hookPoints[id].Name
}

func (sm *OnDemandProbesManager) updateProbes() {
	sm.Lock()
	defer sm.Unlock()

	sm.probes = make([]*manager.Probe, 0)
	for hookID, hookPoint := range sm.hookPoints {
		var baseProbe *manager.Probe
		if hookPoint.IsSyscall && sm.probe.useSyscallWrapper {
			baseProbe = probes.GetOnDemandSyscallProbe()
		} else {
			baseProbe = probes.GetOnDemandRegularProbe()
		}

		newProbe := baseProbe.Copy()
		newProbe.CopyProgram = true
		newProbe.UID = fmt.Sprintf("%s_%s_on_demand_%d", probes.SecurityAgentUID, hookPoint.Name, sm.probeCounter)
		sm.probeCounter++
		newProbe.KeepProgramSpec = false
		if hookPoint.IsSyscall {
			newProbe.HookFuncName = probes.GetSyscallFnName(hookPoint.Name)
		} else {
			newProbe.HookFuncName = hookPoint.Name
		}

		argsEditors := buildArgsEditors(hookPoint.Args)
		argsEditors = append(argsEditors, manager.ConstantEditor{
			Name:  "synth_id",
			Value: uint64(hookID),
		})

		editor := func(spec *ebpf.ProgramSpec) {
			spec.AttachTo = newProbe.HookFuncName
		}

		if err := sm.manager.CloneProgramWithSpecEditor(probes.SecurityAgentUID, newProbe, argsEditors, nil, editor); err != nil {
			seclog.Errorf("error cloning on-demand probe: %v", err)
		}
		sm.probes = append(sm.probes, newProbe)
	}
}

func (sm *OnDemandProbesManager) selectProbes() manager.ProbesSelector {
	sm.RLock()
	defer sm.RUnlock()

	var activatedProbes manager.BestEffort
	for _, p := range sm.probes {
		activatedProbes.Selectors = append(activatedProbes.Selectors, &manager.ProbeSelector{
			ProbeIdentificationPair: p.ProbeIdentificationPair,
		})
	}
	return &activatedProbes
}

// onDemandParamKind needs to stay in sync with `enum param_kind_t`
// from pkg/security/ebpf/c/include/hooks/on_demand.h
type onDemandParamKind int

const (
	onDemandParamKindNoAction onDemandParamKind = iota
	onDemandParamKindInt
	onDemandParamKindNullTerminatedString
)

func buildArgsEditors(args []rules.HookPointArg) []manager.ConstantEditor {
	argKinds := make(map[int]onDemandParamKind)
	for _, arg := range args {
		kind := onDemandParamKindNoAction
		switch arg.Kind {
		case "uint":
			kind = onDemandParamKindInt
		case "null-terminated-string":
			kind = onDemandParamKindNullTerminatedString
		default:
			seclog.Errorf("unknown kind for arg: %s", arg.Kind)
		}

		argKinds[arg.N] = kind
	}

	editors := make([]manager.ConstantEditor, 0, len(argKinds))
	for n, kind := range argKinds {
		editors = append(editors, manager.ConstantEditor{
			Name:  fmt.Sprintf("param%dkind", n),
			Value: uint64(kind),
		})
	}
	return editors
}
