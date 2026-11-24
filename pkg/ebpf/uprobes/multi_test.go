// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package uprobes

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func TestUprobeBackendGrouping(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "multi"}[multi], func(t *testing.T) {
			mgr := &MockManager{}
			var attached []*manager.Probe
			mgr.On("GetProbe", mock.Anything).Return(&manager.Probe{}, false)
			mgr.On("AddHook", "", mock.Anything).Run(func(args mock.Arguments) { attached = append(attached, args.Get(1).(*manager.Probe)) }).Return(nil)
			mgr.On("DetachHook", mock.Anything).Return(nil)
			ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{}, mgr, nil, AttacherDependencies{Inspector: &NativeBinaryInspector{}, ProcessMonitor: newMockProcessMonitor()})
			require.NoError(t, err)
			ua.UseUprobeMulti(multi)
			selector := &manager.AllOf{}
			for _, name := range []string{"uprobe__foo", "uprobe__foo__return", "nodejs_uretprobe__foo"} {
				selector.Selectors = append(selector.Selectors, &manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}})
			}
			path := utils.FilePath{HostPath: "/test"}
			require.NoError(t, ua.attachProbeSelector(selector, path, "binary", &AttachRule{}, map[string]bininspect.FunctionMetadata{"foo": {EntryLocation: 10, ReturnLocations: []uint64{20, 30, 40}}}))
			if multi {
				require.Len(t, attached, 3)
				require.Equal(t, []uint64{10}, attached[0].UprobeOffsets)
				require.Equal(t, []uint64{20, 30, 40}, attached[1].UprobeOffsets)
				require.Equal(t, []uint64{10}, attached[2].UprobeOffsets)
			} else {
				require.Len(t, attached, 5)
				for _, p := range attached {
					require.Empty(t, p.UprobeOffsets)
				}
			}
			// Every group remains a normal manager hook, preserving registry cleanup,
			// selectors and callbacks instead of owning kernel links in the Agent.
			require.NoError(t, ua.detachFromBinary(path))
			mgr.AssertNumberOfCalls(t, "DetachHook", len(attached))
		})
	}
}
