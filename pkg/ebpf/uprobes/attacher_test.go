// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// === Tests

func TestCanCreateAttacher(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{}, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)
}

func TestAttachPidExcludesInternal(t *testing.T) {
	exe := "datadog-agent/bin/system-probe"
	procRoot := createFakeProcFS(t, []FakeProcFSEntry{{pid: 1, cmdline: exe, command: exe, exe: exe}})
	config := &AttacherConfig{
		ExcludeTargets: ExcludeInternal,
		ProcRoot:       procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(1, false)
	require.ErrorIs(t, err, ErrInternalDDogProcessRejected)
}

func TestAttachPidExcludesSelf(t *testing.T) {
	config := &AttacherConfig{
		ExcludeTargets: ExcludeSelf,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(uint32(os.Getpid()), false)
	require.ErrorIs(t, err, ErrSelfExcluded)
}

func TestGetExecutablePath(t *testing.T) {
	exe := "/bin/bash"
	procRoot := createFakeProcFS(t, []FakeProcFSEntry{{pid: 1, cmdline: "", command: exe, exe: exe}})
	config := &AttacherConfig{
		ProcRoot: procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	path, err := ua.getExecutablePath(1)
	require.NoError(t, err, "failed to get executable path for existing PID")
	require.Equal(t, path, exe)

	path, err = ua.getExecutablePath(404)
	require.Error(t, err, "should fail to get executable path for non-existing PID")
	require.Empty(t, path, "should return empty path for non-existing PID")
}

const mapsFileSample = `
08048000-08049000 r-xp 00000000 03:00 8312       /opt/test
08049000-0804a000 rw-p 00001000 03:00 8312       /opt/test
0804a000-0806b000 rw-p 00000000 00:00 0          [heap]
a7cb1000-a7cb2000 ---p 00000000 00:00 0
a7cb2000-a7eb2000 rw-p 00000000 00:00 0
a7eb2000-a7eb3000 ---p 00000000 00:00 0
a7eb3000-a7ed5000 rw-p 00000000 00:00 0
a7ed5000-a8008000 r-xp 00000000 03:00 4222       /lib/libc.so.6
a8008000-a800a000 r--p 00133000 03:00 4222       /lib/libc.so.6
a800a000-a800b000 rw-p 00135000 03:00 4222       /lib/libc.so.6
a800b000-a800e000 rw-p 00000000 00:00 0
a800e000-a8022000 r-xp 00000000 03:00 14462      /lib/libpthread.so.0
a8022000-a8023000 r--p 00013000 03:00 14462      /lib/libpthread.so.0
a8023000-a8024000 rw-p 00014000 03:00 14462      /lib/libpthread.so.0
a8024000-a8027000 rw-p 00000000 00:00 0
a8027000-a8043000 r-xp 00000000 03:00 8317       /lib/ld-linux.so.2
a8043000-a8044000 r--p 0001b000 03:00 8317       /lib/ld-linux.so.2
a8044000-a8045000 rw-p 0001c000 03:00 8317       /lib/ld-linux.so.2
aff35000-aff4a000 rw-p 00000000 00:00 0          [stack]
ffffe000-fffff000 r-xp 00000000 00:00 0          [vdso]
01c00000-02000000 rw-p 00000000 00:0d 6123886    /anon_hugepage (deleted)
`

func TestGetLibrariesFromMapsFile(t *testing.T) {
	pid := 1
	procRoot := createFakeProcFS(t, []FakeProcFSEntry{{pid: uint32(pid), maps: mapsFileSample}})
	config := &AttacherConfig{
		ProcRoot: procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	libs, err := ua.getLibrariesFromMapsFile(pid)
	require.NoError(t, err, "failed to get libraries from maps file")
	require.NotEmpty(t, libs, "should return libraries from maps file")
	expectedLibs := []string{"/opt/test", "/lib/libc.so.6", "/lib/libpthread.so.0", "/lib/ld-linux.so.2"}
	require.ElementsMatch(t, expectedLibs, libs)
}

func TestComputeRequestedSymbols(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{}, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	selectorsOnlyAllOf := []manager.ProbesSelector{
		&manager.AllOf{
			Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}},
			},
		},
	}

	t.Run("OnlyMandatory", func(tt *testing.T) {
		rules := []*AttachRule{{ProbesSelector: selectorsOnlyAllOf}}
		requested, err := ua.computeSymbolsToRequest(rules)
		require.NoError(tt, err)
		require.ElementsMatch(tt, []SymbolRequest{{Name: "SSL_connect"}}, requested)
	})

	selectorsBestEffortAndMandatory := []manager.ProbesSelector{
		&manager.AllOf{
			Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}},
			},
		},
		&manager.BestEffort{
			Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__ThisFunctionDoesNotExistEver"}},
			},
		},
	}

	t.Run("MandatoryAndBestEffort", func(tt *testing.T) {
		rules := []*AttachRule{{ProbesSelector: selectorsBestEffortAndMandatory}}
		requested, err := ua.computeSymbolsToRequest(rules)
		require.NoError(tt, err)
		require.ElementsMatch(tt, []SymbolRequest{{Name: "SSL_connect"}, {Name: "ThisFunctionDoesNotExistEver", BestEffort: true}}, requested)
	})

	selectorsBestEffort := []manager.ProbesSelector{
		&manager.BestEffort{
			Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__ThisFunctionDoesNotExistEver"}},
			},
		},
	}

	t.Run("OnlyBestEffort", func(tt *testing.T) {
		rules := []*AttachRule{{ProbesSelector: selectorsBestEffort}}
		requested, err := ua.computeSymbolsToRequest(rules)
		require.NoError(tt, err)
		require.ElementsMatch(tt, []SymbolRequest{{Name: "SSL_connect", BestEffort: true}, {Name: "ThisFunctionDoesNotExistEver", BestEffort: true}}, requested)
	})

	selectorsWithReturnFunctions := []manager.ProbesSelector{
		&manager.AllOf{
			Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect__return"}},
			},
		},
	}

	t.Run("SelectorsWithReturnFunctions", func(tt *testing.T) {
		rules := []*AttachRule{{ProbesSelector: selectorsWithReturnFunctions}}
		requested, err := ua.computeSymbolsToRequest(rules)
		require.NoError(tt, err)
		require.ElementsMatch(tt, []SymbolRequest{{Name: "SSL_connect", IncludeReturnLocations: true}}, requested)
	})
}

