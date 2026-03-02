// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestProcExePath(t *testing.T) {
	path := ProcExePath(1234)
	assert.True(t, strings.HasSuffix(path, "1234/exe"))
	assert.Contains(t, path, "proc")
}

func TestStatusPath(t *testing.T) {
	path := StatusPath(5678)
	assert.True(t, strings.HasSuffix(path, "5678/status"))
	assert.Contains(t, path, "proc")
}

func TestTaskStatusPath(t *testing.T) {
	path := TaskStatusPath(1234, "5678")
	assert.True(t, strings.HasSuffix(path, "1234/task/5678/status"))
	assert.Contains(t, path, "proc")
}

func TestLoginUIDPath(t *testing.T) {
	path := LoginUIDPath(9999)
	assert.True(t, strings.HasSuffix(path, "9999/loginuid"))
	assert.Contains(t, path, "proc")
}

func TestProcRootPath(t *testing.T) {
	path := ProcRootPath(1234)
	assert.True(t, strings.HasSuffix(path, "1234/root"))
	assert.Contains(t, path, "proc")
}

func TestProcRootFilePath(t *testing.T) {
	tests := []struct {
		name     string
		pid      uint32
		file     string
		expected string
	}{
		{
			name:     "file with leading slash",
			pid:      1234,
			file:     "/etc/passwd",
			expected: filepath.Join(kernel.ProcFSRoot(), "1234", "root", "etc/passwd"),
		},
		{
			name:     "file without leading slash",
			pid:      1234,
			file:     "etc/passwd",
			expected: filepath.Join(kernel.ProcFSRoot(), "1234", "root", "etc/passwd"),
		},
		{
			name:     "empty file",
			pid:      1234,
			file:     "",
			expected: filepath.Join(kernel.ProcFSRoot(), "1234", "root"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcRootFilePath(tt.pid, tt.file)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModulesPath(t *testing.T) {
	path := ModulesPath()
	assert.True(t, strings.HasSuffix(path, "modules"))
	assert.Contains(t, path, "proc")
}

func TestCgroupTaskPath(t *testing.T) {
	path := CgroupTaskPath(1234, 5678)
	assert.True(t, strings.HasSuffix(path, "1234/task/5678/cgroup"))
	assert.Contains(t, path, "proc")
}

func TestNewNSPathFromPid(t *testing.T) {
	nsPath := NewNSPathFromPid(1234, NetNsType)
	require.NotNil(t, nsPath)
	assert.Equal(t, uint32(1234), nsPath.pid)
	assert.Equal(t, NetNsType, nsPath.nsType)
	assert.Empty(t, nsPath.cachedPath)
}

func TestNewNSPathFromPath(t *testing.T) {
	nsPath := NewNSPathFromPath("/some/path", PidNsType)
	require.NotNil(t, nsPath)
	assert.Equal(t, "/some/path", nsPath.cachedPath)
	assert.Equal(t, PidNsType, nsPath.nsType)
	assert.Zero(t, nsPath.pid)
}

func TestNsType_Constants(t *testing.T) {
	// Verify namespace type constants are correct
	assert.Equal(t, NsType("cgroup"), CGroupNsType)
	assert.Equal(t, NsType("ipc"), IpcNsType)
	assert.Equal(t, NsType("mnt"), MntNsType)
	assert.Equal(t, NsType("net"), NetNsType)
	assert.Equal(t, NsType("pid"), PidNsType)
	assert.Equal(t, NsType("pid_for_children"), PidForChildrenNsType)
	assert.Equal(t, NsType("time"), TimeNsType)
	assert.Equal(t, NsType("time_for_children"), TimeForChildrenNsType)
	assert.Equal(t, NsType("user"), UserNsType)
	assert.Equal(t, NsType("uts"), UtsNsType)
}

func TestGetpid(t *testing.T) {
	// Should return a non-zero PID
	pid := Getpid()
	assert.NotZero(t, pid)
}
