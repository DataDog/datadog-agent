// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProcess struct {
	uids []uint32
	gids []uint32
	err  error
}

func (m *mockProcess) Uids() ([]uint32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.uids, nil
}

func (m *mockProcess) Gids() ([]uint32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.gids, nil
}

func newMockProcess(uid, gid uint32) *mockProcess {
	return &mockProcess{
		uids: []uint32{uid, uid, uid},
		gids: []uint32{gid, gid, gid},
	}
}

func TestReadProcessFileLimit(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Test requires root privileges to chown files")
	}

	tempDir := t.TempDir()
	root, err := os.OpenRoot(tempDir)
	require.NoError(t, err)

	// Use non-root UIDs for testing to avoid the root-owned security policy
	testUID := uint32(1000)
	testGID := uint32(1000)
	otherUID := uint32(2000)
	otherGID := uint32(2000)

	createTestFile := func(t *testing.T, name string, content string, uid, gid uint32, mode os.FileMode) string {
		path := filepath.Join(tempDir, name)
		dir := filepath.Dir(path)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), mode))
		require.NoError(t, os.Chown(path, int(uid), int(gid)))
		return name
	}

	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		proc        *mockProcess
		maxSize     int64
		expectError string
		expectData  string
	}{
		{
			name: "successful read - owner with read permission",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "owner_readable.txt", "owner content", testUID, testGID, 0400)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "owner content",
		},
		{
			name: "successful read - owner with rw permission",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "owner_rw.txt", "owner rw", testUID, testGID, 0600)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "owner rw",
		},
		{
			name: "successful read - group with read permission",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "group_readable.txt", "group content", otherUID, testGID, 0040)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "group content",
		},
		{
			name: "successful read - world readable (non-root owned)",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "world_readable.txt", "world content", otherUID, otherGID, 0004)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "world content",
		},
		{
			name: "successful read - root process can read any file",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "root_process.txt", "root reads all", otherUID, otherGID, 0000)
			},
			proc:       newMockProcess(0, 0),
			maxSize:    1024,
			expectData: "root reads all",
		},
		{
			name: "file size limit enforced",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "large_file.txt", "this is a long content that exceeds limit", testUID, testGID, 0400)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    10,
			expectData: "this is a ",
		},
		{
			name: "permission denied - root-owned non-world-readable",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "root_owned_not_world.txt", "root file", 0, 0, 0600)
			},
			proc:        newMockProcess(0, 0),
			maxSize:     1024,
			expectError: "is not readable by process",
		},
		{
			name: "permission denied - no read permission",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "no_read.txt", "cannot read", otherUID, otherGID, 0000)
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is not readable by process",
		},
		{
			name: "permission denied - owner without read",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "owner_no_read.txt", "no read", testUID, testGID, 0200)
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is not readable by process",
		},
		{
			name: "permission denied - group without read",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "group_no_read.txt", "no read", otherUID, testGID, 0020)
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is not readable by process",
		},
		{
			name: "successful read - root-owned world-readable by non-root",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "root_world_readable.txt", "world readable", 0, 0, 0644)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "world readable",
		},
		{
			name: "permission denied - root-owned non-world-readable blocked by security policy",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "root_owned_secret.txt", "secret", 0, 0, 0600)
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is not readable by process",
		},
		{
			name: "permission denied - root-owned 0400 blocked by security policy",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "root_owned_0400.txt", "secret", 0, 0, 0400)
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is not readable by process",
		},
		{
			name: "error - file is executable",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "executable.sh", "#!/bin/bash\necho test", testUID, testGID, 0755)
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is executable",
		},
		{
			name: "error - file is a directory",
			setupFile: func(t *testing.T) string {
				dir := "testdir"
				require.NoError(t, os.MkdirAll(filepath.Join(tempDir, dir), 0755))
				return dir
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "is a directory",
		},
		{
			name: "error - file does not exist",
			setupFile: func(_ *testing.T) string {
				return "nonexistent.txt"
			},
			proc:        newMockProcess(testUID, testGID),
			maxSize:     1024,
			expectError: "no such file or directory",
		},
		{
			name: "absolute path handling",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "abs_path.txt", "absolute", testUID, testGID, 0400)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "absolute",
		},
		{
			name: "empty file",
			setupFile: func(t *testing.T) string {
				return createTestFile(t, "empty.txt", "", testUID, testGID, 0400)
			},
			proc:       newMockProcess(testUID, testGID),
			maxSize:    1024,
			expectData: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := tt.setupFile(t)

			data, fileInfo, err := ReadProcessFileLimit(tt.proc, root, filename, tt.maxSize)

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, data)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectData, string(data))
			assert.NotNil(t, fileInfo)
			assert.False(t, fileInfo.IsDir())
		})
	}
}

