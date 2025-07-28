// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"errors"
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
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// === Tests

const (
	testModuleName   = "mock-module"
	testAttacherName = "mock"
)

func TestCanCreateAttacher(t *testing.T) {
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{}, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)
}

func TestInternalProcessesRegex(t *testing.T) {
	require.True(t, internalProcessRegex.MatchString("datadog-agent/bin/system-probe"))
	require.True(t, internalProcessRegex.MatchString("datadog-agent/bin/trace-agent"))
	require.True(t, internalProcessRegex.MatchString("datadog-agent/bin/process-agent"))
	require.True(t, internalProcessRegex.MatchString("datadog-agent/bin/security-agent"))
	require.True(t, internalProcessRegex.MatchString("datadog-agent/bin/otel-agent"))
}

func TestAttachPidExcludesInternal(t *testing.T) {
	exe := "datadog-agent/bin/system-probe"
	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: 1, Cmdline: exe, Command: exe, Exe: exe}})
	config := AttacherConfig{
		ExcludeTargets: ExcludeInternal,
		ProcRoot:       procRoot,
	}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(1, false)
	require.ErrorIs(t, err, ErrInternalDDogProcessRejected)
}

func TestAttachPidExcludesContainerdTmp(t *testing.T) {
	tmpdir := t.TempDir()

	// Create a tmpdir/tmpmounts/containerd-mount/bar directory with a file in
	// it to simulate a containerd tmp mount. It needs to exist so that the code
	// will be able to read that file
	exe := filepath.Join(tmpdir, "tmpmounts/containerd-mount/bar")
	require.NoError(t, os.MkdirAll(filepath.Dir(exe), 0755))
	require.NoError(t, os.WriteFile(exe, []byte{}, 0644))

	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: 1, Cmdline: exe, Command: exe, Exe: exe}})
	config := AttacherConfig{
		ExcludeTargets:        ExcludeContainerdTmp,
		ProcRoot:              procRoot,
		EnableDetailedLogging: true,
		Rules: []*AttachRule{
			{Targets: AttachToExecutable},
		},
	}

	// Cleanup should be called anyways, even if the attach fails
	inspector := &MockBinaryInspector{}
	inspector.On("Cleanup", mock.Anything).Return(nil)

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, inspector, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(1, false)
	require.ErrorIs(t, err, utils.ErrEnvironment)

	inspector.AssertExpectations(t)
}

func TestAttachPidReadsSharedLibraries(t *testing.T) {
	exe := "foobar"
	pid := uint32(1)
	libname := "/target/libssl.so"
	maps := fmt.Sprintf("08048000-08049000 r-xp 00000000 03:00 8312       %s", libname)
	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: pid, Cmdline: exe, Command: exe, Exe: exe, Maps: maps}})
	config := AttacherConfig{
		ProcRoot: procRoot,
		Rules: []*AttachRule{
			{LibraryNameRegex: regexp.MustCompile(`libssl\.so`), Targets: AttachToSharedLibraries},
			{Targets: AttachToExecutable},
		},
		SharedLibsLibsets:     []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		EnableDetailedLogging: true,
	}

	registry := &MockFileRegistry{}
	// Force a failure on the Register call for the executable, to simulate a
	// binary that doesn't have our desired functions to attach
	registry.On("Register", exe, pid, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("cannot attach"))

	// Expect a call to Register for the library
	registry.On("Register", libname, pid, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)
	require.True(t, ua.handlesExecutables())
	require.True(t, ua.handlesLibraries())
	ua.fileRegistry = registry

	err = ua.AttachPIDWithOptions(pid, true)
	require.Error(t, err)

	// We should get calls to Register both with the executable and the library
	// name, even though the executable returns an error
	registry.AssertExpectations(t)
}

func TestAttachPidExcludesSelf(t *testing.T) {
	config := AttacherConfig{
		ExcludeTargets: ExcludeSelf,
	}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPIDWithOptions(uint32(os.Getpid()), false)
	require.ErrorIs(t, err, ErrSelfExcluded)
}

