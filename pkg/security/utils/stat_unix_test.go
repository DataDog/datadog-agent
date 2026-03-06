// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package utils

import (
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnixStat(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		// Use a file that should exist on most systems
		stat, err := UnixStat("/etc/passwd")
		require.NoError(t, err)
		assert.True(t, stat.Mode&syscall.S_IFREG != 0, "Expected regular file")
	})

	t.Run("existing directory", func(t *testing.T) {
		stat, err := UnixStat("/tmp")
		require.NoError(t, err)
		assert.True(t, stat.Mode&syscall.S_IFDIR != 0, "Expected directory")
	})

	t.Run("non-existent path", func(t *testing.T) {
		_, err := UnixStat("/non/existent/path/that/does/not/exist")
		assert.Error(t, err)
	})
}

func TestUnixStatModeToGoFileMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     uint32
		expected fs.FileMode
	}{
		{
			name:     "regular file with 644 permissions",
			mode:     syscall.S_IFREG | 0644,
			expected: fs.FileMode(0644),
		},
		{
			name:     "directory with 755 permissions",
			mode:     syscall.S_IFDIR | 0755,
			expected: fs.ModeDir | fs.FileMode(0755),
		},
		{
			name:     "symlink",
			mode:     syscall.S_IFLNK | 0777,
			expected: fs.ModeSymlink | fs.FileMode(0777),
		},
		{
			name:     "block device",
			mode:     syscall.S_IFBLK | 0660,
			expected: fs.ModeDevice | fs.FileMode(0660),
		},
		{
			name:     "character device",
			mode:     syscall.S_IFCHR | 0666,
			expected: fs.ModeDevice | fs.ModeCharDevice | fs.FileMode(0666),
		},
		{
			name:     "named pipe (FIFO)",
			mode:     syscall.S_IFIFO | 0644,
			expected: fs.ModeNamedPipe | fs.FileMode(0644),
		},
		{
			name:     "socket",
			mode:     syscall.S_IFSOCK | 0755,
			expected: fs.ModeSocket | fs.FileMode(0755),
		},
		{
			name:     "setuid bit",
			mode:     syscall.S_IFREG | syscall.S_ISUID | 0755,
			expected: fs.ModeSetuid | fs.FileMode(0755),
		},
		{
			name:     "setgid bit",
			mode:     syscall.S_IFREG | syscall.S_ISGID | 0755,
			expected: fs.ModeSetgid | fs.FileMode(0755),
		},
		{
			name:     "sticky bit",
			mode:     syscall.S_IFDIR | syscall.S_ISVTX | 0777,
			expected: fs.ModeDir | fs.ModeSticky | fs.FileMode(0777),
		},
		{
			name:     "all special bits",
			mode:     syscall.S_IFREG | syscall.S_ISUID | syscall.S_ISGID | syscall.S_ISVTX | 0755,
			expected: fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | fs.FileMode(0755),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnixStatModeToGoFileMode(tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUnixStat_CompareWithOsStat(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "unixstat_test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Get stat using our function
	unixStat, err := UnixStat(tmpFile.Name())
	require.NoError(t, err)

	// Get stat using os.Stat
	osStat, err := os.Stat(tmpFile.Name())
	require.NoError(t, err)

	// Compare the mode
	convertedMode := UnixStatModeToGoFileMode(uint32(unixStat.Mode))
	assert.Equal(t, osStat.Mode(), convertedMode)
}
