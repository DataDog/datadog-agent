// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// === Mocks
type mockManager struct {
	mock.Mock
}

func (m *mockManager) AddHook(name string, probe *manager.Probe) error {
	args := m.Called(name, probe)
	return args.Error(0)
}

func (m *mockManager) DetachHook(probeID manager.ProbeIdentificationPair) error {
	args := m.Called(probeID)
	return args.Error(0)
}

func (m *mockManager) GetProbe(probeID manager.ProbeIdentificationPair) (*manager.Probe, bool) {
	args := m.Called(probeID)
	return args.Get(0).(*manager.Probe), args.Bool(1)
}

type mockFileRegistry struct {
	mock.Mock
}

func (m *mockFileRegistry) Register(namespacedPath string, pid uint32, activationCB, deactivationCB func(utils.FilePath) error) error {
	args := m.Called(namespacedPath, pid, activationCB, deactivationCB)
	return args.Error(0)
}

func (m *mockFileRegistry) Unregister(pid uint32) error {
	args := m.Called(pid)
	return args.Error(0)
}

func (m *mockFileRegistry) Clear() {
	m.Called()
}

func (m *mockFileRegistry) GetRegisteredProcesses() map[uint32]struct{} {
	args := m.Called()
	return args.Get(0).(map[uint32]struct{})
}

type mockBinaryInspector struct {
	mock.Mock
}

func (m *mockBinaryInspector) Inspect(path string, requests []SymbolRequest) (map[string]bininspect.FunctionMetadata, bool, error) {
	args := m.Called(path, requests)
	return args.Get(0).(map[string]bininspect.FunctionMetadata), args.Bool(1), args.Error(2)
}

// === Test utils
type FakeProcFSEntry struct {
	pid     uint32
	cmdline string
	command string
	exe     string
	maps    string
}

func createFakeProcFS(t *testing.T, entries []FakeProcFSEntry) string {
	procRoot := t.TempDir()

	for _, entry := range entries {
		baseDir := filepath.Join(procRoot, strconv.Itoa(int(entry.pid)))

		createFile(t, filepath.Join(baseDir, "cmdline"), entry.cmdline)
		createFile(t, filepath.Join(baseDir, "comm"), entry.command)
		createFile(t, filepath.Join(baseDir, "maps"), entry.maps)

		if entry.exe != "" {
			createSymlink(t, entry.exe, filepath.Join(baseDir, "exe"))
		}
	}

	return procRoot
}

func createFile(t *testing.T, path, data string) {
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.WriteFile(path, []byte(data), 0775))
}

func createSymlink(t *testing.T, target, link string) {
	dir := filepath.Dir(link)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.Symlink(target, link))
}

// === Tests

func TestCanCreateAttacher(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{}, &mockManager{}, nil, nil)
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
	ua, err := NewUprobeAttacher("mock", config, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPID(1, false)
	require.ErrorIs(t, err, ErrInternalDDogProcessRejected)
}

func TestAttachPidExcludesSelf(t *testing.T) {
	config := &AttacherConfig{
		ExcludeTargets: ExcludeSelf,
	}
	ua, err := NewUprobeAttacher("mock", config, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.AttachPID(uint32(os.Getpid()), false)
	require.ErrorIs(t, err, ErrSelfExcluded)
}

func TestGetExecutablePath(t *testing.T) {
	exe := "/bin/bash"
	procRoot := createFakeProcFS(t, []FakeProcFSEntry{{pid: 1, cmdline: "", command: exe, exe: exe}})
	config := &AttacherConfig{
		ProcRoot: procRoot,
	}
	ua, err := NewUprobeAttacher("mock", config, &mockManager{}, nil, nil)
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
	ua, err := NewUprobeAttacher("mock", config, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	libs, err := ua.getLibrariesFromMapsFile(pid)
	require.NoError(t, err, "failed to get libraries from maps file")
	require.NotEmpty(t, libs, "should return libraries from maps file")
	expectedLibs := []string{"/opt/test", "/lib/libc.so.6", "/lib/libpthread.so.0", "/lib/ld-linux.so.2"}
	require.ElementsMatch(t, expectedLibs, libs)
}

func TestComputeRequestedSymbols(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{}, &mockManager{}, nil, nil)
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
		ua.config.ProbeSelectors = selectorsOnlyAllOf
		requested, err := ua.computeSymbolsToRequest()
		require.NoError(tt, err)
		require.ElementsMatch(tt, []SymbolRequest{{Name: "SSL_connect"}}, requested)
	})

	selectorsBestEfforAndMandatory := []manager.ProbesSelector{
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
		ua.config.ProbeSelectors = selectorsBestEfforAndMandatory
		requested, err := ua.computeSymbolsToRequest()
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
		ua.config.ProbeSelectors = selectorsBestEffort
		requested, err := ua.computeSymbolsToRequest()
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
		ua.config.ProbeSelectors = selectorsWithReturnFunctions
		requested, err := ua.computeSymbolsToRequest()
		require.NoError(tt, err)
		require.ElementsMatch(tt, []SymbolRequest{{Name: "SSL_connect", IncludeReturnLocations: true}}, requested)
	})
}