func TestAttachToBinaryContainerdTmpReturnsErrEnvironment(t *testing.T) {
	config := AttacherConfig{
		ExcludeTargets: ExcludeContainerdTmp,
	}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.attachToBinary(utils.FilePath{PID: uint32(os.Getpid()), HostPath: "/foo/tmpmounts/containerd-mount/bar"}, nil, nil)
	require.ErrorIs(t, err, utils.ErrEnvironment)
}

func TestGetExecutablePath(t *testing.T) {
	exe := "/bin/bash"
	procRoot := CreateFakeProcFS(t, []FakeProcFSEntry{{Pid: 1, Cmdline: "", Command: exe, Exe: exe}})
	config := AttacherConfig{
		ProcRoot: procRoot,
	}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
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
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	libs, err := ua.getLibrariesFromMapsFile(pid)
	require.NoError(t, err, "failed to get libraries from maps file")
	require.NotEmpty(t, libs, "should return libraries from maps file")
	expectedLibs := []string{"/opt/test", "/lib/libc.so.6", "/lib/libpthread.so.0", "/lib/ld-linux.so.2"}
	require.ElementsMatch(t, expectedLibs, libs)
}

func TestComputeRequestedSymbols(t *testing.T) {
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{}, &MockManager{}, nil, nil, newMockProcessMonitor())
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
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{}, &MockManager{}, nil, nil, newMockProcessMonitor())
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
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, AttacherConfig{Rules: rules, EbpfConfig: ebpfCfg, SharedLibsLibsets: []sharedlibraries.Libset{sharedlibraries.LibsetCrypto}}, &MockManager{}, nil, nil, newMockProcessMonitor())
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

func TestAttachRuleValidatesLibsets(t *testing.T) {
	attachCfg := AttacherConfig{SharedLibsLibsets: []sharedlibraries.Libset{sharedlibraries.LibsetCrypto}}

	t.Run("ValidLibset", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.NoError(tt, rule.Validate(&attachCfg))
	})

	t.Run("IncompatibleLibset", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: regexp.MustCompile(`somethingelse.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.Error(tt, rule.Validate(&attachCfg))
	})

	t.Run("NilLibraryNameRegex", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: nil,
			Targets:          AttachToSharedLibraries,
		}
		require.Error(tt, rule.Validate(&attachCfg))
	})

}

func TestAttachRuleValidatesMultipleLibsets(t *testing.T) {
	attachCfgWithMultipleLibsets := AttacherConfig{SharedLibsLibsets: []sharedlibraries.Libset{sharedlibraries.LibsetCrypto, sharedlibraries.LibsetGPU}}

	t.Run("ValidRules", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.NoError(tt, rule.Validate(&attachCfgWithMultipleLibsets))
		rule = AttachRule{
			LibraryNameRegex: regexp.MustCompile(`libcudart.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.NoError(tt, rule.Validate(&attachCfgWithMultipleLibsets))
	})

	t.Run("InvalidRule", func(tt *testing.T) {
		rule := AttachRule{
			LibraryNameRegex: regexp.MustCompile(`somethingelse.so`),
			Targets:          AttachToSharedLibraries,
		}
		require.Error(tt, rule.Validate(&attachCfgWithMultipleLibsets))
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
		EbpfConfig:        ebpfCfg,
		SharedLibsLibsets: []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
	}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, procMon)
	require.NoError(t, err)
	require.NotNil(t, ua)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Log").Return()
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

	mockRegistry.AssertCalled(t, "Register", cmd.Path, uint32(cmd.Process.Pid), mock.Anything, mock.Anything, mock.Anything)
	mockRegistry.AssertCalled(t, "Register", lib, uint32(cmd.Process.Pid), mock.Anything, mock.Anything, mock.Anything)

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
			SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		}

		ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
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
			SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		}

		ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, &MockManager{}, nil, nil, newMockProcessMonitor())
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
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, mockMan, nil, inspector, newMockProcessMonitor())
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

	// FileRegistry calls the detach callback without host path. Replicate that here.
	detachPath := utils.FilePath{
		ID: target.ID,
	}

	mockMan.On("DetachHook", expectedProbe.ProbeIdentificationPair).Return(nil)
	err = ua.detachFromBinary(detachPath)
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
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, mockMan, nil, inspector, newMockProcessMonitor())
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
				LibraryNameRegex: regexp.MustCompile(`libgnutls.so`),
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
		SharedLibsLibsets: []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
	}

	mockMan := &MockManager{}
	inspector := &MockBinaryInspector{}
	registry := &MockFileRegistry{}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, mockMan, nil, inspector, newMockProcessMonitor())
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

