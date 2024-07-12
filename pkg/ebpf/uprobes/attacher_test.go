// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockManager struct {
	mock.Mock
}

func (m *MockManager) AddHook(name string, probe *manager.Probe) error {
	args := m.Called(name, probe)
	return args.Error(0)
}

func (m *MockManager) DetachHook(probeId manager.ProbeIdentificationPair) error {
	args := m.Called(probeId)
	return args.Error(0)
}

func (m *MockManager) GetProbe(probeId manager.ProbeIdentificationPair) (*manager.Probe, bool) {
	args := m.Called(probeId)
	return args.Get(0).(*manager.Probe), args.Bool(1)
}

func TestCanCreateAttacher(t *testing.T) {
	ua := NewUprobeAttacher("mock", &AttacherConfig{}, &MockManager{}, nil)
	require.NotNil(t, ua)
}

func TestAttachPidExcludesSelf(t *testing.T) {
	config := &AttacherConfig{
		ExcludeTargets: ExcludeSelf,
	}
	ua := NewUprobeAttacher("mock", config, &MockManager{}, nil)
	require.NotNil(t, ua)

	err := ua.AttachPID(uint32(os.Getpid()), false)
	require.ErrorIs(t, err, ErrSelfExcluded)
}

func TestGetExecutablePath(t *testing.T) {
	exe := "/bin/bash"
	procRoot := createFakeProcFS(t, []FakeProcFSEntry{{pid: 1, cmdline: "", command: exe, exe: exe}})
	config := &AttacherConfig{
		ProcRoot: procRoot,
	}
	ua := NewUprobeAttacher("mock", config, &MockManager{}, nil)
	require.NotNil(t, ua)

	path, err := ua.getExecutablePath(1)
	require.NoError(t, err, "failed to get executable path for existing PID")

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
	procRoot := createFakeProcFS(t, []FakeProcFSEntry{{pid: pid, maps: mapsFileSample}})
	config := &AttacherConfig{
		ProcRoot: procRoot,
	}
	ua := NewUprobeAttacher("mock", config, &MockManager{}, nil)
	require.NotNil(t, ua)

	libs, err := ua.getLibrariesFromMapsFile(pid)
	require.NoError(t, err, "failed to get libraries from maps file")
	require.NotEmpty(t, libs, "should return libraries from maps file")
	expectedLibs := []string{"/opt/test", "/lib/libc.so.6", "/lib/libpthread.so.0", "/lib/ld-linux.so.2"}
	require.ElementsMatch(t, expectedLibs, libs)
}

type FakeProcFSEntry struct {
	pid     int
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