func TestStartAndStopWithoutLibraryWatcher(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{}, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.Start()
	require.NoError(t, err)

	ua.Stop()
}

func TestStartAndStopWithLibraryWatcher(t *testing.T) {
	rules := []*AttachRule{{LibraryNameRegex: regexp.MustCompile(`libssl.so`)}}
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{Rules: rules}, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)
	require.True(t, ua.handlesLibraries())
	require.NotNil(t, ua.soWatcher)

	err = ua.Start()
	require.NoError(t, err)

	ua.Stop()
}

func TestRuleMatches(t *testing.T) {
	t.Run("Library", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.True(tt, rule.matchesLibrary("pkg/network/usm/testdata/libmmap/libssl.so.arm64"))
		require.False(tt, rule.matchesExecutable("pkg/network/usm/testdata/libmmap/libssl.so.arm64", nil))
	})

	t.Run("Executable", func(tt *testing.T) {
		rule := AttachRule{
			Targets: AttachToExecutable,
		}
		require.False(tt, rule.matchesLibrary("/bin/bash"))
		require.True(tt, rule.matchesExecutable("/bin/bash", nil))
	})

	t.Run("ExecutableWithFuncFilter", func(tt *testing.T) {
		rule := AttachRule{
			Targets: AttachToExecutable,
			ExecutableFilter: func(path string, _ *ProcInfo) bool {
				return strings.Contains(path, "bash")
			},
		}
		require.False(tt, rule.matchesLibrary("/bin/bash"))
		require.True(tt, rule.matchesExecutable("/bin/bash", nil))
		require.False(tt, rule.matchesExecutable("/bin/thing", nil))
	})
}

