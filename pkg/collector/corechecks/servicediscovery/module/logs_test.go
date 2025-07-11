// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-log-files")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	writeLogPath := filepath.Join(tempDir, "test.log")
	f, err := os.OpenFile(writeLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)
	defer f.Close()

	noAppendLogPath := filepath.Join(tempDir, "noappend.log")
	f, err = os.OpenFile(noAppendLogPath, os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	defer f.Close()

	readWriteLogPath := filepath.Join(tempDir, "readwrite.log")
	f, err = os.OpenFile(readWriteLogPath, os.O_CREATE|os.O_RDWR, 0644)
	require.NoError(t, err)
	defer f.Close()

	writeOtherPath := filepath.Join(tempDir, "test.log.txt")
	f, err = os.OpenFile(writeOtherPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)
	defer f.Close()

	readLogPath := filepath.Join(tempDir, "read.log")
	f, err = os.Create(readLogPath)
	require.NoError(t, err)
	f.Close()
	f, err = os.Open(readLogPath)
	require.NoError(t, err)
	defer f.Close()

	writeLargePath := filepath.Join(tempDir, strings.Repeat("a", 128)+".log")
	f, err = os.OpenFile(writeLargePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)
	defer f.Close()

	// Add a duplicate fdPath for writeLogPath to test deduplication
	f, err = os.OpenFile(writeLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)
	defer f.Close()

	self := int32(os.Getpid())
	buf := make([]byte, readlinkBufferSize)
	openFilesInfo, err := getOpenFilesInfo(self, buf)
	require.NoError(t, err)

	count := 0
	for _, log := range openFilesInfo.logs {
		if log.path == writeLogPath {
			count++
		}
	}
	require.Equal(t, 2, count, "writeLogPath should appear twice in the candidates")

	logFiles := getLogFiles(self, openFilesInfo.logs)

	assert.Contains(t, logFiles, writeLogPath)
	assert.NotContains(t, logFiles, noAppendLogPath)
	assert.NotContains(t, logFiles, readWriteLogPath)
	assert.Contains(t, logFiles, writeLargePath)
	assert.NotContains(t, logFiles, writeOtherPath)
	assert.NotContains(t, logFiles, readLogPath)

	// Ensure writeLogPath appears only once
	count = 0
	for _, logFile := range logFiles {
		if logFile == writeLogPath {
			count++
		}
	}
	assert.Equal(t, 1, count, "writeLogPath should appear only once even if duplicated in input")
}

func TestIsLogFile(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "valid log file",
			path:     "/tmp/foo/application.log",
			expected: true,
		},
		{
			name:     "docker container log excluded",
			path:     "/var/lib/docker/containers/abc123/abc123-json.log",
			expected: false,
		},
		{
			name:     "kubernetes pod log excluded",
			path:     "/var/log/pods/namespace_pod_uid/container/0.log",
			expected: false,
		},
		{
			name:     "file without log extension",
			path:     "/usr/local/application.txt",
			expected: false,
		},
		{
			name:     "file with extensions after .log",
			path:     "/bar/tmp/application.log.gz",
			expected: false,
		},
		{
			name:     "file with log in name but not extension",
			path:     "/foo/logfile.txt",
			expected: false,
		},
		{
			name:     "/var/log file without .log extension",
			path:     "/var/log/messages",
			expected: true,
		},
		{
			name:     "empty path",
			path:     "",
			expected: false,
		},
		{
			name:     "path with only .log",
			path:     ".log",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLogFile(tt.path)
			assert.Equal(t, tt.expected, result, "isLogFile(%q) should return %v", tt.path, tt.expected)
		})
	}
}