func testUprobeAttacherInner(t *testing.T, attacherFunc func() AttacherRunner, targetFunc func() AttacherTargetRunner) {
	if !sharedlibraries.IsSupported(ddebpf.NewConfig()) {
		t.Skip("skip as shared libraries are not supported for this platform")
	}

	attacher := attacherFunc()
	target := targetFunc()

	libPath := getLibSSLPath(t)
	config := RunTestAttacherConfig{
		WaitTimeForAttach: 3 * time.Second,
		WaitTimeForDetach: 3 * time.Second,
		PathsToOpen:       []string{libPath},
		ExpectedProbes: []ProbeRequest{
			{
				ProbeName: "uprobe__SSL_connect",
				Path:      libPath,
			},
			{
				ProbeName: "uprobe__main",
			},
		},
	}

	RunTestAttacher(t, LibraryAndMainAttacherTestConfigName, attacher, target, config)
}

func TestUprobeAttacher(t *testing.T) {
	t.Run("BareAttacher", func(t *testing.T) {
		t.Run("BareProcess", func(t *testing.T) {
			testUprobeAttacherInner(t, NewSameProcessAttacherRunner, NewFmapperRunner)
		})

		t.Run("ContainerizedProcess", func(t *testing.T) {
			testUprobeAttacherInner(t, NewSameProcessAttacherRunner, NewContainerizedFmapperRunner)
		})
	})

	t.Run("ContainerizedAttacher", func(t *testing.T) {
		t.Run("BareProcess", func(t *testing.T) {
			testUprobeAttacherInner(t, NewContainerizedAttacherRunner, NewFmapperRunner)
		})

		t.Run("ContainerizedProcess", func(t *testing.T) {
			testUprobeAttacherInner(t, NewContainerizedAttacherRunner, NewContainerizedFmapperRunner)
		})
	})
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
	procMonitor *processMonitorProxy
}

func TestAttacherSharedLibrary(t *testing.T) {
	ebpftest.TestBuildModes(t, ebpftest.SupportedBuildModes(), "", func(tt *testing.T) {
		if !sharedlibraries.IsSupported(ddebpf.NewConfig()) {
			tt.Skip("shared library tracing not supported for this platform")
		}

		tt.Run("netlink", func(ttt *testing.T) {
			processMonitor := launchProcessMonitor(ttt, false)

			// Use a proxy so we can manually trigger events in case of misses
			procmonObserver := newProcessMonitorProxy(processMonitor)
			suite.Run(ttt, &SharedLibrarySuite{procMonitor: procmonObserver})
		})

		tt.Run("event stream", func(ttt *testing.T) {
			processMonitor := launchProcessMonitor(ttt, true)

			// Use a proxy so we can manually trigger events in case of misses
			procmonObserver := newProcessMonitorProxy(processMonitor)
			suite.Run(ttt, &SharedLibrarySuite{procMonitor: procmonObserver})
		})
	})
}

func (s *SharedLibrarySuite) SetupTest() {
	// Reset callbacks
	s.procMonitor.Reset()
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
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		EnablePeriodicScanNewProcesses: false,
	}

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, attachCfg, &MockManager{}, nil, nil, s.procMonitor)
	require.NoError(t, err)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Log").Return()
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

	// The ideal path would be that the process monitor sends an exit event for
	// the process as it's killed. However, sometimes these events are missed
	// and the callbacks aren't called. Unlike the "Process launch" event, we
	// cannot recreate the process exit, which would be the ideal solution to
	// ensure we're testing the correct behavior (including any
	// filters/callbacks on the process monitor). Instead, we manually trigger
	// the exit event for the process using the processMonitorProxy, which
	// should replicate the same codepath.
	waitAndRetryIfFail(t,
		func() {
			require.NoError(t, cmd.Process.Kill())
		},
		func() bool {
			return methodHasBeenCalledWithPredicate(mockRegistry, "Unregister", func(call mock.Call) bool {
				return call.Arguments[0].(uint32) == uint32(cmd.Process.Pid)
			})
		},
		func(testSuccess bool) {
			if !testSuccess {
				// If the test failed once, manually trigger the exit event
				s.procMonitor.triggerExit(uint32(cmd.Process.Pid))
			}
		}, 2, 10*time.Millisecond, 500*time.Millisecond, "attacher did not correctly handle exit events received calls %v", mockRegistry.Calls)

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
		EbpfConfig:        ebpfCfg,
		SharedLibsLibsets: []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
	}

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, attachCfg, &MockManager{}, nil, nil, s.procMonitor)
	require.NoError(t, err)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Log").Return()
	mockRegistry.On("Unregister", mock.Anything).Return(nil)
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	time.Sleep(10 * time.Millisecond)
	// simulate a slow (1 second) : open, read, close of the file
	// in a new pid and mount namespaces
	o, err := exec.Command("unshare", "--fork", "--pid", "-R", root, "/ash", "-c",
		fmt.Sprintf("touch foo && mv foo %s && sleep 1 < %s", libpath, libpath)).CombinedOutput()
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