func TestReadProcessFileLimit_ProcessErrors(t *testing.T) {
	tempDir := t.TempDir()
	root, err := os.OpenRoot(tempDir)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	tests := []struct {
		name        string
		proc        *mockProcess
		expectError bool
	}{
		{
			name: "error getting UIDs",
			proc: &mockProcess{
				err: assert.AnError,
			},
			expectError: true,
		},
		{
			name: "empty UIDs slice - returns error",
			proc: &mockProcess{
				uids: []uint32{},
				gids: []uint32{1000, 1000, 1000},
			},
			expectError: true,
		},
		{
			name: "empty GIDs slice - returns error",
			proc: &mockProcess{
				uids: []uint32{1000, 1000, 1000},
				gids: []uint32{},
			},
			expectError: true,
		},
		{
			name: "single UID - returns error (needs at least 2)",
			proc: &mockProcess{
				uids: []uint32{1000},
				gids: []uint32{1000, 1000, 1000},
			},
			expectError: true,
		},
		{
			name: "single GID - returns error (needs at least 2)",
			proc: &mockProcess{
				uids: []uint32{1000, 1000, 1000},
				gids: []uint32{1000},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, fileInfo, err := ReadProcessFileLimit(tt.proc, root, "test.txt", 1024)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, data)
				assert.Nil(t, fileInfo)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, data)
				assert.NotNil(t, fileInfo)
			}
		})
	}
}

func TestReadProcessFileLimit_PathHandling(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Test requires root privileges to chown files")
	}

	tempDir := t.TempDir()
	root, err := os.OpenRoot(tempDir)
	require.NoError(t, err)

	testUID := uint32(1000)
	testGID := uint32(1000)

	// Create test file in subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	testFile := filepath.Join(subDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("path test"), 0400))
	require.NoError(t, os.Chown(testFile, int(testUID), int(testGID)))

	proc := newMockProcess(testUID, testGID)

	tests := []struct {
		name        string
		path        string
		expected    string
		expectError bool
	}{
		{
			name:        "relative path",
			path:        "subdir/test.txt",
			expected:    "path test",
			expectError: false,
		},
		{
			name:        "absolute path",
			path:        "/subdir/test.txt",
			expected:    "path test",
			expectError: false,
		},
		{
			name:        "path with dots",
			path:        "./subdir/./test.txt",
			expected:    "path test",
			expectError: false,
		},
		{
			name:        "path traversal - parent directory",
			path:        "../out",
			expectError: true,
		},
		{
			name:        "path traversal - multiple levels up",
			path:        "../../out",
			expectError: true,
		},
		{
			name:        "path traversal - from subdirectory",
			path:        "subdir/../../out",
			expectError: true,
		},
		{
			name:        "path traversal - absolute with parent",
			path:        "/../out",
			expectError: true,
		},
		{
			name:        "path traversal - complex",
			path:        "/subdir/../../../out",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, fileInfo, err := ReadProcessFileLimit(proc, root, tt.path, 1024)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, data)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, string(data))
				assert.NotNil(t, fileInfo)
			}
		})
	}
}
