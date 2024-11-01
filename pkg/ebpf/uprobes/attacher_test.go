// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	eventmonitortestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	procmontestutil "github.com/DataDog/datadog-agent/pkg/process/monitor/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// === Tests

func TestCanCreateAttacher(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", AttacherConfig{}, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)
}

func TestAttachPidExcludesInternal(t *testing.T) {
	exe := "datadog-agent/bin/system-probe"
	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: 1, Cmdline: exe, Command: exe, Exe: exe}})
	config := AttacherConfig{
		ExcludeTargets: ExcludeInternal,
		ProcRoot:       procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(1, false)
	require.ErrorIs(t, err, ErrInternalDDogProcessRejected)
}

func TestAttachPidExcludesSelf(t *testing.T) {
	config := AttacherConfig{
		ExcludeTargets: ExcludeSelf,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(uint32(os.Getpid()), false)
	require.ErrorIs(t, err, ErrSelfExcluded)
}

func TestGetExecutablePath(t *testing.T) {
	exe := "/bin/bash"
	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: 1, Cmdline: "", Command: exe, Exe: exe}})
	config := AttacherConfig{
		ProcRoot: procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, newMockProcessMonitor())
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
	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: uint32(pid), Maps: mapsFileSample}})
	config := AttacherConfig{
		ProcRoot: procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	libs, err := ua.getLibrariesFromMapsFile(pid)
	require.NoError(t, err, "failed to get libraries from maps file")
	require.NotEmpty(t, libs, "should return libraries from maps file")
	expectedLibs := []string{"/opt/test", "/lib/libc.so.6", "/lib/libpthread.so.0", "/lib/ld-linux.so.2"}
	require.ElementsMatch(t, expectedLibs, libs)
}

func TestComputeRequestedSymbols(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", AttacherConfig{}, &MockManager{}, nil, nil, newMockProcessMonitor())
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
	ua, err := NewUprobeAttacher("mock", AttacherConfig{}, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.Start()
	require.NoError(t, err)

	ua.Stop()
}

func TestStartAndStopWithLibraryWatcher(t *testing.T) {
	ebpfCfg := ddebpf.NewConfig()
	require.NotNil(t, ebpfCfg)
	if !sharedlibraries.IsSupported(ebpfCfg) {
		t.Skip("Kernel version does not support shared libraries")
		return
	}

	rules := []*AttachRule{{LibraryNameRegex: regexp.MustCompile(`libssl.so`), Targets: AttachToSharedLibraries}}
	ua, err := NewUprobeAttacher("mock", AttacherConfig{Rules: rules, EbpfConfig: ebpfCfg}, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)
	require.True(t, ua.handlesLibraries())

	err = ua.Start()
	require.NoError(t, err)
	require.NotNil(t, ua.soWatcher)

	ua.Stop()
}

func TestRuleMatches(t *testing.T) {
	t.Run("Library", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.True(tt, rule.matchesLibrary("pkg/network/usm/testdata/site-packages/dd-trace/libssl.so.arm64"))
		require.False(tt, rule.matchesExecutable("pkg/network/usm/testdata/site-packages/dd-trace/libssl.so.arm64", nil))
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
	ebpfCfg := ddebpf.NewConfig()
	require.NotNil(t, ebpfCfg)
	if !sharedlibraries.IsSupported(ebpfCfg) {
		t.Skip("Kernel version does not support shared libraries")
		return
	}

	procMon := launchProcessMonitor(t, false)

	config := AttacherConfig{
		Rules: []*AttachRule{{
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
			Targets:          AttachToExecutable | AttachToSharedLibraries,
		}},
		EbpfConfig: ebpfCfg,
	}
	ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, procMon)
	require.NoError(t, err)
	require.NotNil(t, ua)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Unregister", mock.Anything).Return(nil)
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	lib := getLibSSLPath(t)

	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return methodHasBeenCalledAtLeastTimes(mockRegistry, "Register", 2)
	}, 1500*time.Millisecond, 10*time.Millisecond, "received calls %v", mockRegistry.Calls)

	mockRegistry.AssertCalled(t, "Register", lib, uint32(cmd.Process.Pid), mock.Anything, mock.Anything, mock.Anything)
	mockRegistry.AssertCalled(t, "Register", cmd.Path, uint32(cmd.Process.Pid), mock.Anything, mock.Anything, mock.Anything)
}