func TestMonitor(t *testing.T) {
	config := &AttacherConfig{
		Rules: []*AttachRule{{
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
			Targets:          AttachToExecutable | AttachToSharedLibraries,
		}},
		ProcessMonitorEventStream: false,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	lib := getLibSSLPath(t)

	ua.Start()
	t.Cleanup(ua.Stop)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)
	log.Errorf("rules:%+v", config.Rules[0])
	require.Eventually(t, func() bool {
		return len(mockRegistry.Calls) == 2 // Once for the library, another for the process itself
	}, 100*time.Millisecond, 10*time.Millisecond, "received calls %v", mockRegistry.Calls)

	mockRegistry.AssertCalled(t, "Register", lib, uint32(cmd.Process.Pid), mock.Anything, mock.Anything)
	mockRegistry.AssertCalled(t, "Register", cmd.Path, uint32(cmd.Process.Pid), mock.Anything, mock.Anything)
}

func TestInitialScan(t *testing.T) {
	selfPID, err := kernel.RootNSPID()
	require.NoError(t, err)
	procs := []FakeProcFSEntry{
		{pid: 1, cmdline: "/bin/bash", command: "/bin/bash", exe: "/bin/bash"},
		{pid: 2, cmdline: "/bin/bash", command: "/bin/bash", exe: "/bin/bash"},
		{pid: uint32(selfPID), cmdline: "datadog-agent/bin/system-probe", command: "sysprobe", exe: "sysprobe"},
	}
	procFS := createFakeProcFS(t, procs)

	config := &AttacherConfig{
		ProcRoot: procFS,
		Rules: []*AttachRule{{
			Targets:          AttachToExecutable | AttachToSharedLibraries,
			LibraryNameRegex: regexp.MustCompile(`.*`),
		}},
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry which two processes to expect
	mockRegistry.On("Register", "/bin/bash", uint32(1), mock.Anything, mock.Anything).Return(nil)
	mockRegistry.On("Register", "/bin/bash", uint32(2), mock.Anything, mock.Anything).Return(nil)

	err = ua.initialScan()
	require.NoError(t, err)

	mockRegistry.AssertExpectations(t)
}

func TestParseSymbolFromEBPFProbeName(t *testing.T) {
	t.Run("ValidName", func(tt *testing.T) {
		name := "uprobe__SSL_connect"
		symbol, manualReturn, err := parseSymbolFromEBPFProbeName(name)
		require.NoError(tt, err)
		require.False(tt, manualReturn)
		require.Equal(tt, "SSL_connect", symbol)
	})
	t.Run("ValidNameWithReturnMarker", func(tt *testing.T) {
		name := "uprobe__SSL_connect__return"
		symbol, manualReturn, err := parseSymbolFromEBPFProbeName(name)
		require.NoError(tt, err)
		require.True(tt, manualReturn)
		require.Equal(tt, "SSL_connect", symbol)
	})
	t.Run("InvalidNameWithUnrecognizedThirdPart", func(tt *testing.T) {
		name := "uprobe__SSL_connect__something"
		_, _, err := parseSymbolFromEBPFProbeName(name)
		require.Error(tt, err)
	})
	t.Run("InvalidNameNoSymbol", func(tt *testing.T) {
		name := "nothing"
		_, _, err := parseSymbolFromEBPFProbeName(name)
		require.Error(tt, err)
	})
}

func TestAttachToBinaryAndDetach(t *testing.T) {
	proc := FakeProcFSEntry{
		pid:     1,
		cmdline: "/bin/bash",
		exe:     "/bin/bash",
	}
	procFS := createFakeProcFS(t, []FakeProcFSEntry{proc})

	config := &AttacherConfig{
		ProcRoot: procFS,
		Rules: []*AttachRule{
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}},
				},
			},
		},
	}

	mockMan := &MockManager{}
	inspector := &MockBinaryInspector{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: proc.exe,
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)
	inspector.On("Cleanup", mock.Anything).Return(nil)

	// Tell the manager to return no probe when finding an existing one
	var nilProbe *manager.Probe // we can't just pass nil directly, if we do that the mock cannot convert it to *manager.Probe
	mockMan.On("GetProbe", mock.Anything).Return(nilProbe, false)

	// Tell the manager to accept the probe
	uid := "1hipf_0" // this is the UID that the manager will generate, from a path identifier with 0/0 as device/inode
	expectedProbe := &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect", UID: uid},
		BinaryPath:              target.HostPath,
		UprobeOffset:            symbolToAttach.EntryLocation,
		HookFuncName:            "SSL_connect",
	}
	mockMan.On("AddHook", mock.Anything, expectedProbe).Return(nil)

	err = ua.attachToBinary(target, config.Rules, NewProcInfo(procFS, proc.pid))
	require.NoError(t, err)
	mockMan.AssertExpectations(t)

	mockMan.On("DetachHook", expectedProbe.ProbeIdentificationPair).Return(nil)
	err = ua.detachFromBinary(target)
	require.NoError(t, err)
	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}