func (s *SharedLibrarySuite) TestMultipleLibsets() {
	t := s.T()
	ebpfCfg := ddebpf.NewConfig()

	// Create test files for different libsets
	cryptoLibPath, _ := createTempTestFile(t, "foo-libssl.so")
	gpuLibPath, _ := createTempTestFile(t, "foo-libcudart.so")
	libcLibPath, _ := createTempTestFile(t, "foo-libc.so")

	attachCfg := AttacherConfig{
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`foo-libssl\.so`),
				Targets:          AttachToSharedLibraries,
			},
			{
				LibraryNameRegex: regexp.MustCompile(`foo-libcudart\.so`),
				Targets:          AttachToSharedLibraries,
			},
			{
				LibraryNameRegex: regexp.MustCompile(`foo-libc\.so`),
				Targets:          AttachToSharedLibraries,
			},
		},
		EbpfConfig:                     ebpfCfg,
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto, sharedlibraries.LibsetGPU, sharedlibraries.LibsetLibc},
		EnablePeriodicScanNewProcesses: false,
	}

	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, attachCfg, &MockManager{}, nil, nil, s.procMonitor)
	require.NoError(t, err)

	mockRegistry := &MockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Log").Return()
	mockRegistry.On("Unregister", mock.Anything).Return(nil)
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, ua.Start())
	t.Cleanup(ua.Stop)

	// Test that all three libsets can be detected and registered
	type testCase struct {
		libPath     string
		libName     string
		description string
	}

	testCases := []testCase{
		{cryptoLibPath, "foo-libssl.so", "crypto library"},
		{gpuLibPath, "foo-libcudart.so", "GPU library"},
		{libcLibPath, "foo-libc.so", "libc library"},
	}

	var commands []*exec.Cmd

	for _, tc := range testCases {
		var cmd *exec.Cmd
		waitAndRetryIfFail(t,
			func() {
				cmd, err = fileopener.OpenFromAnotherProcess(t, tc.libPath)
				require.NoError(t, err)
			},
			func() bool {
				return methodHasBeenCalledWithPredicate(mockRegistry, "Register", func(call mock.Call) bool {
					return strings.Contains(call.Arguments[0].(string), tc.libName)
				})
			},
			func(testSuccess bool) {
				if !testSuccess && cmd != nil && cmd.Process != nil {
					cmd.Process.Kill()
				}
			},
			3, 10*time.Millisecond, 500*time.Millisecond, "did not catch %s process, received calls %v", tc.description, mockRegistry.Calls)

		require.NotNil(t, cmd)
		require.NotNil(t, cmd.Process)
		commands = append(commands, cmd)
	}

	for i, cmd := range commands {
		mockRegistry.AssertCalled(t, "Register", testCases[i].libPath, uint32(cmd.Process.Pid), mock.Anything, mock.Anything, mock.Anything)
		cmd.Process.Kill()
	}

	// Verify unregister calls for all processes
	require.Eventually(t, func() bool {
		return methodHasBeenCalledAtLeastTimes(mockRegistry, "Unregister", len(commands))
	}, 1500*time.Millisecond, 10*time.Millisecond, "did not receive unregister calls for all processes, received calls %v", mockRegistry.Calls)
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