func TestSync(t *testing.T) {
	selfPID, err := kernel.RootNSPID()
	require.NoError(t, err)
	rules := []*AttachRule{{
		Targets:          AttachToExecutable | AttachToSharedLibraries,
		LibraryNameRegex: regexp.MustCompile(`.*`),
		ExecutableFilter: func(path string, _ *ProcInfo) bool { return !strings.Contains(path, "donttrack") },
	}}

	t.Run("DetectsExistingProcesses", func(tt *testing.T) {
		procs := []FakeProcFSEntry{
			{Pid: 1, Cmdline: "/bin/bash", Command: "/bin/bash", Exe: "/bin/bash"},
			{Pid: 2, Cmdline: "/bin/bash", Command: "/bin/bash", Exe: "/bin/bash"},
			{Pid: 3, Cmdline: "/bin/donttrack", Command: "/bin/donttrack", Exe: "/bin/donttrack"},
			{Pid: uint32(selfPID), Cmdline: "datadog-agent/bin/system-probe", Command: "sysprobe", Exe: "sysprobe"},
		}
		procFS := CreateFakeProcFS(t, procs)

		config := AttacherConfig{
			ProcRoot:                       procFS,
			Rules:                          rules,
			EnablePeriodicScanNewProcesses: true,
		}

		ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, newMockProcessMonitor())
		require.NoError(tt, err)
		require.NotNil(tt, ua)

		mockRegistry := &MockFileRegistry{}
		ua.fileRegistry = mockRegistry

		// Tell mockRegistry which two processes to expect
		mockRegistry.On("Register", "/bin/bash", uint32(1), mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockRegistry.On("Register", "/bin/bash", uint32(2), mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err = ua.Sync(true, false)
		require.NoError(tt, err)

		mockRegistry.AssertExpectations(tt)
	})

	t.Run("RemovesDeletedProcesses", func(tt *testing.T) {
		procs := []FakeProcFSEntry{
			{Pid: 1, Cmdline: "/bin/bash", Command: "/bin/bash", Exe: "/bin/bash"},
			{Pid: 2, Cmdline: "/bin/bash", Command: "/bin/bash", Exe: "/bin/bash"},
			{Pid: 3, Cmdline: "/bin/donttrack", Command: "/bin/donttrack", Exe: "/bin/donttrack"},
			{Pid: uint32(selfPID), Cmdline: "datadog-agent/bin/system-probe", Command: "sysprobe", Exe: "sysprobe"},
		}
		procFS := CreateFakeProcFS(t, procs)

		config := AttacherConfig{
			ProcRoot:                       procFS,
			Rules:                          rules,
			EnablePeriodicScanNewProcesses: true,
		}

		ua, err := NewUprobeAttacher("mock", config, &MockManager{}, nil, nil, newMockProcessMonitor())
		require.NoError(tt, err)
		require.NotNil(tt, ua)

		mockRegistry := &MockFileRegistry{}
		ua.fileRegistry = mockRegistry

		// Tell mockRegistry which two processes to expect
		mockRegistry.On("Register", "/bin/bash", uint32(1), mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockRegistry.On("Register", "/bin/bash", uint32(2), mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockRegistry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{})

		err = ua.Sync(true, true)
		require.NoError(tt, err)
		mockRegistry.AssertExpectations(tt)

		// Now remove one process
		require.NoError(t, os.RemoveAll(filepath.Join(procFS, "2")))
		mockRegistry.ExpectedCalls = nil // Clear expected calls
		mockRegistry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{1: {}, 2: {}})
		mockRegistry.On("Unregister", uint32(2)).Return(nil)

		require.NoError(t, ua.Sync(true, true))
		mockRegistry.AssertExpectations(tt)
	})
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
		Pid:     1,
		Cmdline: "/bin/bash",
		Exe:     "/bin/bash",
	}
	procFS := CreateFakeProcFS(t, []FakeProcFSEntry{proc})

	config := AttacherConfig{
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
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: proc.Exe,
		PID:      proc.Pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, nil)
	inspector.On("Cleanup", mock.Anything).Return(nil)

	// Tell the manager to return no probe when finding an existing one
	var nilProbe *manager.Probe // we can't just pass nil directly, if we do that the mock cannot convert it to *manager.Probe
	mockMan.On("GetProbe", mock.Anything).Return(nilProbe, false)

	// Tell the manager to accept the probe
	uid := "1hipfd0" // this is the UID that the manager will generate, from a path identifier with 0/0 as device/inode
	expectedProbe := &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect", UID: uid},
		BinaryPath:              target.HostPath,
		UprobeOffset:            symbolToAttach.EntryLocation,
		HookFuncName:            "SSL_connect",
	}
	mockMan.On("AddHook", mock.Anything, expectedProbe).Return(nil)

	err = ua.attachToBinary(target, config.Rules, NewProcInfo(procFS, proc.Pid))
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
		Pid:     1,
		Cmdline: "/bin/bash",
		Exe:     "/bin/bash",
	}
	procFS := CreateFakeProcFS(t, []FakeProcFSEntry{proc})

	config := AttacherConfig{
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
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: proc.Exe,
		PID:      proc.Pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234, ReturnLocations: []uint64{0x0, 0x1}}
	inspector.On("Inspect", target, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, nil)

	// Tell the manager to return no probe when finding an existing one
	var nilProbe *manager.Probe // we can't just pass nil directly, if we do that the mock cannot convert it to *manager.Probe
	mockMan.On("GetProbe", mock.Anything).Return(nilProbe, false)

	// Tell the manager to accept the probe
	uidBase := "1hipf" // this is the UID that the manager will generate, from a path identifier with 0/0 as device/inode
	for n := 0; n < len(symbolToAttach.ReturnLocations); n++ {
		expectedProbe := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "uprobe__SSL_connect__return",
				UID:          fmt.Sprintf("%sr%d", uidBase, n)},
			BinaryPath:   target.HostPath,
			UprobeOffset: symbolToAttach.ReturnLocations[n],
			HookFuncName: "SSL_connect",
		}
		mockMan.On("AddHook", mock.Anything, expectedProbe).Return(nil)
	}

	err = ua.attachToBinary(target, config.Rules, NewProcInfo(procFS, proc.Pid))
	require.NoError(t, err)
	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}

