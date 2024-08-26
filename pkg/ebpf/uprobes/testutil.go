// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

// === Mocks

// MockManager is a mock implementation of the manager.Manager interface.
type MockManager struct {
	mock.Mock
}

// AddHook is a mock implementation of the manager.Manager.AddHook method.
func (m *MockManager) AddHook(name string, probe *manager.Probe) error {
	args := m.Called(name, probe)
	return args.Error(0)
}

// DetachHook is a mock implementation of the manager.Manager.DetachHook method.
func (m *MockManager) DetachHook(probeID manager.ProbeIdentificationPair) error {
	args := m.Called(probeID)
	return args.Error(0)
}

// GetProbe is a mock implementation of the manager.Manager.GetProbe method.
func (m *MockManager) GetProbe(probeID manager.ProbeIdentificationPair) (*manager.Probe, bool) {
	args := m.Called(probeID)
	return args.Get(0).(*manager.Probe), args.Bool(1)
}

// MockFileRegistry is a mock implementation of the FileRegistry interface.
type MockFileRegistry struct {
	mock.Mock
}

// Register is a mock implementation of the FileRegistry.Register method.
func (m *MockFileRegistry) Register(namespacedPath string, pid uint32, activationCB, deactivationCB utils.Callback) error {
	args := m.Called(namespacedPath, pid, activationCB, deactivationCB)
	return args.Error(0)
}

// Unregister is a mock implementation of the FileRegistry.Unregister method.
func (m *MockFileRegistry) Unregister(pid uint32) error {
	args := m.Called(pid)
	return args.Error(0)
}

// Clear is a mock implementation of the FileRegistry.Clear method.
func (m *MockFileRegistry) Clear() {
	m.Called()
}

// GetRegisteredProcesses is a mock implementation of the FileRegistry.GetRegisteredProcesses method.
func (m *MockFileRegistry) GetRegisteredProcesses() map[uint32]struct{} {
	args := m.Called()
	return args.Get(0).(map[uint32]struct{})
}

// MockBinaryInspector is a mock implementation of the BinaryInspector interface.
type MockBinaryInspector struct {
	mock.Mock
}

// Inspect is a mock implementation of the BinaryInspector.Inspect method.
func (m *MockBinaryInspector) Inspect(fpath utils.FilePath, requests []SymbolRequest) (map[string]bininspect.FunctionMetadata, bool, error) {
	args := m.Called(fpath, requests)
	return args.Get(0).(map[string]bininspect.FunctionMetadata), args.Bool(1), args.Error(2)
}

// Cleanup is a mock implementation of the BinaryInspector.Cleanup method.
func (m *MockBinaryInspector) Cleanup(fpath utils.FilePath) {
	_ = m.Called(fpath)
}

// === Test utils

// FakeProcFSEntry represents a fake /proc filesystem entry for testing purposes.
type FakeProcFSEntry struct {
	Pid     uint32
	Cmdline string
	Command string
	Exe     string
	Maps    string
}

// CreateFakeProcFS creates a fake /proc filesystem with the given entries, useful for testing attachment to processes.
func CreateFakeProcFS(t *testing.T, entries []FakeProcFSEntry) string {
	procRoot := t.TempDir()

	for _, entry := range entries {
		baseDir := filepath.Join(procRoot, strconv.Itoa(int(entry.Pid)))

		createFile(t, filepath.Join(baseDir, "cmdline"), entry.Cmdline)
		createFile(t, filepath.Join(baseDir, "comm"), entry.Command)
		createFile(t, filepath.Join(baseDir, "maps"), entry.Maps)
		createSymlink(t, entry.Exe, filepath.Join(baseDir, "exe"))
	}

	return procRoot
}

func createFile(t *testing.T, path, data string) {
	if data == "" {
		return
	}

	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.WriteFile(path, []byte(data), 0775))
}

func createSymlink(t *testing.T, target, link string) {
	if target == "" {
		return
	}

	dir := filepath.Dir(link)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.Symlink(target, link))
}

func getLibSSLPath(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "libmmap")
	return filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))
}