func TestSyncRetryAndReattach(t *testing.T) {
	proc := FakeProcFSEntry{
		Pid:     1,
		Cmdline: "/bin/bash",
		Command: "/bin/bash",
		Exe:     "/bin/bash",
	}
	procFS := CreateFakeProcFS(t, []FakeProcFSEntry{proc})
	emptyProcFS := CreateFakeProcFS(t, []FakeProcFSEntry{})

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
		EnablePeriodicScanNewProcesses: true,
		MaxPeriodicScansPerProcess:     2,
	}

	registry := &MockFileRegistry{}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, nil, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	ua.fileRegistry = registry

	// First attempt should fail, registry should report no processes
	registry.On("Register", proc.Exe, proc.Pid, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("inspection failed")).Once()
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	err = ua.Sync(true, true)
	require.NoError(t, err) // Sync itself doesn't return errors from individual attachments
	require.Equal(t, 1, ua.scansPerPid[proc.Pid])
	registry.AssertExpectations(t)

	// Second attempt should succeed, registry should still report no processes
	registry.On("Register", proc.Exe, proc.Pid, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	err = ua.Sync(true, true)
	require.NoError(t, err)
	require.Equal(t, 2, ua.scansPerPid[proc.Pid])
	registry.AssertExpectations(t)

	// Scan an empty procFS to simulate a process exit, the registry in this case does know about the process
	// to simulate it has been attached correctly
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{proc.Pid: {}}).Once()
	registry.On("Unregister", proc.Pid).Return(nil).Once()
	ua.config.ProcRoot = emptyProcFS
	err = ua.Sync(true, true)
	require.NoError(t, err)
	require.Empty(t, ua.scansPerPid)
	registry.AssertExpectations(t)

	// Should be able to re-attach to same PID, registry doesn't know about the process as it was detached before
	ua.config.ProcRoot = procFS
	registry.On("Register", proc.Exe, proc.Pid, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	err = ua.Sync(true, true)
	require.NoError(t, err)
	require.Equal(t, config.MaxPeriodicScansPerProcess, ua.scansPerPid[proc.Pid]) // attached correctly, so marked as already scanned to the max
	registry.AssertExpectations(t)
}

func TestSyncNoAttach(t *testing.T) {
	proc := FakeProcFSEntry{
		Pid:     1,
		Cmdline: "/bin/bash",
		Command: "/bin/bash",
		Exe:     "/bin/bash",
	}
	procFS := CreateFakeProcFS(t, []FakeProcFSEntry{proc})
	emptyProcFS := CreateFakeProcFS(t, []FakeProcFSEntry{})

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
		EnablePeriodicScanNewProcesses: true,
		MaxPeriodicScansPerProcess:     2,
	}

	registry := &MockFileRegistry{}
	ua, err := NewUprobeAttacher(testModuleName, testAttacherName, config, nil, nil, nil, newMockProcessMonitor())
	require.NoError(t, err)
	require.NotNil(t, ua)

	ua.fileRegistry = registry

	// All attempts should fail
	registry.On("Register", proc.Exe, proc.Pid, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("inspection failed")).Once()
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	err = ua.Sync(true, true)
	require.NoError(t, err) // Sync itself doesn't return errors from individual attachments
	require.Equal(t, 1, ua.scansPerPid[proc.Pid])
	registry.AssertExpectations(t)

	// Second attempt should still fail
	registry.On("Register", proc.Exe, proc.Pid, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("inspection failed")).Once()
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	err = ua.Sync(true, true)
	require.NoError(t, err)
	require.Equal(t, 2, ua.scansPerPid[proc.Pid])
	registry.AssertExpectations(t)

	// Third attempt should not even get to the registry, so we don't expect any calls to the Register method
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	err = ua.Sync(true, true)
	require.NoError(t, err)
	require.Equal(t, 2, ua.scansPerPid[proc.Pid])
	registry.AssertExpectations(t)

	// Scan an empty procFS to simulate the process exiting, it should disapper from the scansPerPid map
	registry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{}).Once()
	ua.config.ProcRoot = emptyProcFS
	err = ua.Sync(true, true)
	require.NoError(t, err)
	require.Empty(t, ua.scansPerPid)
	registry.AssertExpectations(t)
}
