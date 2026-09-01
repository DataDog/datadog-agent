// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf && test

package uprobes

import (
	"os"
	"strings"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	ciliumebpf "github.com/cilium/ebpf"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

// TestCanUseMultiAttach verifies the kernel gate resolves to true on a supported host. The CI
// kernel matrix is >= 6.10, which is the floor uprobe_multi requires.
func TestCanUseMultiAttach(t *testing.T) {
	require.True(t, CanUseMultiAttach(), "expected uprobe_multi support on kernel >= 6.10")
}

// markUprobesMultiAttach mirrors USM's setMultiAttachType: it marks every uprobe program with
// expected_attach_type BPF_TRACE_UPROBE_MULTI, which is a load-time property and a precondition
// for attaching the program with a uprobe_multi link.
func markUprobesMultiAttach(m *manager.Manager) error {
	specs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if spec != nil && strings.HasPrefix(spec.SectionName, "uprobe/") {
			spec.AttachType = ciliumebpf.AttachTraceUprobeMulti
		}
	}
	return nil
}

// TestAttachToBinaryWithMultiAttach exercises the uprobe_multi attach path end to end. A real
// program is loaded and marked for uprobe_multi, the attacher attaches it to a real binary through
// a single link rather than a per-probe perf event, and detaching closes that link. This is the
// path whose whole purpose is to make system-probe shutdown independent of the attachment count.
func TestAttachToBinaryWithMultiAttach(t *testing.T) {
	if !CanUseMultiAttach() {
		t.Skip("uprobe_multi not supported on this kernel")
	}

	// No probes are declared on the manager: we want the program loaded so the attacher can look
	// it up, but not activated by the manager itself, since the attacher owns attachment.
	mgr := &manager.Manager{}
	mgr.InstructionPatchers = append(mgr.InstructionPatchers, markUprobesMultiAttach)

	inspector := &MockBinaryInspector{}
	config := AttacherConfig{
		Rules: []*AttachRule{
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__main"}},
				},
			},
		},
		EnableMultiAttach: true,
	}

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, mgr, nil, AttacherDependencies{Inspector: inspector, ProcessMonitor: newMockProcessMonitor()})
	require.NoError(t, err)
	require.NotNil(t, ua)

	require.NoError(t, ddebpf.LoadCOREAsset("uprobe_attacher-test.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		require.NoError(t, mgr.InitWithOptions(buf, opts))
		require.NoError(t, mgr.Start())
		t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })
		return nil
	}))

	// Attach to the test binary itself, which is a real ELF we can open and attach to.
	target := utils.FilePath{HostPath: "/proc/self/exe", PID: uint32(os.Getpid())}

	inspector.On("Inspect", mock.Anything, mock.Anything).Return(map[int]*InspectionResult{
		0: {SymbolMap: map[string]bininspect.FunctionMetadata{"main": {EntryLocation: 0x1000}}},
	}, nil)
	inspector.On("Cleanup", mock.Anything).Return(nil)

	require.NoError(t, ua.attachToBinary(target, config.Rules, NewProcInfo(ua.config.ProcRoot, target.PID)))

	// The multi path must have been taken: a single link keyed by program name, and nothing on the
	// per-probe bookkeeping.
	links := ua.fileIDToMultiLinks[target.ID]
	require.Len(t, links, 1, "expected exactly one uprobe_multi link")
	l, ok := links["uprobe__main"]
	require.True(t, ok, "expected a link keyed by the program name")
	require.NotNil(t, l)
	require.Empty(t, ua.fileIDToAttachedProbes[target.ID], "per-probe attach path must not be used")

	// Detaching should close the link and clear the bookkeeping.
	require.NoError(t, ua.detachFromBinary(utils.FilePath{ID: target.ID}))
	require.Empty(t, ua.fileIDToMultiLinks[target.ID], "expected links to be cleared after detach")
}
