// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package uprobes

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

type testMultiLink struct {
	closed int
	err    error
}

func (l *testMultiLink) Close() error { l.closed++; return l.err }

func multiTestAttacher(t *testing.T) *UprobeAttacher {
	t.Helper()
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{}, &MockManager{}, nil, AttacherDependencies{Inspector: &NativeBinaryInspector{}, ProcessMonitor: newMockProcessMonitor()})
	require.NoError(t, err)
	ua.UseUprobeMultiPrograms(map[string]*ebpf.Program{
		"uprobe__foo": {}, "uprobe__foo__return": {}, "uretprobe__foo": {},
	})
	return ua
}

func multiTestSelector(names ...string) manager.ProbesSelector {
	selector := &manager.AllOf{}
	for _, name := range names {
		selector.Selectors = append(selector.Selectors, &manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: name}})
	}
	return selector
}

func TestMultiUprobeGroupsAndReturnSemantics(t *testing.T) {
	ua := multiTestAttacher(t)
	var links []*testMultiLink
	var returns []bool
	var offsets [][]uint64
	ua.attachMulti = func(_ string, _ *ebpf.Program, locations []uint64, ret bool) (io.Closer, error) {
		l := &testMultiLink{}
		links = append(links, l)
		returns = append(returns, ret)
		offsets = append(offsets, locations)
		return l, nil
	}
	path := utils.FilePath{HostPath: "/test"}
	symbols := map[string]bininspect.FunctionMetadata{"foo": {EntryLocation: 10, ReturnLocations: []uint64{20, 30, 40}}}
	selector := multiTestSelector("uprobe__foo", "uprobe__foo__return", "uretprobe__foo")
	for range 2 {
		require.NoError(t, ua.attachProbeSelector(selector, path, "", &AttachRule{}, symbols))
	}
	require.Equal(t, [][]uint64{{10}, {20, 30, 40}, {10}}, offsets)
	require.Equal(t, []bool{false, false, true}, returns)
	require.Len(t, links, 3, "repeated attachment shares the original groups")
	links[0].err = errors.New("close failure")
	require.ErrorContains(t, ua.detachFromBinary(path), "close failure")
	for _, l := range links {
		require.Equal(t, 1, l.closed, "a failed close must not prevent other groups being closed")
	}
	require.Empty(t, ua.fileIDToMultiLinks)
}

func TestMultiUprobePrefixedReturnProbe(t *testing.T) {
	ua := multiTestAttacher(t)
	ua.multiPrograms["nodejs_uretprobe__SSL_read"] = &ebpf.Program{}
	var returnProbe bool
	ua.attachMulti = func(_ string, _ *ebpf.Program, offsets []uint64, ret bool) (io.Closer, error) {
		require.Equal(t, []uint64{42}, offsets)
		returnProbe = ret
		return &testMultiLink{}, nil
	}
	require.NoError(t, ua.attachProbeSelector(multiTestSelector("nodejs_uretprobe__SSL_read"), utils.FilePath{}, "", &AttachRule{}, map[string]bininspect.FunctionMetadata{
		"SSL_read": {EntryLocation: 42},
	}))
	require.True(t, returnProbe, "a protocol prefix must not turn a native return probe into an entry probe")
}

func TestMultiUprobeRegistryReferencesAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "shared inode and reattach", true: "partial attachment failure"}[fail], func(t *testing.T) {
			ua := multiTestAttacher(t)
			var links []*testMultiLink
			ua.attachMulti = func(_ string, _ *ebpf.Program, _ []uint64, _ bool) (io.Closer, error) {
				if fail && len(links) == 1 {
					return nil, errors.New("attach failure")
				}
				l := &testMultiLink{}
				links = append(links, l)
				return l, nil
			}
			path := filepath.Join(t.TempDir(), "binary")
			require.NoError(t, os.WriteFile(path, nil, 0600))
			selector := multiTestSelector("uprobe__foo", "uprobe__foo__return")
			attach := func(path utils.FilePath) error {
				return ua.attachProbeSelector(selector, path, "", &AttachRule{}, map[string]bininspect.FunctionMetadata{"foo": {EntryLocation: 1, ReturnLocations: []uint64{2, 3}}})
			}
			register := func(pid uint32) error {
				return ua.fileRegistry.Register(path, pid, attach, ua.detachFromBinary, utils.IgnoreCB)
			}
			t.Cleanup(ua.fileRegistry.Clear)
			if fail {
				require.ErrorContains(t, register(uint32(os.Getpid())), "attach failure")
				require.Len(t, links, 1)
				require.Equal(t, 1, links[0].closed)
				require.Empty(t, ua.fileIDToMultiLinks)
				return
			}
			require.NoError(t, register(uint32(os.Getpid())))
			require.ErrorIs(t, register(uint32(os.Getppid())), utils.ErrPathIsAlreadyRegistered)
			require.Len(t, links, 2)
			require.NoError(t, ua.fileRegistry.Unregister(uint32(os.Getpid())))
			require.Equal(t, 0, links[0].closed)
			require.NoError(t, ua.fileRegistry.Unregister(uint32(os.Getppid())))
			require.Equal(t, 1, links[0].closed)
			require.Equal(t, 1, links[1].closed)
			require.NoError(t, register(uint32(os.Getpid())))
			require.Len(t, links, 4)
			ua.fileRegistry.Clear()
			require.Equal(t, 1, links[2].closed)
			require.Equal(t, 1, links[3].closed)
		})
	}
}

func TestMultiUprobeRejectsMissingProgramOrLocations(t *testing.T) {
	ua := multiTestAttacher(t)
	selector := multiTestSelector("uprobe__foo__return")
	require.ErrorContains(t, ua.attachProbeSelector(selector, utils.FilePath{}, "", &AttachRule{}, map[string]bininspect.FunctionMetadata{"foo": {}}), "no probe locations")
	delete(ua.multiPrograms, "uprobe__foo__return")
	require.ErrorContains(t, ua.attachProbeSelector(selector, utils.FilePath{}, "", &AttachRule{}, map[string]bininspect.FunctionMetadata{"foo": {ReturnLocations: []uint64{1}}}), "missing multi-uprobe program")
}

func TestMultiProgramGraphIsolatesTailCalls(t *testing.T) {
	programs := map[string]*ebpf.ProgramSpec{
		"root":      {Type: ebpf.Kprobe, Instructions: asm.Instructions{asm.LoadMapPtr(asm.R1, 0).WithReference("routes"), asm.Mov.Imm(asm.R0, 42), asm.Return()}},
		"decoder":   {Type: ebpf.Kprobe, Instructions: asm.Instructions{asm.LoadMapPtr(asm.R1, 0).WithReference("data"), asm.LoadMapPtr(asm.R2, 0).WithReference("routes"), asm.Return()}},
		"unrelated": {Type: ebpf.SocketFilter},
	}
	maps := map[string]*ebpf.MapSpec{"routes": {Type: ebpf.ProgramArray, MaxEntries: 8}, "data": {Type: ebpf.Hash, MaxEntries: 123}}
	route := manager.TailCallRoute{ProgArrayName: "routes", Key: 1, ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "decoder"}}
	prepared, err := prepareMultiPrograms(programs, maps, []string{"root"}, []manager.TailCallRoute{route})
	require.NoError(t, err)
	require.Len(t, prepared.spec.Programs, 2)
	require.Equal(t, ebpf.AttachTraceUprobeMulti, prepared.spec.Programs["root"].AttachType)
	require.Equal(t, ebpf.AttachTraceUprobeMulti, prepared.spec.Programs["decoder"].AttachType)
	require.Zero(t, programs["root"].AttachType, "original perf graph must remain unchanged")
	require.Equal(t, int64(42), prepared.spec.Programs["root"].Instructions[1].Constant, "preserve patched constants")
	require.NotSame(t, maps["routes"], prepared.spec.Maps["routes"])
	require.Equal(t, uint32(123), prepared.spec.Maps["data"].MaxEntries)
	require.Equal(t, []manager.TailCallRoute{route}, prepared.routes)
	_, err = prepareMultiPrograms(programs, maps, []string{"missing"}, nil)
	require.Error(t, err)
	route.Program = &ebpf.Program{}
	_, err = prepareMultiPrograms(programs, maps, []string{"root"}, []manager.TailCallRoute{route})
	require.ErrorContains(t, err, "external tail-call")
}