func TestAttachToBinaryAtReturnLocation(t *testing.T) {
	proc := FakeProcFSEntry{
		pid:     1,
		cmdline: "/bin/bash",
		exe:     "/bin/bash",
	}
	procFS := createFakeProcFS(t, []FakeProcFSEntry{proc})

	config := &AttacherConfig{
		ProcRoot: procFS,
		Rules: []*AttachRule{
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect__return"}},
				},
			},
		},
	}

	mockMan := &MockManager{}
	inspector := &MockBinaryInspector{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: proc.exe,
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234, ReturnLocations: []uint64{0x0, 0x1}}
	inspector.On("Inspect", target, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)

	// Tell the manager to return no probe when finding an existing one
	var nilProbe *manager.Probe // we can't just pass nil directly, if we do that the mock cannot convert it to *manager.Probe
	mockMan.On("GetProbe", mock.Anything).Return(nilProbe, false)

	// Tell the manager to accept the probe
	uidBase := "1hipf" // this is the UID that the manager will generate, from a path identifier with 0/0 as device/inode
	for n := 0; n < len(symbolToAttach.ReturnLocations); n++ {
		expectedProbe := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "uprobe__SSL_connect__return",
				UID:          fmt.Sprintf("%s_%d", uidBase, n)},
			BinaryPath:   target.HostPath,
			UprobeOffset: symbolToAttach.ReturnLocations[n],
			HookFuncName: "SSL_connect",
		}
		mockMan.On("AddHook", mock.Anything, expectedProbe).Return(nil)
	}

	err = ua.attachToBinary(target, config.Rules, NewProcInfo(procFS, proc.pid))
	require.NoError(t, err)
	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}

const mapsFileWithSSL = `
08048000-08049000 r-xp 00000000 03:00 8312       /usr/lib/libssl.so
`

