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
	"github.com/cilium/ebpf/features"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestCanUseMultiAttach asserts that CanUseMultiAttach agrees with the two things it gates on: the
// uprobe_multi feature probe and the >= 6.10 kernel floor (or the DD_USM_FORCE_UPROBE_MULTI
// override). Asserting consistency rather than a hard true keeps this test meaningful on every
// kernel in the CI matrix, which spans well below 6.10, instead of red-failing on older hosts.
func TestCanUseMultiAttach(t *testing.T) {
	v, err := kernel.HostVersion()
	require.NoError(t, err)
	featureSupported := features.HaveBPFLinkUprobeMulti() == nil
	// Mirror canUseMultiAttach exactly: DD_USM_ENABLE_UPROBE_MULTI gates everything, and both
	// switches are parsed with strconv.ParseBool semantics (via uprobeMultiBoolEnv), not an exact
	// "true", so the expectation stays correct whatever value operators set.
	enabled := uprobeMultiBoolEnv("DD_USM_ENABLE_UPROBE_MULTI", true)
	forced := uprobeMultiBoolEnv("DD_USM_FORCE_UPROBE_MULTI", false)
	want := enabled && featureSupported && (forced || v >= multiAttachMinKernel)
	require.Equal(t, want, CanUseMultiAttach())
}

// markUprobesMultiAttach mirrors USM's setMultiAttachType: it marks every uprobe and uretprobe
// program with expected_attach_type BPF_TRACE_UPROBE_MULTI, which is a load-time property and a
// precondition for attaching the program with a uprobe_multi link. Both section prefixes must be
// covered because production (ebpf_main.go setMultiAttachType) marks both, and real uretprobes
// (uretprobe__SSL_*, nodejs_uretprobe__*, istio_uretprobe__*) take the UretprobeMulti branch.
func markUprobesMultiAttach(m *manager.Manager) error {
	specs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if spec != nil && (strings.HasPrefix(spec.SectionName, "uprobe/") || strings.HasPrefix(spec.SectionName, "uretprobe/")) {
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

// TestAttachUretprobeWithMultiAttach exercises the UretprobeMulti branch of the multi-attach path,
// which every real uretprobe (uretprobe__SSL_*, nodejs_uretprobe__*, istio_uretprobe__*) takes.
// The plain-uprobe branch is covered by TestAttachToBinaryWithMultiAttach; a return probe is routed
// differently (its uretprobe/ section -> ex.UretprobeMulti), so it needs its own coverage.
func TestAttachUretprobeWithMultiAttach(t *testing.T) {
	if !CanUseMultiAttach() {
		t.Skip("uprobe_multi not supported on this kernel")
	}

	mgr := &manager.Manager{}
	mgr.InstructionPatchers = append(mgr.InstructionPatchers, markUprobesMultiAttach)

	inspector := &MockBinaryInspector{}
	config := AttacherConfig{
		Rules: []*AttachRule{
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uretprobe__SSL_connect"}},
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

	target := utils.FilePath{HostPath: "/proc/self/exe", PID: uint32(os.Getpid())}

	// A real uretprobe program is attached at the function entry offset; the kernel turns it into a
	// return probe, so the entry location is what the multi link is given.
	inspector.On("Inspect", mock.Anything, mock.Anything).Return(map[int]*InspectionResult{
		0: {SymbolMap: map[string]bininspect.FunctionMetadata{"SSL_connect": {EntryLocation: 0x1000}}},
	}, nil)
	inspector.On("Cleanup", mock.Anything).Return(nil)

	require.NoError(t, ua.attachToBinary(target, config.Rules, NewProcInfo(ua.config.ProcRoot, target.PID)))

	links := ua.fileIDToMultiLinks[target.ID]
	require.Len(t, links, 1, "expected exactly one uprobe_multi link")
	l, ok := links["uretprobe__SSL_connect"]
	require.True(t, ok, "expected a link keyed by the uretprobe program name")
	require.NotNil(t, l)
	require.Empty(t, ua.fileIDToAttachedProbes[target.ID], "per-probe attach path must not be used")

	require.NoError(t, ua.detachFromBinary(utils.FilePath{ID: target.ID}))
	require.Empty(t, ua.fileIDToMultiLinks[target.ID], "expected links to be cleared after detach")
}

// TestMultiAttachZeroLocationsIsError covers the multi path's zero-location guard: a mandatory
// manual-return probe whose symbol resolves but has no return locations must produce an error, the
// way the per-probe path fails selector validation, rather than being silently recorded as a
// successful attach with no probes. Uses a mock manager, so it needs no eBPF assets and runs on any
// kernel (useMultiAttach is forced on).
func TestMultiAttachZeroLocationsIsError(t *testing.T) {
	rule := &AttachRule{
		Targets: AttachToExecutable,
		ProbesSelector: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_do_handshake__return"}},
		},
		// Override the parsed options so the test does not depend on name parsing: a mandatory
		// manual-return probe on symbol SSL_do_handshake.
		ProbeOptionsOverride: map[string]ProbeOptions{
			"uprobe__SSL_do_handshake__return": {Symbol: "SSL_do_handshake", IsManualReturn: true},
		},
	}
	config := AttacherConfig{Rules: []*AttachRule{rule}, EnableMultiAttach: true}

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, AttacherDependencies{ProcessMonitor: newMockProcessMonitor()})
	require.NoError(t, err)
	ua.useMultiAttach = true // exercise the multi path regardless of host kernel

	fpath := utils.FilePath{HostPath: "/proc/self/exe", PID: uint32(os.Getpid())}
	// The symbol is found, but the manual-return probe has no return locations.
	inspectResult := map[string]bininspect.FunctionMetadata{"SSL_do_handshake": {ReturnLocations: nil}}

	err = ua.attachProbeSelector(rule.ProbesSelector[0], fpath, "testuid", rule, inspectResult)
	require.Error(t, err, "a mandatory rule with zero attach locations must error on the multi path")
	require.Contains(t, err.Error(), "no attach locations")
}

// TestUprobeMultiBoolEnv covers the env parsing behind DD_USM_ENABLE_UPROBE_MULTI (the rollback
// switch) and DD_USM_FORCE_UPROBE_MULTI: unset returns the default, and the value is parsed with
// strconv.ParseBool semantics rather than an exact "true", with unparseable values falling back to
// the default.
func TestUprobeMultiBoolEnv(t *testing.T) {
	const key = "DD_USM_TEST_BOOL_ENV"
	tests := []struct {
		name  string
		set   bool
		value string
		def   bool
		want  bool
	}{
		{name: "unset uses default true", set: false, def: true, want: true},
		{name: "unset uses default false", set: false, def: false, want: false},
		{name: "true", set: true, value: "true", def: false, want: true},
		{name: "false", set: true, value: "false", def: true, want: false},
		{name: "numeric 1", set: true, value: "1", def: false, want: true},
		{name: "numeric 0", set: true, value: "0", def: true, want: false},
		{name: "uppercase TRUE", set: true, value: "TRUE", def: false, want: true},
		{name: "unparseable falls back to default", set: true, value: "yes", def: true, want: true},
		{name: "empty falls back to default", set: true, value: "", def: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(key, tt.value)
			}
			require.Equal(t, tt.want, uprobeMultiBoolEnv(key, tt.def))
		})
	}
}