const mapsFileWithSSL = `
08048000-08049000 r-xp 00000000 03:00 8312       /usr/lib/libssl.so
`

func TestAttachToLibrariesOfPid(t *testing.T) {
	proc := FakeProcFSEntry{
		Pid:     1,
		Cmdline: "/bin/bash",
		Exe:     "/bin/bash",
		Maps:    mapsFileWithSSL,
	}
	procFS := CreateFakeProcFS(t, []FakeProcFSEntry{proc})

	config := AttacherConfig{
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
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)
	ua.fileRegistry = registry

	target := utils.FilePath{
		HostPath: "/usr/lib/libssl.so",
		PID:      proc.Pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, nil)

	// Tell the manager to return no probe when finding an existing one
	var nilProbe *manager.Probe // we can't just pass nil directly, if we do that the mock cannot convert it to *manager.Probe
	mockMan.On("GetProbe", mock.Anything).Return(nilProbe, false)

	// Tell the manager to accept the probe
	uid := "1hipfd0" // this is the UID that the manager will generate, from a path identifier with 0/0 as device/inode
	expectedProbe := &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect", UID: uid},
		BinaryPath:              target.HostPath,
		UprobeOffset:            symbolToAttach.EntryLocation,
		HookFuncName:            "SSL_connect",
	}
	mockMan.On("AddHook", mock.Anything, expectedProbe).Return(nil)

	// Tell the registry to expect the process
	registry.On("Register", target.HostPath, uint32(proc.Pid), mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// if this function calls the manager adding a probe with a different name than the one we requested, the test
	// will fail
	err = ua.attachToLibrariesOfPID(proc.Pid)
	require.NoError(t, err)

	// We need to retrieve the calls from the registry and manually call the callback
	// to simulate the process being registered
	registry.AssertExpectations(t)
	cb := registry.Calls[0].Arguments[2].(utils.Callback)
	require.NoError(t, cb(target))

	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}

type attachedProbe struct {
	probe *manager.Probe
	fpath *utils.FilePath
}

func (ap *attachedProbe) String() string {
	return fmt.Sprintf("attachedProbe{probe: %s, PID: %d, path: %s}", ap.probe.EBPFFuncName, ap.fpath.PID, ap.fpath.HostPath)
}

func stringifyAttachedProbes(probes []attachedProbe) []string {
	var result []string
	for _, ap := range probes {
		result = append(result, ap.String())
	}
	return result
}

