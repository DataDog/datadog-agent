// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// --- collectProcessLogs tests ---

func TestCollectProcessLogs_NilWmeta(t *testing.T) {
	entries := collectProcessLogs(nil)
	assert.Nil(t, entries)
}

func TestCollectProcessLogs_WithMock(t *testing.T) {
	mock := &mockWorkloadMeta{
		processes: []*workloadmeta.Process{
			{
				Pid:  1234,
				Name: "myapp",
				Service: &workloadmeta.Service{
					GeneratedName: "my-service",
					LogFiles:      []string{"/var/log/myapp.log", "/var/log/myapp-error.log"},
				},
			},
			{
				Pid:     5678,
				Name:    "no-service",
				Service: nil,
			},
			{
				Pid:  9999,
				Name: "empty-logs",
				Service: &workloadmeta.Service{
					GeneratedName: "empty-svc",
					LogFiles:      nil,
				},
			},
		},
	}

	entries := collectProcessLogs(mock)
	require.Len(t, entries, 2)

	assert.Equal(t, "/var/log/myapp.log", entries[0].Path)
	assert.Equal(t, "process", entries[0].Source)
	assert.Equal(t, "myapp", entries[0].ProcessName)
	assert.Equal(t, int32(1234), entries[0].PID)
	assert.Equal(t, "my-service", entries[0].ServiceName)

	assert.Equal(t, "/var/log/myapp-error.log", entries[1].Path)
}

// --- collectK8sLogs tests ---

func TestCollectK8sLogs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create /var/log/pods structure
	podsDir := filepath.Join(tmpDir, "var", "log", "pods", "ns_pod_uid", "container")
	require.NoError(t, os.MkdirAll(podsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(podsDir, "0.log"), []byte("log"), 0644))

	// Create /var/log/containers structure
	containersDir := filepath.Join(tmpDir, "var", "log", "containers")
	require.NoError(t, os.MkdirAll(containersDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(containersDir, "container.log"), []byte("log"), 0644))

	// Create a non-.log file that should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(containersDir, "not-a-log.txt"), []byte("nope"), 0644))

	entries, errs := collectK8sLogs(tmpDir)
	assert.Empty(t, errs)
	assert.Len(t, entries, 2)

	for _, e := range entries {
		assert.Equal(t, "kubernetes", e.Source)
		assert.NotEmpty(t, e.Path)
	}
}

func TestCollectK8sLogs_NonExistentDir(t *testing.T) {
	entries, errs := collectK8sLogs("/nonexistent/prefix")
	assert.Empty(t, errs)
	assert.Empty(t, entries)
}

// --- collectFilesystemLogs tests ---

func TestCollectFilesystemLogs_VarLog(t *testing.T) {
	tmpDir := t.TempDir()

	// /var/log/ with some files
	varLog := filepath.Join(tmpDir, "var", "log")
	require.NoError(t, os.MkdirAll(varLog, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(varLog, "syslog"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(varLog, "messages"), []byte("data"), 0644))

	// /var/log/pods should be excluded by isLogFile
	podsDir := filepath.Join(varLog, "pods")
	require.NoError(t, os.MkdirAll(podsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(podsDir, "pod.log"), []byte("data"), 0644))

	entries, errs := collectFilesystemLogs(tmpDir, nil)
	assert.Empty(t, errs)

	// syslog and messages should be included; pods/pod.log should not
	var paths []string
	for _, e := range entries {
		paths = append(paths, e.Path)
		assert.Equal(t, "filesystem", e.Source)
	}
	assert.Contains(t, paths, "/var/log/syslog")
	assert.Contains(t, paths, "/var/log/messages")
	assert.NotContains(t, paths, "/var/log/pods/pod.log")
}

func TestCollectFilesystemLogs_AdditionalDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an additional directory with mixed files
	appLogs := filepath.Join(tmpDir, "opt", "app", "logs")
	require.NoError(t, os.MkdirAll(appLogs, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appLogs, "app.log"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(appLogs, "app.txt"), []byte("data"), 0644))

	entries, errs := collectFilesystemLogs(tmpDir, []string{"/opt/app/logs"})
	assert.Empty(t, errs)

	// Both files should be included (additional dirs include all regular files)
	var paths []string
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	assert.Contains(t, paths, "/opt/app/logs/app.log")
	assert.Contains(t, paths, "/opt/app/logs/app.txt")
}

func TestCollectFilesystemLogs_AdditionalDirNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	entries, errs := collectFilesystemLogs(tmpDir, []string{"/nonexistent/dir"})
	assert.Empty(t, entries)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "additional directory not found")
}

// --- isLogFile tests ---

func TestIsLogFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/var/log/syslog", true},
		{"/var/log/messages", true},
		{"/var/log/auth.log", true},
		{"/var/log/nginx/access.log", true},
		{"/var/log/pods/something.log", false},
		{"/var/log/containers/test.log", false},
		{"/var/log/docker/something.log", false},
		{"/opt/app/app.log", true},
		{"/opt/app/data.txt", false},
		{"/var/lib/docker/containers/abc/abc.log", false},
		{"/home/user/output.log", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, isLogFile(tt.path), "isLogFile(%q)", tt.path)
		})
	}
}

// --- validateAdditionalDirs tests ---

func TestValidateAdditionalDirs(t *testing.T) {
	tests := []struct {
		name    string
		dirs    []string
		wantErr bool
	}{
		{"nil dirs", nil, false},
		{"empty dirs", []string{}, false},
		{"valid absolute paths", []string{"/opt/logs", "/var/custom"}, false},
		{"relative path", []string{"relative/path"}, true},
		{"path with ..", []string{"/opt/../etc"}, true},
		{"mixed valid and invalid", []string{"/opt/logs", "relative"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAdditionalDirs(tt.dirs)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Deduplication test ---

func TestDeduplication(t *testing.T) {
	// Simulate two sources returning the same path
	seen := make(map[string]struct{})
	var result []LogFileEntry

	entries := []LogFileEntry{
		{Path: "/var/log/syslog", Source: "process"},
		{Path: "/var/log/syslog", Source: "filesystem"},
		{Path: "/var/log/other.log", Source: "filesystem"},
	}

	for _, entry := range entries {
		if _, ok := seen[entry.Path]; ok {
			continue
		}
		seen[entry.Path] = struct{}{}
		result = append(result, entry)
	}

	assert.Len(t, result, 2)
	assert.Equal(t, "process", result[0].Source) // first seen wins
}

// --- Host path conversion tests ---

func TestToHostPath(t *testing.T) {
	assert.Equal(t, "/host/var/log", toHostPath("/host", "/var/log"))
	assert.Equal(t, "/var/log", toHostPath("", "/var/log"))
}

func TestToOutputPath(t *testing.T) {
	assert.Equal(t, "/var/log", toOutputPath("/host", "/host/var/log"))
	assert.Equal(t, "/var/log", toOutputPath("", "/var/log"))
}

// --- mockWorkloadMeta implements the subset of workloadmeta.Component we need ---

type mockWorkloadMeta struct {
	workloadmeta.Component
	processes []*workloadmeta.Process
}

func (m *mockWorkloadMeta) ListProcesses() []*workloadmeta.Process {
	return m.processes
}