func TestAttachToLibrariesOfPid(t *testing.T) {
	proc := FakeProcFSEntry{
		pid:     1,
		cmdline: "/bin/bash",
		exe:     "/bin/bash",
		maps:    mapsFileWithSSL,
	}
	procFS := createFakeProcFS(t, []FakeProcFSEntry{proc})

	config := &AttacherConfig{
		ProcRoot: procFS,
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libssl.so`),
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{
						ProbeIdentificationPair: manager.ProbeIdentificationPair{
							EBPFFuncName: "uprobe__SSL_connect",
						},
					},
				},
				Targets: AttachToSharedLibraries,
			},
			{
				LibraryNameRegex: regexp.MustCompile(`libtls.so`),
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{
						ProbeIdentificationPair: manager.ProbeIdentificationPair{
							EBPFFuncName: "uprobe__TLS_connect",
						},
					},
				},
				Targets: AttachToSharedLibraries,
			},
		},
	}

	mockMan := &MockManager{}
	inspector := &MockBinaryInspector{}
	registry := &MockFileRegistry{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)
	ua.fileRegistry = registry

	target := utils.FilePath{
		HostPath: "/usr/lib/libssl.so",
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)

	// Tell the manager to return no probe when finding an existing one
	var nilProbe *manager.Probe // we can't just pass nil directly, if we do that the mock cannot convert it to *manager.Probe
	mockMan.On("GetProbe", mock.Anything).Return(nilProbe, false)

	// Tell the manager to accept the probe
	uid := "1hipf_0" // this is the UID that the manager will generate, from a path identifier with 0/0 as device/inode
	expectedProbe := &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect", UID: uid},
		BinaryPath:              target.HostPath,
		UprobeOffset:            symbolToAttach.EntryLocation,
		HookFuncName:            "SSL_connect",
	}
	mockMan.On("AddHook", mock.Anything, expectedProbe).Return(nil)

	// Tell the registry to expect the process
	registry.On("Register", target.HostPath, uint32(proc.pid), mock.Anything, mock.Anything).Return(nil)

	// if this function calls the manager adding a probe with a different name than the one we requested, the test
	// will fail
	err = ua.attachToLibrariesOfPID(proc.pid)
	require.NoError(t, err)

	// We need to retrieve the calls from the registry and manually call the callback
	// to simulate the process being registered
	registry.AssertExpectations(t)
	cb := registry.Calls[0].Arguments[2].(func(utils.FilePath) error)
	require.NoError(t, cb(target))

	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}

type attachedProbe struct {
	probe *manager.Probe
	fpath *utils.FilePath
}

func TestUprobeAttacher(t *testing.T) {
	lib := getLibSSLPath(t)
	ebpfCfg := ddebpf.NewConfig()
	require.NotNil(t, ebpfCfg)

	buf, err := bytecode.GetReader(ebpfCfg.BPFDir, "uprobe_attacher-test.o")
	require.NoError(t, err)
	t.Cleanup(func() { buf.Close() })

	connectProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}
	mainProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__main"}

	mgr := manager.Manager{}

	attacherCfg := &AttacherConfig{
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libssl.so`),
				Targets:          AttachToSharedLibraries,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: connectProbeID},
				},
			},
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: mainProbeID},
				},
			},
		},
		ExcludeTargets: ExcludeInternal | ExcludeSelf,
		EbpfConfig:     ebpfCfg,
	}

	var attachedProbes []attachedProbe

	callback := func(probe *manager.Probe, fpath *utils.FilePath) {
		attachedProbes = append(attachedProbes, attachedProbe{probe: probe, fpath: fpath})
	}

	ua, err := NewUprobeAttacher("test", attacherCfg, &mgr, callback, &NativeBinaryInspector{})
	require.NoError(t, err)
	require.NotNil(t, ua)

	require.NoError(t, mgr.InitWithOptions(buf, manager.Options{}))
	require.NoError(t, mgr.Start())
	t.Cleanup(func() { mgr.Stop(manager.CleanAll) })
	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(attachedProbes) == 2
	}, 500*time.Millisecond, 50*time.Millisecond, "expected to attach 2 probes, got %d: %+v", len(attachedProbes), attachedProbes)

	// Check that the probes were attached
	var connectProbe, mainProbe *attachedProbe
	for _, ap := range attachedProbes {
		if ap.probe.EBPFFuncName == "uprobe__SSL_connect" {
			connectProbe = &ap
		} else if ap.probe.EBPFFuncName == "uprobe__main" {
			mainProbe = &ap
		}
	}

	require.NotNil(t, connectProbe)
	// Allow suffix, as sometimes the path reported is /proc/<pid>/root/<path>
	require.True(t, strings.HasSuffix(connectProbe.fpath.HostPath, lib), "expected to attach to %s, got %s", lib, connectProbe.fpath.HostPath)
	require.Equal(t, uint32(cmd.Process.Pid), connectProbe.fpath.PID)

	require.NotNil(t, mainProbe)
	require.Equal(t, uint32(cmd.Process.Pid), mainProbe.fpath.PID)
}