func TestUprobeAttacher(t *testing.T) {
	lib := getLibSSLPath(t)
	ebpfCfg := ddebpf.NewConfig()
	require.NotNil(t, ebpfCfg)

	if !sharedlibraries.IsSupported(ebpfCfg) {
		t.Skip("Kernel version does not support shared libraries")
		return
	}

	procMon := launchProcessMonitor(t, false)

	buf, err := bytecode.GetReader(ebpfCfg.BPFDir, "uprobe_attacher-test.o")
	require.NoError(t, err)
	t.Cleanup(func() { buf.Close() })

	connectProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}
	mainProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__main"}

	mgr := manager.Manager{}

	attacherCfg := AttacherConfig{
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
				ProbeOptionsOverride: map[string]ProbeOptions{
					mainProbeID.EBPFFuncName: {
						IsManualReturn: false,
						Symbol:         "main.main",
					},
				},
			},
		},
		ExcludeTargets:        ExcludeInternal | ExcludeSelf,
		EbpfConfig:            ebpfCfg,
		EnableDetailedLogging: true,
	}

	var attachedProbes []attachedProbe

	callback := func(probe *manager.Probe, fpath *utils.FilePath) {
		attachedProbes = append(attachedProbes, attachedProbe{probe: probe, fpath: fpath})
	}

	ua, err := NewUprobeAttacher("test", attacherCfg, &mgr, callback, &NativeBinaryInspector{}, procMon)
	require.NoError(t, err)
	require.NotNil(t, ua)

	require.NoError(t, mgr.InitWithOptions(buf, manager.Options{}))
	require.NoError(t, mgr.Start())
	t.Cleanup(func() { mgr.Stop(manager.CleanAll) })
	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	var connectProbe, mainProbe *attachedProbe
	require.Eventually(t, func() bool {
		// Find the probes we want to attach.
		// Note that we might attach to other processes, so filter by ours only
		for _, ap := range attachedProbes {
			if ap.probe.EBPFFuncName == "uprobe__SSL_connect" && ap.fpath.PID == uint32(cmd.Process.Pid) {
				connectProbe = &ap
			} else if ap.probe.EBPFFuncName == "uprobe__main" && ap.fpath.PID == uint32(cmd.Process.Pid) {
				mainProbe = &ap
			}
		}

		return connectProbe != nil && mainProbe != nil
	}, 5*time.Second, 50*time.Millisecond, "expected to attach 2 probes, got %d: %v (%v)", len(attachedProbes), attachedProbes, stringifyAttachedProbes(attachedProbes))

	require.NotNil(t, connectProbe)
	// Allow suffix, as sometimes the path reported is /proc/<pid>/root/<path>
	require.True(t, strings.HasSuffix(connectProbe.fpath.HostPath, lib), "expected to attach to %s, got %s", lib, connectProbe.fpath.HostPath)
	require.Equal(t, uint32(cmd.Process.Pid), connectProbe.fpath.PID)

	require.NotNil(t, mainProbe)
	require.Equal(t, uint32(cmd.Process.Pid), mainProbe.fpath.PID)
}

func launchProcessMonitor(t *testing.T, useEventStream bool) *monitor.ProcessMonitor {
	pm := monitor.GetProcessMonitor()
	t.Cleanup(pm.Stop)
	require.NoError(t, pm.Initialize(useEventStream))
	if useEventStream {
		eventmonitortestutil.StartEventMonitor(t, procmontestutil.RegisterProcessMonitorEventConsumer)
	}

	return pm
}

func createTempTestFile(t *testing.T, name string) (string, utils.PathIdentifier) {
	fullPath := filepath.Join(t.TempDir(), name)

	f, err := os.Create(fullPath)
	f.WriteString("foobar")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() {
		os.RemoveAll(fullPath)
	})

	pathID, err := utils.NewPathIdentifier(fullPath)
	require.NoError(t, err)

	return fullPath, pathID
}

type SharedLibrarySuite struct {
	suite.Suite
	procMonitor ProcessMonitor
}

func TestAttacherSharedLibrary(t *testing.T) {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() {
		modes = append(modes, ebpftest.Prebuilt)
	}
	ebpftest.TestBuildModes(t, modes, "", func(tt *testing.T) {
		if !sharedlibraries.IsSupported(ddebpf.NewConfig()) {
			tt.Skip("shared library tracing not supported for this platform")
		}

		tt.Run("netlink", func(ttt *testing.T) {
			processMonitor := launchProcessMonitor(ttt, false)
			suite.Run(ttt, &SharedLibrarySuite{procMonitor: processMonitor})
		})

		tt.Run("event stream", func(ttt *testing.T) {
			processMonitor := launchProcessMonitor(ttt, true)
			suite.Run(ttt, &SharedLibrarySuite{procMonitor: processMonitor})
		})
	})
}

