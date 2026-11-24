// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package uprobes

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func TestUprobeBackendGrouping(t *testing.T) {
	for _, tc := range []struct {
		name               string
		multiFoo, multiBar bool
		want               [][]uint64
	}{
		{name: "legacy", want: [][]uint64{{10}, {20}, {30}, {40}, {10}, {50}, {60}}},
		{name: "multi", multiFoo: true, multiBar: true, want: [][]uint64{{10}, {20, 30, 40}, {10}, {50, 60}}},
		{name: "mixed", multiFoo: true, want: [][]uint64{{10}, {20, 30, 40}, {10}, {50}, {60}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &MockManager{}
			for _, program := range []struct {
				name  string
				multi bool
			}{{"uprobe__foo__return", tc.multiFoo}, {"uprobe__bar__return", tc.multiBar}} {
				spec := &ebpf.ProgramSpec{}
				if program.multi {
					spec.AttachType = ebpf.AttachTraceUprobeMulti
				}
				mgr.On("GetProgramSpec", manager.ProbeIdentificationPair{EBPFFuncName: program.name}).Return([]*ebpf.ProgramSpec{spec}, true, nil)
			}
			var attached []*manager.Probe
			mgr.On("GetProbe", mock.Anything).Return(&manager.Probe{}, false)
			mgr.On("AddHook", "", mock.Anything).Run(func(args mock.Arguments) { attached = append(attached, args.Get(1).(*manager.Probe)) }).Return(nil)
			mgr.On("DetachHook", mock.Anything).Return(nil)
			ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{}, mgr, nil, AttacherDependencies{Inspector: &NativeBinaryInspector{}, ProcessMonitor: newMockProcessMonitor()})
			require.NoError(t, err)
			selector := &manager.AllOf{}
			for _, name := range []string{"uprobe__foo", "uprobe__foo__return", "nodejs_uretprobe__foo", "uprobe__bar__return"} {
				selector.Selectors = append(selector.Selectors, &manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}})
			}
			path := utils.FilePath{HostPath: "/test"}
			require.NoError(t, ua.attachProbeSelector(selector, path, "binary", &AttachRule{}, map[string]bininspect.FunctionMetadata{
				"foo": {EntryLocation: 10, ReturnLocations: []uint64{20, 30, 40}},
				"bar": {ReturnLocations: []uint64{50, 60}},
			}))
			require.Len(t, attached, len(tc.want))
			for i, offsets := range tc.want {
				require.Equal(t, offsets, attached[i].UprobeOffsets)
			}
			// Every group remains a normal manager hook, preserving registry cleanup,
			// selectors and callbacks instead of owning kernel links in the Agent.
			require.NoError(t, ua.detachFromBinary(path))
			mgr.AssertNumberOfCalls(t, "DetachHook", len(attached))
			mgr.AssertExpectations(t)
		})
	}
}