func TestStartAndStopWithoutLibraryWatcher(t *testing.T) {
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{}, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	err = ua.Start()
	require.NoError(t, err)

	ua.Stop()
}

func TestStartAndStopWithLibraryWatcher(t *testing.T) {
	rules := []*AttachRule{{LibraryNameRegex: regexp.MustCompile(`libssl.so`)}}
	ua, err := NewUprobeAttacher("mock", &AttacherConfig{Rules: rules}, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)
	require.True(t, ua.handlesLibraries())
	require.NotNil(t, ua.soWatcher)

	err = ua.Start()
	require.NoError(t, err)

	ua.Stop()
}

func TestMonitor(t *testing.T) {
	config := &AttacherConfig{
		Rules:                     []*AttachRule{{LibraryNameRegex: regexp.MustCompile(`libssl.so`)}},
		ProcessMonitorEventStream: false,
	}
	ua, err := NewUprobeAttacher("mock", config, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	mockRegistry := &mockFileRegistry{}
	ua.fileRegistry = mockRegistry

	// Tell mockRegistry to return on any calls, we will check the values later
	mockRegistry.On("Clear").Return()
	mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "libmmap")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))

	ua.Start()
	t.Cleanup(ua.Stop)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(mockRegistry.Calls) == 2 // Once for the library, another for the process itself
	}, 100*time.Millisecond, 10*time.Millisecond)

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

	config := &AttacherConfig{ProcRoot: procFS}
	ua, err := NewUprobeAttacher("mock", config, &mockManager{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, ua)

	mockRegistry := &mockFileRegistry{}
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

func TestAttachToBinary(t *testing.T) {
	proc := FakeProcFSEntry{
		pid:     1,
		cmdline: "/bin/bash",
		exe:     "/bin/bash",
	}
	procFS := createFakeProcFS(t, []FakeProcFSEntry{proc})

	config := &AttacherConfig{
		ProcRoot: procFS,
		ProbeSelectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}},
		},
	}

	mockMan := &mockManager{}
	inspector := &mockBinaryInspector{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: proc.exe,
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target.HostPath, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)

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

	err = ua.attachToBinary(target, nil)
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
		ProbeSelectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect__return"}},
		},
	}

	mockMan := &mockManager{}
	inspector := &mockBinaryInspector{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: proc.exe,
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234, ReturnLocations: []uint64{0x0, 0x1}}
	inspector.On("Inspect", target.HostPath, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)

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

	err = ua.attachToBinary(target, nil)
	require.NoError(t, err)
	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}

func TestAttachToBinaryShouldIgnoreNonMatchingProbes(t *testing.T) {
	proc := FakeProcFSEntry{
		pid:     1,
		cmdline: "/bin/bash",
		exe:     "/bin/bash",
	}
	procFS := createFakeProcFS(t, []FakeProcFSEntry{proc})

	config := &AttacherConfig{
		ProcRoot: procFS,
		ProbeSelectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "uprobe__SSL_connect",
			}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "uprobe__TLS_connect",
			}},
		},
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libssl.so`),
				UprobeNameRegex:  regexp.MustCompile(`SSL_.*`),
			},
		},
	}

	mockMan := &mockManager{}
	inspector := &mockBinaryInspector{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: "/usr/lib/libssl.so",
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target.HostPath, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)

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

	// if this function calls the manager adding a probe with a different name than the one we requested, the test
	// will fail
	err = ua.attachToBinary(target, nil)
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
		ProbeSelectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "uprobe__SSL_connect",
			}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "uprobe__TLS_connect",
			}},
		},
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libssl.so`),
				UprobeNameRegex:  regexp.MustCompile(`SSL_.*`),
			},
			{
				LibraryNameRegex: regexp.MustCompile(`libtls.so`),
				UprobeNameRegex:  regexp.MustCompile(`SSL_.*`),
			},
		},
	}

	mockMan := &mockManager{}
	inspector := &mockBinaryInspector{}
	ua, err := NewUprobeAttacher("mock", config, mockMan, nil, inspector)
	require.NoError(t, err)
	require.NotNil(t, ua)

	target := utils.FilePath{
		HostPath: "/usr/lib/libssl.so",
		PID:      proc.pid,
	}

	// Tell the inspector to return a simple symbol
	symbolToAttach := bininspect.FunctionMetadata{EntryLocation: 0x1234}
	inspector.On("Inspect", target.HostPath, mock.Anything).Return(map[string]bininspect.FunctionMetadata{"SSL_connect": symbolToAttach}, true, nil)

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

	// if this function calls the manager adding a probe with a different name than the one we requested, the test
	// will fail
	err = ua.attachToLibrariesOfPID(proc.pid)
	require.NoError(t, err)
	inspector.AssertExpectations(t)
	mockMan.AssertExpectations(t)
}