func (s *SharedLibrarySuite) TestSingleFile() {
	t := s.T()
	ebpfCfg := ddebpf.NewConfig()

	fooPath1, _ := createTempTestFile(t, "foo-libssl.so")

	attachCfg := AttacherConfig{
		Rules: []*AttachRule{{
			LibraryNameRegex: regexp.MustCompile(`foo-libssl.so`),
			Targets:          AttachToSharedLibraries,
		}},
		EbpfConfig:                     ebpfCfg,
		EnablePeriodicScanNewProcesses: false,
		PerformInitialScan:             false,
	}

	ua, err := NewUprobeAttacher("test", attachCfg, &MockManager{}, nil, nil, s.procMonitor)
	require.NoError(t, err)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Unregister", mock.Anything).Return(nil)
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	// We can have missed events so we need to retry
	var cmd *exec.Cmd
	waitAndRetryIfFail(t,
		func() {
			cmd, err = fileopener.OpenFromAnotherProcess(t, fooPath1)
			require.NoError(t, err)
		},
		func() bool {
			return methodHasBeenCalledTimes(mockRegistry, "Register", 1)
		},
		func(testSuccess bool) {
			// Only kill the process if the test failed, if it succeeded we want to kill it later
			// to check if the Unregister call was done correctly
			if !testSuccess && cmd != nil && cmd.Process != nil {
				cmd.Process.Kill()
			}
		},
		3, 10*time.Millisecond, 500*time.Millisecond, "did not catch process running, received calls %v", mockRegistry.Calls)

	mockRegistry.AssertCalled(t, "Register", fooPath1, uint32(cmd.Process.Pid), mock.Anything, mock.Anything, mock.Anything)

	mockRegistry.Calls = nil
	require.NoError(t, cmd.Process.Kill())

	require.Eventually(t, func() bool {
		// Other processes might have finished and forced the Unregister call to the registry
		return methodHasBeenCalledWithPredicate(mockRegistry, "Unregister", func(call mock.Call) bool {
			return call.Arguments[0].(uint32) == uint32(cmd.Process.Pid)
		})
	}, time.Second*10, 200*time.Millisecond, "received calls %v", mockRegistry.Calls)

	mockRegistry.AssertCalled(t, "Unregister", uint32(cmd.Process.Pid))
}

func (s *SharedLibrarySuite) TestDetectionWithPIDAndRootNamespace() {
	t := s.T()
	ebpfCfg := ddebpf.NewConfig()

	_, err := os.Stat("/usr/bin/busybox")
	if err != nil {
		t.Skip("skip for the moment as some distro are not friendly with busybox package")
	}

	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "root")
	err = os.MkdirAll(root, 0755)
	require.NoError(t, err)

	libpath := "/fooroot-crypto.so"

	err = exec.Command("cp", "/usr/bin/busybox", root+"/ash").Run()
	require.NoError(t, err)
	err = exec.Command("cp", "/usr/bin/busybox", root+"/sleep").Run()
	require.NoError(t, err)

	attachCfg := AttacherConfig{
		Rules: []*AttachRule{{
			LibraryNameRegex: regexp.MustCompile(`fooroot-crypto.so`),
			Targets:          AttachToSharedLibraries,
		}},
		EbpfConfig: ebpfCfg,
	}

	ua, err := NewUprobeAttacher("test", attachCfg, &MockManager{}, nil, nil, s.procMonitor)
	require.NoError(t, err)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Unregister", mock.Anything).Return(nil)
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	time.Sleep(10 * time.Millisecond)
	// simulate a slow (1 second) : open, write, close of the file
	// in a new pid and mount namespaces
	o, err := exec.Command("unshare", "--fork", "--pid", "-R", root, "/ash", "-c", fmt.Sprintf("sleep 1 > %s", libpath)).CombinedOutput()
	if err != nil {
		t.Log(err, string(o))
	}
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	require.Eventually(t, func() bool {
		return methodHasBeenCalledTimes(mockRegistry, "Register", 1)
	}, time.Second*10, 100*time.Millisecond, "received calls %v", mockRegistry.Calls)

	// assert that soWatcher detected foo-crypto.so being opened and triggered the callback
	foundCall := false
	for _, call := range mockRegistry.Calls {
		if call.Method == "Register" {
			args := call.Arguments
			require.True(t, strings.HasSuffix(args[0].(string), libpath))
			foundCall = true
		}
	}
	require.True(t, foundCall)

	// must fail on the host
	_, err = os.Stat(libpath)
	require.Error(t, err)
}

func methodHasBeenCalledTimes(registry *MockFileRegistry, methodName string, times int) bool {
	calls := 0
	for _, call := range registry.Calls {
		if call.Method == methodName {
			calls++
		}
	}
	return calls == times
}

func methodHasBeenCalledAtLeastTimes(registry *MockFileRegistry, methodName string, times int) bool {
	calls := 0
	for _, call := range registry.Calls {
		if call.Method == methodName {
			calls++
		}
	}
	return calls >= times
}

func methodHasBeenCalledWithPredicate(registry *MockFileRegistry, methodName string, predicate func(mock.Call) bool) bool {
	for _, call := range registry.Calls {
		if call.Method == methodName && predicate(call) {
			return true
		}
	}
	return false
}
